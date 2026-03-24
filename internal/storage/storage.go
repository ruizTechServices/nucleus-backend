package storage

import (
	"context"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

type ExecutionRecord struct {
	ExecutionID string    `json:"execution_id"`
	SessionID   string    `json:"session_id"`
	ToolName    string    `json:"tool_name"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

type TerminalSessionRecord struct {
	TerminalSessionID string     `json:"terminal_session_id"`
	SessionID         string     `json:"session_id"`
	WorkingDirectory  string     `json:"working_directory,omitempty"`
	ShellProfile      string     `json:"shell_profile,omitempty"`
	Status            string     `json:"status"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
}

type ApprovalRecord struct {
	ApprovalID string    `json:"approval_id"`
	SessionID  string    `json:"session_id"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at"`
}

type ErrorRecord struct {
	ErrorID     string         `json:"error_id"`
	SessionID   string         `json:"session_id,omitempty"`
	ExecutionID string         `json:"execution_id,omitempty"`
	Code        int            `json:"code"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
}

type StateStore interface {
	UpsertSession(ctx context.Context, metadata session.Metadata) error
	RecordTerminalSession(ctx context.Context, record TerminalSessionRecord) error
	RecordExecution(ctx context.Context, record ExecutionRecord) error
	RecordApproval(ctx context.Context, record ApprovalRecord) error
	RecordError(ctx context.Context, record ErrorRecord) error
}
