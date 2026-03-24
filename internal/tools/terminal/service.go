package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type Config struct {
	DefaultWorkingDirectory string
	Now                     func() time.Time
	GenerateSessionID       func() (string, error)
	Runner                  CommandRunner
}

type CommandRequest struct {
	WorkingDirectory string
	Command          string
	Program          string
	Args             []string
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

type CommandRunner interface {
	Run(ctx context.Context, request CommandRequest) (CommandResult, error)
}

type Service struct {
	mu                      sync.Mutex
	sessions                map[string]*managedSession
	now                     func() time.Time
	generateSessionID       func() (string, error)
	runner                  CommandRunner
	defaultWorkingDirectory string
	shuttingDown            bool
}

type managedSession struct {
	ID               string
	SessionID        string
	WorkingDirectory string
	ShellProfile     string
	StartedAt        time.Time
	EndedAt          *time.Time
	Ended            bool
	nextActiveID     int
	activeCommands   map[string]context.CancelFunc
}

type LocalRunner struct{}

func NewService(config Config) (*Service, error) {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	generateSessionID := config.GenerateSessionID
	if generateSessionID == nil {
		generateSessionID = newTerminalSessionID
	}

	runner := config.Runner
	if runner == nil {
		runner = LocalRunner{}
	}

	defaultWorkingDirectory, err := normalizeWorkingDirectory(config.DefaultWorkingDirectory)
	if err != nil {
		return nil, err
	}

	return &Service{
		sessions:                make(map[string]*managedSession),
		now:                     now,
		generateSessionID:       generateSessionID,
		runner:                  runner,
		defaultWorkingDirectory: defaultWorkingDirectory,
	}, nil
}

func (s *Service) Entries() []tools.Entry {
	if s == nil {
		return nil
	}

	return []tools.Entry{
		{Handler: startSessionHandler{service: s}},
		{Handler: execHandler{service: s}},
		{Handler: endSessionHandler{service: s}},
	}
}

type startSessionHandler struct {
	service *Service
}

func (h startSessionHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolStartSession,
		Risk:             tools.RiskHigh,
		RequiresApproval: true,
		InputSchema:      StartSessionRequest{},
		OutputSchema:     StartSessionResponse{},
	}
}

func (h startSessionHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[StartSessionRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	workingDirectory, err := h.service.resolveWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return tools.Result{}, tools.NewValidationError(err.Error(), map[string]any{
			"working_directory": request.WorkingDirectory,
		})
	}

	startedAt := h.service.now().UTC()
	terminalSessionID, err := h.service.generateSessionID()
	if err != nil {
		return tools.Result{}, err
	}

	sessionState := &managedSession{
		ID:               terminalSessionID,
		SessionID:        call.Session.SessionID,
		WorkingDirectory: workingDirectory,
		ShellProfile:     strings.TrimSpace(request.ShellProfile),
		StartedAt:        startedAt,
		activeCommands:   make(map[string]context.CancelFunc),
	}

	h.service.mu.Lock()
	if h.service.shuttingDown {
		h.service.mu.Unlock()
		return tools.Result{}, tools.NewDeniedError("terminal service is shutting down", nil)
	}
	h.service.sessions[terminalSessionID] = sessionState
	h.service.mu.Unlock()

	return tools.Result{
		ToolName: ToolStartSession,
		Payload: StartSessionResponse{
			TerminalSessionID: terminalSessionID,
			StartedAt:         startedAt,
			WorkingDirectory:  displayPath(workingDirectory),
		},
		Metadata: map[string]any{
			"shell_profile":     sessionState.ShellProfile,
			"working_directory": displayPath(workingDirectory),
			"started_at":        startedAt.UTC().Format(time.RFC3339Nano),
		},
	}, nil
}

type execHandler struct {
	service *Service
}

func (h execHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolExec,
		Risk:             tools.RiskHigh,
		RequiresApproval: true,
		InputSchema:      ExecRequest{},
		OutputSchema:     ExecResponse{},
	}
}

func (h execHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[ExecRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	if strings.TrimSpace(request.TerminalSessionID) == "" {
		return tools.Result{}, tools.NewValidationError("terminal session id is required", nil)
	}

	if strings.TrimSpace(request.Command) == "" {
		return tools.Result{}, tools.NewValidationError("terminal command is required", map[string]any{
			"terminal_session_id": request.TerminalSessionID,
		})
	}

	if request.TimeoutMS < 0 {
		return tools.Result{}, tools.NewValidationError("terminal timeout must be zero or greater", map[string]any{
			"terminal_session_id": request.TerminalSessionID,
			"timeout_ms":          request.TimeoutMS,
		})
	}

	commandTokens, err := parseCommand(request.Command)
	if err != nil {
		return tools.Result{}, tools.NewValidationError(err.Error(), map[string]any{
			"terminal_session_id": request.TerminalSessionID,
			"command":             request.Command,
		})
	}

	if err := validateExecutable(commandTokens); err != nil {
		return tools.Result{}, tools.NewValidationError(err.Error(), map[string]any{
			"terminal_session_id": request.TerminalSessionID,
			"command":             request.Command,
		})
	}

	execCtx, execCancel := context.WithCancel(ctx)
	sessionState, activeID, err := h.service.beginExecution(call.Session.SessionID, request.TerminalSessionID, execCancel)
	if err != nil {
		execCancel()
		return tools.Result{}, err
	}

	defer execCancel()
	defer h.service.unregisterActiveCommand(sessionState.ID, activeID)

	commandResult, err := h.service.runner.Run(execCtx, CommandRequest{
		WorkingDirectory: sessionState.WorkingDirectory,
		Command:          request.Command,
		Program:          commandTokens[0],
		Args:             commandTokens[1:],
	})
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{
		ToolName: ToolExec,
		Payload: ExecResponse{
			TerminalSessionID: request.TerminalSessionID,
			Command:           request.Command,
			Stdout:            commandResult.Stdout,
			Stderr:            commandResult.Stderr,
			ExitCode:          commandResult.ExitCode,
			DurationMS:        commandResult.Duration.Milliseconds(),
		},
		Metadata: map[string]any{
			"working_directory": displayPath(sessionState.WorkingDirectory),
		},
	}, nil
}

type endSessionHandler struct {
	service *Service
}

func (h endSessionHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolEndSession,
		Risk:             tools.RiskMedium,
		RequiresApproval: true,
		InputSchema:      EndSessionRequest{},
		OutputSchema:     EndSessionResponse{},
	}
}

func (h endSessionHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[EndSessionRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	if strings.TrimSpace(request.TerminalSessionID) == "" {
		return tools.Result{}, tools.NewValidationError("terminal session id is required", nil)
	}

	sessionState, err := h.service.endSession(call.Session.SessionID, request.TerminalSessionID)
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{
		ToolName: ToolEndSession,
		Payload: EndSessionResponse{
			TerminalSessionID: request.TerminalSessionID,
			Status:            "ended",
		},
		Metadata: map[string]any{
			"working_directory": displayPath(sessionState.WorkingDirectory),
			"shell_profile":     sessionState.ShellProfile,
			"started_at":        sessionState.StartedAt.UTC().Format(time.RFC3339Nano),
			"ended_at":          sessionState.EndedAt.Format(time.RFC3339Nano),
		},
	}, nil
}

func (s *Service) resolveWorkingDirectory(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return s.defaultWorkingDirectory, nil
	}

	return normalizeWorkingDirectory(raw)
}

func normalizeWorkingDirectory(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", err
		}

		return filepath.Clean(workingDirectory), nil
	}

	if strings.ContainsRune(trimmed, rune(0)) {
		return "", fmt.Errorf("terminal working directory contains invalid null byte")
	}

	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("terminal working directory must be absolute")
	}

	workingDirectory, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(workingDirectory)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return "", fmt.Errorf("terminal working directory must be a directory")
	}

	return workingDirectory, nil
}

func (s *Service) beginExecution(ownerSessionID string, terminalSessionID string, cancel context.CancelFunc) (*managedSession, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionState, ok := s.sessions[terminalSessionID]
	if !ok {
		return nil, "", tools.NewNotFoundError("terminal session not found", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	if sessionState.SessionID != ownerSessionID {
		return nil, "", tools.NewDeniedError("terminal session does not belong to the current session", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	if sessionState.Ended {
		return nil, "", tools.NewValidationError("terminal session is not active", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	if s.shuttingDown {
		return nil, "", tools.NewDeniedError("terminal service is shutting down", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	sessionState.nextActiveID++
	activeID := fmt.Sprintf("active_%d", sessionState.nextActiveID)
	sessionState.activeCommands[activeID] = cancel
	return sessionState, activeID, nil
}

func (s *Service) unregisterActiveCommand(terminalSessionID string, activeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionState, ok := s.sessions[terminalSessionID]; ok {
		delete(sessionState.activeCommands, activeID)
	}
}

func (s *Service) endSession(ownerSessionID string, terminalSessionID string) (*managedSession, error) {
	s.mu.Lock()
	sessionState, ok := s.sessions[terminalSessionID]
	if !ok {
		s.mu.Unlock()
		return nil, tools.NewNotFoundError("terminal session not found", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	if sessionState.SessionID != ownerSessionID {
		s.mu.Unlock()
		return nil, tools.NewDeniedError("terminal session does not belong to the current session", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	if sessionState.Ended {
		s.mu.Unlock()
		return nil, tools.NewValidationError("terminal session is not active", map[string]any{
			"terminal_session_id": terminalSessionID,
		})
	}

	endedAt := s.now().UTC()
	sessionState.Ended = true
	sessionState.EndedAt = &endedAt

	cancels := make([]context.CancelFunc, 0, len(sessionState.activeCommands))
	for _, cancel := range sessionState.activeCommands {
		cancels = append(cancels, cancel)
	}
	sessionState.activeCommands = map[string]context.CancelFunc{}
	s.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}

	return sessionState, nil
}

func (LocalRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	startedAt := time.Now()
	cmd := exec.CommandContext(ctx, request.Program, request.Args...)
	cmd.Dir = request.WorkingDirectory

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: time.Since(startedAt),
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}

		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return result, context.Canceled
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}

		result.ExitCode = -1
		result.Stderr = strings.TrimSpace(strings.TrimSpace(result.Stderr) + "\n" + err.Error())
		return result, nil
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	return result, nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	s.shuttingDown = true

	endedAt := s.now().UTC()
	cancels := make([]context.CancelFunc, 0)
	for _, sessionState := range s.sessions {
		if !sessionState.Ended {
			sessionState.Ended = true
			sessionState.EndedAt = &endedAt
		}

		for _, cancel := range sessionState.activeCommands {
			cancels = append(cancels, cancel)
		}
	}
	s.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		activeCommands := 0
		for _, sessionState := range s.sessions {
			activeCommands += len(sessionState.activeCommands)
		}
		s.mu.Unlock()

		if activeCommands == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func parseCommand(raw string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune

	for _, ch := range strings.TrimSpace(raw) {
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
				continue
			}
			current.WriteRune(ch)
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == ' ' || ch == '\t':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("terminal command contains an unterminated quote")
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("terminal command is required")
	}

	return tokens, nil
}

func validateExecutable(tokens []string) error {
	executable := strings.ToLower(filepath.Base(tokens[0]))
	switch executable {
	case "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe", "sh", "bash", "zsh":
		return fmt.Errorf("terminal shell wrappers are not allowed by default")
	}

	return nil
}

func decodeArguments[T any](arguments map[string]any) (T, error) {
	if arguments == nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments are required", nil)
	}

	payload, err := json.Marshal(arguments)
	if err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	return decoded, nil
}

func displayPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	return filepath.ToSlash(filepath.Clean(path))
}

func newTerminalSessionID() (string, error) {
	return fmt.Sprintf("term_%d", time.Now().UTC().UnixNano()), nil
}
