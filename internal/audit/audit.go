package audit

import (
	"context"
	"time"
)

type Event struct {
	EventID           string         `json:"event_id"`
	EventType         string         `json:"event_type"`
	OccurredAt        time.Time      `json:"occurred_at"`
	SessionID         string         `json:"session_id,omitempty"`
	ExecutionID       string         `json:"execution_id,omitempty"`
	TerminalSessionID string         `json:"terminal_session_id,omitempty"`
	ToolName          string         `json:"tool_name,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
}

type EventSink interface {
	Append(ctx context.Context, event Event) error
}
