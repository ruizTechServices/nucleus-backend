package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/desktop"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/screenshot"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/terminal"
)

const (
	methodRuntimeHealth     = "runtime.health"
	methodSessionBootstrap  = "session.bootstrap"
	methodSessionStatus     = "session.status"
	methodToolsList         = "tools.list"
	methodToolsCall         = "tools.call"
	methodTerminalStart     = terminal.ToolStartSession
	methodTerminalExec      = terminal.ToolExec
	methodTerminalEnd       = terminal.ToolEndSession
	methodScreenshotCapture = screenshot.ToolCapture
	methodDesktopGetState   = desktop.ToolGetState
)

type HealthResult struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

type sessionTokenParams struct {
	SessionToken string `json:"session_token"`
}

type toolCallParams struct {
	SessionToken string         `json:"session_token"`
	ToolName     string         `json:"tool_name"`
	Arguments    map[string]any `json:"arguments"`
}

type terminalStartSessionParams struct {
	SessionToken     string `json:"session_token"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	ShellProfile     string `json:"shell_profile,omitempty"`
}

type terminalExecParams struct {
	SessionToken      string `json:"session_token"`
	TerminalSessionID string `json:"terminal_session_id"`
	Command           string `json:"command"`
	TimeoutMS         int    `json:"timeout_ms"`
}

type terminalEndSessionParams struct {
	SessionToken      string `json:"session_token"`
	TerminalSessionID string `json:"terminal_session_id"`
}

type screenshotCaptureParams struct {
	SessionToken string `json:"session_token"`
	DisplayID    string `json:"display_id,omitempty"`
}

type desktopGetStateParams struct {
	SessionToken string `json:"session_token"`
}

type toolsListResult struct {
	Tools []tools.Definition `json:"tools"`
}

type toolsCallResult struct {
	ToolName    string         `json:"tool_name"`
	ExecutionID string         `json:"execution_id"`
	Result      any            `json:"result,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (r *Runtime) Handle(ctx context.Context, request rpc.Request) (rpc.Response, error) {
	if response := r.admitRequest(request.ID); response != nil {
		return *response, nil
	}
	defer r.releaseRequest()

	if request.JSONRPC != rpc.Version {
		return rpc.NewErrorResponse(request.ID, rpc.CodeInvalidRequest, "invalid JSON-RPC version", map[string]any{
			"expected": rpc.Version,
			"actual":   request.JSONRPC,
		}), nil
	}

	if strings.TrimSpace(request.Method) == "" {
		return rpc.NewErrorResponse(request.ID, rpc.CodeInvalidRequest, "missing JSON-RPC method", nil), nil
	}

	switch request.Method {
	case methodRuntimeHealth:
		return r.handleRuntimeHealth(request.ID), nil
	case methodSessionBootstrap:
		return r.handleSessionBootstrap(ctx, request), nil
	case methodSessionStatus:
		return r.handleSessionStatus(ctx, request), nil
	default:
		if namespaceRequiresSession(request.Method) {
			metadata, response := r.authenticateRequest(ctx, request)
			if response != nil {
				return *response, nil
			}

			if response := r.authorizeRequest(ctx, request, metadata); response != nil {
				return *response, nil
			}

			switch request.Method {
			case methodToolsList:
				return r.handleToolsList(ctx, request.ID, metadata), nil
			case methodToolsCall:
				return r.handleToolsCall(ctx, request.ID, metadata, request), nil
			case methodTerminalStart:
				return r.handleTerminalStartSession(ctx, request.ID, metadata, request), nil
			case methodTerminalExec:
				return r.handleTerminalExec(ctx, request.ID, metadata, request), nil
			case methodTerminalEnd:
				return r.handleTerminalEndSession(ctx, request.ID, metadata, request), nil
			case methodScreenshotCapture:
				return r.handleScreenshotCapture(ctx, request.ID, metadata, request), nil
			case methodDesktopGetState:
				return r.handleDesktopGetState(ctx, request.ID, metadata, request), nil
			}
		}

		return rpc.NewErrorResponse(request.ID, rpc.CodeMethodNotFound, "method not found", map[string]any{
			"method": request.Method,
		}), nil
	}
}

func (r *Runtime) handleRuntimeHealth(id any) rpc.Response {
	return rpc.NewResultResponse(id, HealthResult{
		Service: r.buildInfo.Service,
		Status:  "ok",
		Version: r.buildInfo.Version,
	})
}

func (r *Runtime) handleSessionBootstrap(ctx context.Context, request rpc.Request) rpc.Response {
	if r.dependencies.Sessions == nil {
		return rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "session service unavailable", nil)
	}

	params, ok := decodeParams[session.BootstrapRequest](request)
	if !ok {
		return rpc.NewErrorResponse(request.ID, rpc.CodeValidationError, "invalid session.bootstrap params", nil)
	}

	metadata, err := r.dependencies.Sessions.Bootstrap(ctx, params)
	if err != nil {
		if errors.Is(err, session.ErrInvalidBootstrapToken) {
			return rpc.NewErrorResponse(request.ID, rpc.CodeInvalidBootstrapToken, "invalid bootstrap token", nil)
		}

		return rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "failed to bootstrap session", map[string]any{
			"reason": err.Error(),
		})
	}

	if response := r.persistSessionBootstrap(ctx, request.ID, metadata); response != nil {
		return *response
	}

	return rpc.NewResultResponse(request.ID, metadata)
}

func (r *Runtime) handleSessionStatus(ctx context.Context, request rpc.Request) rpc.Response {
	metadata, response := r.authenticateRequest(ctx, request)
	if response != nil {
		return *response
	}

	return rpc.NewResultResponse(request.ID, metadata)
}

func (r *Runtime) handleToolsList(ctx context.Context, id any, metadata session.Metadata) rpc.Response {
	startedAt := time.Now().UTC()

	var definitions []tools.Definition
	if r.dependencies.Registry != nil {
		definitions = r.dependencies.Registry.List()
	}

	finishedAt := time.Now().UTC()
	if response := r.persistOperationalExecution(ctx, id, metadata, methodToolsList, startedAt, finishedAt, map[string]any{
		"tool_count": len(definitions),
	}); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, toolsListResult{
		Tools: definitions,
	})
}

func (r *Runtime) handleToolsCall(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	if r.dependencies.Registry == nil {
		return rpc.NewErrorResponse(id, rpc.CodeInternalError, "tool registry unavailable", nil)
	}

	if r.dependencies.Executor == nil {
		return rpc.NewErrorResponse(id, rpc.CodeInternalError, "executor unavailable", nil)
	}

	params, ok := decodeParams[toolCallParams](request)
	if !ok || strings.TrimSpace(params.ToolName) == "" {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid tools.call params", nil)
	}

	handler, found := r.dependencies.Registry.Lookup(params.ToolName)
	if !found {
		return rpc.NewErrorResponse(id, rpc.CodeToolNotFound, "tool not found", map[string]any{
			"tool_name": params.ToolName,
		})
	}

	arguments := params.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	result, err := r.dependencies.Executor.Execute(ctx, executor.Request{
		Session: metadata,
		Tool:    handler,
		Call: tools.Call{
			Session:   metadata,
			ToolName:  params.ToolName,
			Arguments: arguments,
		},
		Timeout: requestTimeout(arguments),
	})
	if err != nil {
		var callErr *tools.CallError
		if errors.As(err, &callErr) {
			code := codeForCallError(callErr)

			if response := r.persistExecutionFailure(ctx, id, metadata, params.ToolName, result.ExecutionID, result.StartedAt, result.FinishedAt, code, callErr.Message, callErr.Details); response != nil {
				return *response
			}

			return rpc.NewErrorResponse(id, code, callErr.Message, callErr.Details)
		}

		if errors.Is(err, executor.ErrTimeout) {
			if response := r.persistExecutionFailure(ctx, id, metadata, params.ToolName, result.ExecutionID, result.StartedAt, result.FinishedAt, rpc.CodeExecutionTimeout, "tool execution timed out", nil); response != nil {
				return *response
			}

			return rpc.NewErrorResponse(id, rpc.CodeExecutionTimeout, "tool execution timed out", map[string]any{
				"tool_name": params.ToolName,
			})
		}

		if response := r.persistExecutionFailure(ctx, id, metadata, params.ToolName, result.ExecutionID, result.StartedAt, result.FinishedAt, rpc.CodeExecutionFailed, err.Error(), nil); response != nil {
			return *response
		}

		return rpc.NewErrorResponse(id, rpc.CodeExecutionFailed, "tool execution failed", map[string]any{
			"tool_name": params.ToolName,
			"reason":    err.Error(),
		})
	}

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, toolsCallResult{
		ToolName:    result.ToolResult.ToolName,
		ExecutionID: result.ExecutionID,
		Result:      result.ToolResult.Payload,
		Metadata:    result.ToolResult.Metadata,
	})
}

func (r *Runtime) handleTerminalStartSession(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	params, ok := decodeParams[terminalStartSessionParams](request)
	if !ok {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid terminal.start_session params", nil)
	}

	result, response, err := r.executeMethodTool(ctx, id, metadata, methodTerminalStart, map[string]any{
		"working_directory": params.WorkingDirectory,
		"shell_profile":     params.ShellProfile,
	}, 0, "terminal service unavailable")
	if response != nil {
		return *response
	}

	if err != nil {
		return r.methodToolErrorResponse(ctx, id, metadata, methodTerminalStart, result, err, map[string]any{
			"working_directory": params.WorkingDirectory,
		}, "terminal execution failed")
	}

	payload, ok := result.ToolResult.Payload.(terminal.StartSessionResponse)
	if !ok {
		return rpc.NewErrorResponse(id, rpc.CodeInternalError, "invalid terminal.start_session result payload", nil)
	}

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	if response := r.persistTerminalSessionStarted(ctx, id, metadata.SessionID, payload, result.ToolResult.Metadata); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, payload)
}

func (r *Runtime) handleTerminalExec(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	params, ok := decodeParams[terminalExecParams](request)
	if !ok || strings.TrimSpace(params.TerminalSessionID) == "" {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid terminal.exec params", nil)
	}

	result, response, err := r.executeMethodTool(ctx, id, metadata, methodTerminalExec, map[string]any{
		"terminal_session_id": params.TerminalSessionID,
		"command":             params.Command,
		"timeout_ms":          params.TimeoutMS,
	}, time.Duration(params.TimeoutMS)*time.Millisecond, "terminal service unavailable")
	if response != nil {
		return *response
	}

	if err != nil {
		if errors.Is(err, executor.ErrTimeout) {
			details := map[string]any{
				"terminal_session_id": params.TerminalSessionID,
				"command":             params.Command,
				"timeout_ms":          params.TimeoutMS,
			}

			if response := r.persistExecutionFailure(ctx, id, metadata, methodTerminalExec, result.ExecutionID, result.StartedAt, result.FinishedAt, rpc.CodeExecutionTimeout, "terminal command timed out", details); response != nil {
				return *response
			}

			return rpc.NewErrorResponse(id, rpc.CodeExecutionTimeout, "terminal command timed out", details)
		}

		return r.methodToolErrorResponse(ctx, id, metadata, methodTerminalExec, result, err, map[string]any{
			"terminal_session_id": params.TerminalSessionID,
			"command":             params.Command,
			"timeout_ms":          params.TimeoutMS,
		}, "terminal execution failed")
	}

	payload, ok := result.ToolResult.Payload.(terminal.ExecResponse)
	if !ok {
		return rpc.NewErrorResponse(id, rpc.CodeInternalError, "invalid terminal.exec result payload", nil)
	}

	payload.ExecutionID = result.ExecutionID
	result.ToolResult.Payload = payload

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, payload)
}

func (r *Runtime) handleTerminalEndSession(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	params, ok := decodeParams[terminalEndSessionParams](request)
	if !ok || strings.TrimSpace(params.TerminalSessionID) == "" {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid terminal.end_session params", nil)
	}

	result, response, err := r.executeMethodTool(ctx, id, metadata, methodTerminalEnd, map[string]any{
		"terminal_session_id": params.TerminalSessionID,
	}, 0, "terminal service unavailable")
	if response != nil {
		return *response
	}

	if err != nil {
		return r.methodToolErrorResponse(ctx, id, metadata, methodTerminalEnd, result, err, map[string]any{
			"terminal_session_id": params.TerminalSessionID,
		}, "terminal execution failed")
	}

	payload, ok := result.ToolResult.Payload.(terminal.EndSessionResponse)
	if !ok {
		return rpc.NewErrorResponse(id, rpc.CodeInternalError, "invalid terminal.end_session result payload", nil)
	}

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	if response := r.persistTerminalSessionEnded(ctx, id, metadata.SessionID, params.TerminalSessionID, result.ToolResult.Metadata); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, payload)
}

func (r *Runtime) handleScreenshotCapture(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	params, ok := decodeParams[screenshotCaptureParams](request)
	if !ok {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid screenshot.capture params", nil)
	}

	result, response, err := r.executeMethodTool(ctx, id, metadata, methodScreenshotCapture, map[string]any{
		"display_id": params.DisplayID,
	}, 0, "screenshot service unavailable")
	if response != nil {
		return *response
	}

	if err != nil {
		return r.methodToolErrorResponse(ctx, id, metadata, methodScreenshotCapture, result, err, map[string]any{
			"display_id": params.DisplayID,
		}, "screenshot capture failed")
	}

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, toolsCallResult{
		ToolName:    result.ToolResult.ToolName,
		ExecutionID: result.ExecutionID,
		Result:      result.ToolResult.Payload,
		Metadata:    result.ToolResult.Metadata,
	})
}

func (r *Runtime) handleDesktopGetState(ctx context.Context, id any, metadata session.Metadata, request rpc.Request) rpc.Response {
	params, ok := decodeParams[desktopGetStateParams](request)
	if !ok || strings.TrimSpace(params.SessionToken) == "" {
		return rpc.NewErrorResponse(id, rpc.CodeValidationError, "invalid desktop.get_state params", nil)
	}

	result, response, err := r.executeMethodTool(ctx, id, metadata, methodDesktopGetState, map[string]any{}, 0, "desktop service unavailable")
	if response != nil {
		return *response
	}

	if err != nil {
		return r.methodToolErrorResponse(ctx, id, metadata, methodDesktopGetState, result, err, nil, "desktop state retrieval failed")
	}

	if response := r.persistExecutorExecution(ctx, id, metadata, result); response != nil {
		return *response
	}

	return rpc.NewResultResponse(id, toolsCallResult{
		ToolName:    result.ToolResult.ToolName,
		ExecutionID: result.ExecutionID,
		Result:      result.ToolResult.Payload,
		Metadata:    result.ToolResult.Metadata,
	})
}

func (r *Runtime) executeMethodTool(ctx context.Context, id any, metadata session.Metadata, toolName string, arguments map[string]any, timeout time.Duration, unavailableMessage string) (executor.Result, *rpc.Response, error) {
	if r.dependencies.Registry == nil {
		response := rpc.NewErrorResponse(id, rpc.CodeInternalError, "tool registry unavailable", nil)
		return executor.Result{}, &response, nil
	}

	if r.dependencies.Executor == nil {
		response := rpc.NewErrorResponse(id, rpc.CodeInternalError, "executor unavailable", nil)
		return executor.Result{}, &response, nil
	}

	handler, found := r.dependencies.Registry.Lookup(toolName)
	if !found {
		response := rpc.NewErrorResponse(id, rpc.CodeInternalError, unavailableMessage, map[string]any{
			"tool_name": toolName,
		})
		return executor.Result{}, &response, nil
	}

	result, err := r.dependencies.Executor.Execute(ctx, executor.Request{
		Session: metadata,
		Tool:    handler,
		Call: tools.Call{
			Session:   metadata,
			ToolName:  toolName,
			Arguments: arguments,
		},
		Timeout: timeout,
	})

	return result, nil, err
}

func (r *Runtime) methodToolErrorResponse(ctx context.Context, id any, metadata session.Metadata, toolName string, result executor.Result, err error, details map[string]any, fallbackMessage string) rpc.Response {
	var callErr *tools.CallError
	if errors.As(err, &callErr) {
		code := codeForCallError(callErr)
		mergedDetails := mergeDetails(details, callErr.Details)

		if response := r.persistExecutionFailure(ctx, id, metadata, toolName, result.ExecutionID, result.StartedAt, result.FinishedAt, code, callErr.Message, mergedDetails); response != nil {
			return *response
		}

		return rpc.NewErrorResponse(id, code, callErr.Message, mergedDetails)
	}

	if response := r.persistExecutionFailure(ctx, id, metadata, toolName, result.ExecutionID, result.StartedAt, result.FinishedAt, rpc.CodeExecutionFailed, err.Error(), details); response != nil {
		return *response
	}

	return rpc.NewErrorResponse(id, rpc.CodeExecutionFailed, fallbackMessage, mergeDetails(details, map[string]any{
		"reason": err.Error(),
	}))
}

func codeForCallError(callErr *tools.CallError) int {
	switch callErr.Kind {
	case tools.ErrorKindValidation:
		return rpc.CodeValidationError
	case tools.ErrorKindDenied:
		return rpc.CodePolicyDenied
	case tools.ErrorKindNotFound:
		return rpc.CodeTerminalSessionNotFound
	default:
		return rpc.CodeExecutionFailed
	}
}

func mergeDetails(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}

	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}

	for key, value := range extra {
		merged[key] = value
	}

	return merged
}

func (r *Runtime) authenticateRequest(ctx context.Context, request rpc.Request) (session.Metadata, *rpc.Response) {
	if r.dependencies.Sessions == nil {
		response := rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "session service unavailable", nil)
		return session.Metadata{}, &response
	}

	params, ok := decodeParams[sessionTokenParams](request)
	if !ok || strings.TrimSpace(params.SessionToken) == "" {
		response := rpc.NewErrorResponse(request.ID, rpc.CodeInvalidSessionToken, "missing or invalid session token", nil)
		return session.Metadata{}, &response
	}

	metadata, err := r.dependencies.Sessions.Validate(ctx, params.SessionToken)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrInvalidSessionToken):
			response := rpc.NewErrorResponse(request.ID, rpc.CodeInvalidSessionToken, "invalid session token", nil)
			return session.Metadata{}, &response
		case errors.Is(err, session.ErrSessionExpired):
			response := rpc.NewErrorResponse(request.ID, rpc.CodeSessionExpired, "session expired", nil)
			return session.Metadata{}, &response
		default:
			response := rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "failed to validate session", map[string]any{
				"reason": err.Error(),
			})
			return session.Metadata{}, &response
		}
	}

	return metadata, nil
}

func decodeParams[T any](request rpc.Request) (T, bool) {
	params, err := rpc.DecodeParams[T](request.Params)
	if err != nil {
		var zero T
		return zero, false
	}

	return params, true
}

func namespaceRequiresSession(method string) bool {
	return strings.HasPrefix(method, "tools.") ||
		strings.HasPrefix(method, "terminal.") ||
		strings.HasPrefix(method, "screenshot.") ||
		strings.HasPrefix(method, "desktop.") ||
		strings.HasPrefix(method, "logs.") ||
		strings.HasPrefix(method, "approvals.")
}

func requestTimeout(arguments map[string]any) time.Duration {
	timeoutMS, ok := arguments["timeout_ms"]
	if !ok {
		return 0
	}

	switch typed := timeoutMS.(type) {
	case int:
		return time.Duration(typed) * time.Millisecond
	case int32:
		return time.Duration(typed) * time.Millisecond
	case int64:
		return time.Duration(typed) * time.Millisecond
	case float64:
		return time.Duration(int(typed)) * time.Millisecond
	default:
		return 0
	}
}
