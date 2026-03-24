package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var ErrTimeout = errors.New("execution timed out")

type EventType string

const (
	EventStarted   EventType = "execution.started"
	EventCompleted EventType = "execution.completed"
	EventFailed    EventType = "execution.failed"
)

type Event struct {
	Type        EventType
	ExecutionID string
	SessionID   string
	ToolName    string
	OccurredAt  time.Time
	Duration    time.Duration
	Metadata    map[string]any
}

type Observer interface {
	Record(ctx context.Context, event Event) error
}

type Config struct {
	DefaultTimeout time.Duration
	Now            func() time.Time
	GenerateID     func() (string, error)
	Observer       Observer
}

type DefaultRunner struct {
	defaultTimeout time.Duration
	now            func() time.Time
	generateID     func() (string, error)
	observer       Observer
}

func NewDefaultRunner(config Config) *DefaultRunner {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	generateID := config.GenerateID
	if generateID == nil {
		generateID = newExecutionID
	}

	defaultTimeout := config.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}

	return &DefaultRunner{
		defaultTimeout: defaultTimeout,
		now:            now,
		generateID:     generateID,
		observer:       config.Observer,
	}
}

func (r *DefaultRunner) Execute(ctx context.Context, request Request) (Result, error) {
	if request.Tool == nil {
		return Result{}, errors.New("tool handler is required")
	}

	executionID, err := r.generateID()
	if err != nil {
		return Result{}, err
	}

	toolName := request.Call.ToolName
	if toolName == "" {
		toolName = request.Tool.Definition().Name
	}

	startedAt := r.now().UTC()
	if err := r.recordEvent(ctx, Event{
		Type:        EventStarted,
		ExecutionID: executionID,
		SessionID:   request.Session.SessionID,
		ToolName:    toolName,
		OccurredAt:  startedAt,
	}); err != nil {
		return Result{}, err
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = r.defaultTimeout
	}

	execCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	if request.Call.Session.SessionID == "" {
		request.Call.Session = request.Session
	}

	toolResult, invokeErr := request.Tool.Invoke(execCtx, request.Call)
	finishedAt := r.now().UTC()
	duration := finishedAt.Sub(startedAt)

	result := Result{
		ExecutionID: executionID,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
	}

	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		if err := r.recordEvent(ctx, Event{
			Type:        EventFailed,
			ExecutionID: executionID,
			SessionID:   request.Session.SessionID,
			ToolName:    toolName,
			OccurredAt:  finishedAt,
			Duration:    duration,
			Metadata: map[string]any{
				"reason": "timeout",
			},
		}); err != nil {
			return result, err
		}

		return result, ErrTimeout
	}

	if invokeErr != nil {
		if err := r.recordEvent(ctx, Event{
			Type:        EventFailed,
			ExecutionID: executionID,
			SessionID:   request.Session.SessionID,
			ToolName:    toolName,
			OccurredAt:  finishedAt,
			Duration:    duration,
			Metadata: map[string]any{
				"reason": invokeErr.Error(),
			},
		}); err != nil {
			return result, err
		}

		return result, invokeErr
	}

	toolResult.ExecutionID = executionID
	if toolResult.ToolName == "" {
		toolResult.ToolName = toolName
	}

	result.ToolResult = toolResult

	if err := r.recordEvent(ctx, Event{
		Type:        EventCompleted,
		ExecutionID: executionID,
		SessionID:   request.Session.SessionID,
		ToolName:    toolName,
		OccurredAt:  finishedAt,
		Duration:    duration,
	}); err != nil {
		return result, err
	}

	return result, nil
}

func (r *DefaultRunner) recordEvent(ctx context.Context, event Event) error {
	if r.observer == nil {
		return nil
	}

	return r.observer.Record(ctx, event)
}

func newExecutionID() (string, error) {
	var payload [12]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}

	return "exec_" + hex.EncodeToString(payload[:]), nil
}
