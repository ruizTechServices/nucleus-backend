package transport

import (
	"context"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
)

type Handler interface {
	Handle(ctx context.Context, request rpc.Request) (rpc.Response, error)
}

type Listener interface {
	Serve(ctx context.Context, handler Handler) error
	Close() error
	Network() string
}

type GracefulListener interface {
	Listener
	BeginShutdown() error
}
