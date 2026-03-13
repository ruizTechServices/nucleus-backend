package executor

import (
	"context"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type Request struct {
	Session session.Metadata
	Tool    tools.Handler
	Call    tools.Call
	Timeout time.Duration
}

type Result struct {
	ExecutionID string
	ToolResult  tools.Result
	StartedAt   time.Time
	FinishedAt  time.Time
}

type Runner interface {
	Execute(ctx context.Context, request Request) (Result, error)
}
