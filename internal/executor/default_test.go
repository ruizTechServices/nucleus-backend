package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type memoryObserver struct {
	events []Event
}

func (m *memoryObserver) Record(_ context.Context, event Event) error {
	m.events = append(m.events, event)
	return nil
}

type successHandler struct{}

func (successHandler) Definition() tools.Definition {
	return tools.Definition{Name: "filesystem.read"}
}

func (successHandler) Invoke(_ context.Context, _ tools.Call) (tools.Result, error) {
	return tools.Result{
		Payload: map[string]any{
			"path": "C:/allowed/file.txt",
		},
	}, nil
}

type timeoutHandler struct{}

func (timeoutHandler) Definition() tools.Definition {
	return tools.Definition{Name: "terminal.exec"}
}

func (timeoutHandler) Invoke(ctx context.Context, _ tools.Call) (tools.Result, error) {
	<-ctx.Done()
	return tools.Result{}, ctx.Err()
}

func TestDefaultRunnerExecutesToolAndEmitsLifecycleEvents(t *testing.T) {
	observer := &memoryObserver{}
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	runner := NewDefaultRunner(Config{
		Observer: observer,
		Now: func() time.Time {
			current := now
			now = now.Add(time.Second)
			return current
		},
		GenerateID: func() (string, error) {
			return "exec_test", nil
		},
	})

	result, err := runner.Execute(context.Background(), Request{
		Session: session.Metadata{SessionID: "sess_1"},
		Tool:    successHandler{},
		Call: tools.Call{
			ToolName: "filesystem.read",
		},
	})
	if err != nil {
		t.Fatalf("expected execution to succeed: %v", err)
	}

	if result.ExecutionID != "exec_test" || result.ToolResult.ExecutionID != "exec_test" {
		t.Fatalf("expected stable execution id, got %+v", result)
	}

	if len(observer.events) != 2 || observer.events[0].Type != EventStarted || observer.events[1].Type != EventCompleted {
		t.Fatalf("unexpected executor lifecycle events: %+v", observer.events)
	}
}

func TestDefaultRunnerReturnsTimeoutAndFailureEvent(t *testing.T) {
	observer := &memoryObserver{}
	runner := NewDefaultRunner(Config{
		Observer:       observer,
		DefaultTimeout: 10 * time.Millisecond,
		GenerateID: func() (string, error) {
			return "exec_timeout", nil
		},
	})

	_, err := runner.Execute(context.Background(), Request{
		Session: session.Metadata{SessionID: "sess_1"},
		Tool:    timeoutHandler{},
		Call: tools.Call{
			ToolName: "terminal.exec",
		},
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}

	if len(observer.events) != 2 || observer.events[1].Type != EventFailed {
		t.Fatalf("expected failed lifecycle event after timeout, got %+v", observer.events)
	}
}
