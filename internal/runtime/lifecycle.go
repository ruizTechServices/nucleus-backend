package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
)

type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

type closeable interface {
	Close() error
}

type gracefulTransport interface {
	BeginShutdown() error
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.shuttingDown = true
	r.mu.Unlock()

	var errs []error

	transportClosed := false
	if graceful, ok := r.dependencies.Transport.(gracefulTransport); ok && graceful != nil {
		if err := graceful.BeginShutdown(); err != nil {
			errs = append(errs, err)
		}
	} else if r.dependencies.Transport != nil {
		if err := r.dependencies.Transport.Close(); err != nil {
			errs = append(errs, err)
		}
		transportClosed = true
	}

	for _, shutdowner := range r.dependencies.Shutdowners {
		if shutdowner == nil {
			continue
		}

		if err := shutdowner.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if err := r.waitForDrain(ctx); err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
	}

	if !transportClosed && r.dependencies.Transport != nil {
		if err := r.dependencies.Transport.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if err := closeIfPossible(r.dependencies.Audit); err != nil {
		errs = append(errs, err)
	}

	if err := closeIfPossible(r.dependencies.Storage); err != nil {
		errs = append(errs, err)
	}

	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()

	return errors.Join(errs...)
}

func (r *Runtime) Close() error {
	return r.Shutdown(context.Background())
}

func (r *Runtime) admitRequest(id any) *rpc.Response {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.shuttingDown || r.closed {
		response := rpc.NewErrorResponse(id, rpc.CodeRuntimeShuttingDown, "runtime is shutting down", nil)
		return &response
	}

	if r.maxConcurrentRequests > 0 && r.inFlight >= r.maxConcurrentRequests {
		response := rpc.NewErrorResponse(id, rpc.CodeResourceExhausted, "runtime concurrency limit reached", map[string]any{
			"limit": r.maxConcurrentRequests,
		})
		return &response
	}

	r.inFlight++
	return nil
}

func (r *Runtime) releaseRequest() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.inFlight > 0 {
		r.inFlight--
	}
}

func (r *Runtime) waitForDrain(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		r.mu.Lock()
		drained := r.inFlight == 0
		r.mu.Unlock()

		if drained {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func closeIfPossible(value any) error {
	closer, ok := value.(closeable)
	if !ok || closer == nil {
		return nil
	}

	return closer.Close()
}
