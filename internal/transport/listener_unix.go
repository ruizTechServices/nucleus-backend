//go:build !windows

package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type unixListener struct {
	endpoint string

	mu       sync.Mutex
	listener net.Listener
	closing  bool
}

func DefaultEndpoint(name string) string {
	return filepath.Join(os.TempDir(), "nucleus", sanitizeName(name)+".sock")
}

func NewLocalListener(endpoint string) (Listener, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return nil, fmt.Errorf("unix socket endpoint is required")
	}

	return &unixListener{endpoint: trimmed}, nil
}

func (l *unixListener) BeginShutdown() error {
	return nil
}

func (l *unixListener) Serve(ctx context.Context, handler Handler) error {
	if handler == nil {
		return fmt.Errorf("transport handler is required")
	}

	if err := prepareUnixSocketPath(l.endpoint); err != nil {
		return err
	}

	listener, err := net.Listen("unix", l.endpoint)
	if err != nil {
		return err
	}

	if err := os.Chmod(l.endpoint, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(l.endpoint)
		return err
	}

	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		_ = listener.Close()
		_ = os.Remove(l.endpoint)
		return nil
	}
	l.listener = listener
	l.mu.Unlock()

	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			_ = l.Close()
		}()
	}

	for {
		connection, err := listener.Accept()
		if err != nil {
			if l.isClosing() || (ctx != nil && ctx.Err() != nil) {
				return nil
			}
			return err
		}

		go handleConnection(connection, handler)
	}
}

func (l *unixListener) Close() error {
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		return nil
	}

	l.closing = true
	listener := l.listener
	l.listener = nil
	endpoint := l.endpoint
	l.mu.Unlock()

	var err error
	if listener != nil {
		err = listener.Close()
	}

	_ = os.Remove(endpoint)
	return err
}

func (l *unixListener) Network() string {
	return "unix"
}

func (l *unixListener) isClosing() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closing
}

func dialEndpoint(ctx context.Context, endpoint string) (io.ReadWriteCloser, error) {
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", endpoint)
}

func prepareUnixSocketPath(endpoint string) error {
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return err
	}

	info, err := os.Lstat(endpoint)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("unix socket endpoint %q already exists and is not a socket", endpoint)
		}

		if err := os.Remove(endpoint); err != nil {
			return err
		}
	}

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
