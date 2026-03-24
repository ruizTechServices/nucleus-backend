//go:build windows

package transport

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var waitNamedPipeProc = windows.NewLazySystemDLL("kernel32.dll").NewProc("WaitNamedPipeW")

type namedPipeListener struct {
	endpoint string

	mu           sync.Mutex
	acceptHandle windows.Handle
	closing      bool
	firstPipe    bool
}

func DefaultEndpoint(name string) string {
	return `\\.\pipe\` + sanitizeName(name)
}

func NewLocalListener(endpoint string) (Listener, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return nil, fmt.Errorf("named pipe endpoint is required")
	}

	if !strings.HasPrefix(trimmed, `\\.\pipe\`) {
		return nil, fmt.Errorf("named pipe endpoint must start with \\\\.\\pipe\\")
	}

	return &namedPipeListener{
		endpoint:  trimmed,
		firstPipe: true,
	}, nil
}

func (l *namedPipeListener) BeginShutdown() error {
	return nil
}

func (l *namedPipeListener) Serve(ctx context.Context, handler Handler) error {
	if handler == nil {
		return fmt.Errorf("transport handler is required")
	}

	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			_ = l.Close()
		}()
	}

	for {
		handle, err := l.createPipeInstance()
		if err != nil {
			if l.isClosing() || (ctx != nil && ctx.Err() != nil) {
				return nil
			}
			return err
		}

		err = windows.ConnectNamedPipe(handle, nil)
		l.clearAcceptHandle(handle)

		switch {
		case err == nil:
		case err == windows.ERROR_PIPE_CONNECTED:
		default:
			_ = windows.CloseHandle(handle)
			if l.isClosing() || (ctx != nil && ctx.Err() != nil) {
				return nil
			}
			return err
		}

		if l.isClosing() || (ctx != nil && ctx.Err() != nil) {
			_ = windows.CloseHandle(handle)
			return nil
		}

		go handleConnection(os.NewFile(uintptr(handle), l.endpoint), handler)
	}
}

func (l *namedPipeListener) Close() error {
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		return nil
	}

	l.closing = true
	l.acceptHandle = 0
	endpoint := l.endpoint
	l.mu.Unlock()

	closeCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	connection, err := dialEndpoint(closeCtx, endpoint)
	if err == nil && connection != nil {
		return connection.Close()
	}

	return nil
}

func (l *namedPipeListener) Network() string {
	return "named-pipe"
}

func (l *namedPipeListener) createPipeInstance() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(l.endpoint)
	if err != nil {
		return 0, err
	}

	securityAttributes, err := newPipeSecurityAttributes()
	if err != nil {
		return 0, err
	}

	flags := uint32(windows.PIPE_ACCESS_DUPLEX)

	l.mu.Lock()
	if l.firstPipe {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
		l.firstPipe = false
	}
	l.mu.Unlock()

	pipeMode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	handle, err := windows.CreateNamedPipe(
		name,
		flags,
		pipeMode,
		windows.PIPE_UNLIMITED_INSTANCES,
		64*1024,
		64*1024,
		0,
		securityAttributes,
	)
	if err != nil {
		return 0, err
	}

	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		_ = windows.CloseHandle(handle)
		return 0, fmt.Errorf("named pipe listener is closing")
	}
	l.acceptHandle = handle
	l.mu.Unlock()

	return handle, nil
}

func (l *namedPipeListener) clearAcceptHandle(handle windows.Handle) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.acceptHandle == handle {
		l.acceptHandle = 0
	}
}

func (l *namedPipeListener) isClosing() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closing
}

func dialEndpoint(ctx context.Context, endpoint string) (io.ReadWriteCloser, error) {
	path, err := windows.UTF16PtrFromString(endpoint)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error

	for {
		waitTimeout := uint32(50)
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, ctx.Err()
			}
			if remaining < 50*time.Millisecond {
				waitTimeout = uint32(remaining / time.Millisecond)
				if waitTimeout == 0 {
					waitTimeout = 1
				}
			}
		}

		if err := waitNamedPipe(path, waitTimeout); err != nil && err != windows.ERROR_SEM_TIMEOUT {
			lastErr = err
		}

		handle, err := windows.CreateFile(
			path,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil,
			windows.OPEN_EXISTING,
			windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION,
			0,
		)
		if err == nil {
			if handle == 0 || handle == windows.InvalidHandle {
				return nil, fmt.Errorf("dial named pipe %q: createfile returned invalid handle", endpoint)
			}
			mode := uint32(windows.PIPE_READMODE_BYTE)
			if setErr := windows.SetNamedPipeHandleState(handle, &mode, nil, nil); setErr != nil {
				_ = windows.CloseHandle(handle)
				return nil, setErr
			}
			return os.NewFile(uintptr(handle), endpoint), nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("dial named pipe %q: %w", endpoint, lastErr)
			}
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitNamedPipe(path *uint16, timeout uint32) error {
	result, _, err := waitNamedPipeProc.Call(uintptr(unsafe.Pointer(path)), uintptr(timeout))
	if result != 0 {
		return nil
	}
	if err != nil && err != windows.ERROR_SUCCESS {
		return err
	}
	return windows.ERROR_SEM_TIMEOUT
}

func newPipeSecurityAttributes() (*windows.SecurityAttributes, error) {
	descriptor, err := windows.NewSecurityDescriptor()
	if err != nil {
		return nil, err
	}

	// A nil DACL allows local clients through the pipe endpoint while
	// PIPE_REJECT_REMOTE_CLIENTS continues to enforce local-only access.
	if err := descriptor.SetDACL(nil, true, false); err != nil {
		return nil, err
	}

	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}, nil
}
