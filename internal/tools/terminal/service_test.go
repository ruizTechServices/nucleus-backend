package terminal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type fakeRunner struct {
	run func(context.Context, CommandRequest) (CommandResult, error)
}

func (f fakeRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return f.run(ctx, request)
}

func TestServiceSupportsMultipleCommandsWithinManagedSession(t *testing.T) {
	workingDirectory := t.TempDir()
	var commands []string

	service, err := NewService(Config{
		DefaultWorkingDirectory: workingDirectory,
		GenerateSessionID: func() (string, error) {
			return "term_test_1", nil
		},
		Runner: fakeRunner{
			run: func(_ context.Context, request CommandRequest) (CommandResult, error) {
				commands = append(commands, request.Command)
				return CommandResult{
					Stdout:   "ok:" + request.Command,
					ExitCode: 0,
					Duration: 12 * time.Millisecond,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected terminal service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	startHandler, ok := registry.Lookup(ToolStartSession)
	if !ok {
		t.Fatal("expected terminal.start_session handler to be registered")
	}

	startResult, err := startHandler.Invoke(context.Background(), tools.Call{
		Session:  session.Metadata{SessionID: "sess_test_1"},
		ToolName: ToolStartSession,
		Arguments: map[string]any{
			"working_directory": workingDirectory,
			"shell_profile":     "default",
		},
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session to succeed: %v", err)
	}

	startPayload, ok := startResult.Payload.(StartSessionResponse)
	if !ok {
		t.Fatalf("expected start session payload, got %T", startResult.Payload)
	}

	execHandler, ok := registry.Lookup(ToolExec)
	if !ok {
		t.Fatal("expected terminal.exec handler to be registered")
	}

	for _, command := range []string{"go version", "go env GOROOT"} {
		execResult, err := execHandler.Invoke(context.Background(), tools.Call{
			Session:  session.Metadata{SessionID: "sess_test_1"},
			ToolName: ToolExec,
			Arguments: map[string]any{
				"terminal_session_id": startPayload.TerminalSessionID,
				"command":             command,
				"timeout_ms":          1000,
			},
		})
		if err != nil {
			t.Fatalf("expected terminal.exec for %q to succeed: %v", command, err)
		}

		execPayload, ok := execResult.Payload.(ExecResponse)
		if !ok {
			t.Fatalf("expected exec payload, got %T", execResult.Payload)
		}

		if execPayload.TerminalSessionID != startPayload.TerminalSessionID {
			t.Fatalf("expected shared terminal session id, got %+v", execPayload)
		}
	}

	if len(commands) != 2 {
		t.Fatalf("expected two commands to execute, got %+v", commands)
	}

	endHandler, ok := registry.Lookup(ToolEndSession)
	if !ok {
		t.Fatal("expected terminal.end_session handler to be registered")
	}

	endResult, err := endHandler.Invoke(context.Background(), tools.Call{
		Session:  session.Metadata{SessionID: "sess_test_1"},
		ToolName: ToolEndSession,
		Arguments: map[string]any{
			"terminal_session_id": startPayload.TerminalSessionID,
		},
	})
	if err != nil {
		t.Fatalf("expected terminal.end_session to succeed: %v", err)
	}

	endPayload, ok := endResult.Payload.(EndSessionResponse)
	if !ok {
		t.Fatalf("expected end session payload, got %T", endResult.Payload)
	}

	if endPayload.Status != "ended" {
		t.Fatalf("expected ended status, got %+v", endPayload)
	}
}

func TestServiceEndSessionCancelsActiveCommands(t *testing.T) {
	workingDirectory := t.TempDir()
	commandStarted := make(chan struct{})
	commandCanceled := make(chan struct{})

	service, err := NewService(Config{
		DefaultWorkingDirectory: workingDirectory,
		GenerateSessionID: func() (string, error) {
			return "term_test_cleanup", nil
		},
		Runner: fakeRunner{
			run: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
				close(commandStarted)
				<-ctx.Done()
				close(commandCanceled)
				return CommandResult{}, ctx.Err()
			},
		},
	})
	if err != nil {
		t.Fatalf("expected terminal service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	startHandler, _ := registry.Lookup(ToolStartSession)
	execHandler, _ := registry.Lookup(ToolExec)
	endHandler, _ := registry.Lookup(ToolEndSession)

	startResult, err := startHandler.Invoke(context.Background(), tools.Call{
		Session:  session.Metadata{SessionID: "sess_cleanup"},
		ToolName: ToolStartSession,
		Arguments: map[string]any{
			"working_directory": workingDirectory,
		},
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session to succeed: %v", err)
	}

	startPayload := startResult.Payload.(StartSessionResponse)

	errCh := make(chan error, 1)
	go func() {
		_, err := execHandler.Invoke(context.Background(), tools.Call{
			Session:  session.Metadata{SessionID: "sess_cleanup"},
			ToolName: ToolExec,
			Arguments: map[string]any{
				"terminal_session_id": startPayload.TerminalSessionID,
				"command":             "go version",
			},
		})
		errCh <- err
	}()

	select {
	case <-commandStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal command to start")
	}

	_, err = endHandler.Invoke(context.Background(), tools.Call{
		Session:  session.Metadata{SessionID: "sess_cleanup"},
		ToolName: ToolEndSession,
		Arguments: map[string]any{
			"terminal_session_id": startPayload.TerminalSessionID,
		},
	})
	if err != nil {
		t.Fatalf("expected terminal.end_session to succeed: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected in-flight command to be canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal command goroutine to exit")
	}

	select {
	case <-commandCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal command cleanup to cancel the runner")
	}
}

func TestLocalRunnerCapturesStructuredCommandResult(t *testing.T) {
	workingDirectory := t.TempDir()

	result, err := LocalRunner{}.Run(context.Background(), CommandRequest{
		WorkingDirectory: workingDirectory,
		Command:          "go env GOOS",
		Program:          "go",
		Args:             []string{"env", "GOOS"},
	})
	if err != nil {
		t.Fatalf("expected local runner to execute go env: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %+v", result)
	}

	if result.Stdout == "" {
		t.Fatalf("expected stdout to be captured, got %+v", result)
	}

	if result.Duration <= 0 {
		t.Fatalf("expected positive duration, got %+v", result)
	}

	expectedDirectory := filepath.Clean(workingDirectory)
	if expectedDirectory == "" {
		t.Fatal("expected non-empty working directory")
	}
}

func TestServiceShutdownCancelsActiveCommands(t *testing.T) {
	workingDirectory := t.TempDir()
	commandStarted := make(chan struct{})
	commandCanceled := make(chan struct{})

	service, err := NewService(Config{
		DefaultWorkingDirectory: workingDirectory,
		GenerateSessionID: func() (string, error) {
			return "term_shutdown", nil
		},
		Runner: fakeRunner{
			run: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
				close(commandStarted)
				<-ctx.Done()
				close(commandCanceled)
				return CommandResult{}, ctx.Err()
			},
		},
	})
	if err != nil {
		t.Fatalf("expected terminal service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	startHandler, _ := registry.Lookup(ToolStartSession)
	execHandler, _ := registry.Lookup(ToolExec)

	startResult, err := startHandler.Invoke(context.Background(), tools.Call{
		Session:  session.Metadata{SessionID: "sess_shutdown"},
		ToolName: ToolStartSession,
		Arguments: map[string]any{
			"working_directory": workingDirectory,
		},
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session to succeed: %v", err)
	}

	startPayload := startResult.Payload.(StartSessionResponse)

	errCh := make(chan error, 1)
	go func() {
		_, err := execHandler.Invoke(context.Background(), tools.Call{
			Session:  session.Metadata{SessionID: "sess_shutdown"},
			ToolName: ToolExec,
			Arguments: map[string]any{
				"terminal_session_id": startPayload.TerminalSessionID,
				"command":             "go version",
			},
		})
		errCh <- err
	}()

	select {
	case <-commandStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal command to start")
	}

	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected terminal service shutdown to succeed: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected shutdown to cancel the in-flight command, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal command goroutine to exit")
	}

	select {
	case <-commandCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected terminal shutdown to cancel the runner")
	}

	service.mu.Lock()
	sessionState := service.sessions[startPayload.TerminalSessionID]
	service.mu.Unlock()

	if sessionState == nil || !sessionState.Ended || sessionState.EndedAt == nil {
		t.Fatalf("expected terminal session to be marked ended on shutdown, got %+v", sessionState)
	}
}
