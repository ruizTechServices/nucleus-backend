package storage

import (
	"context"
	"time"

	"nucleus-backend/internal/session"
)

type ExecutionRecord struct {
	ExecutionID string    `json:"execution_id"`
	SessionID   string    `json:"session_id"`
	ToolName    string    `json:"tool_name"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

type ApprovalRecord struct {
	ApprovalID string    `json:"approval_id"`
	SessionID  string    `json:"session_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at"`
}

type StateStore interface {
	UpsertSession(ctx context.Context, metadata session.Metadata) error
	RecordExecution(ctx context.Context, record ExecutionRecord) error
	RecordApproval(ctx context.Context, record ApprovalRecord) error
}
