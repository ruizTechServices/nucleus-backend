package policy

import (
	"context"

	"nucleus-backend/internal/session"
)

type Decision string

const (
	DecisionAllow            Decision = "allow"
	DecisionDeny             Decision = "deny"
	DecisionApprovalRequired Decision = "approval_required"
)

type Input struct {
	Session    session.Metadata
	Action     string
	Resource   string
	Attributes map[string]any
}

type Result struct {
	Decision   Decision
	Reason     string
	ApprovalID string
}

type Engine interface {
	Evaluate(ctx context.Context, input Input) (Result, error)
}
