package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/audit"
	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/storage"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/terminal"
)

func (r *Runtime) persistSessionBootstrap(ctx context.Context, requestID any, metadata session.Metadata) *rpc.Response {
	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.UpsertSession(ctx, metadata); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist session state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:    mustNewID("evt"),
			EventType:  "session.bootstrapped",
			OccurredAt: time.Now().UTC(),
			SessionID:  metadata.SessionID,
			Payload: map[string]any{
				"client_name":    metadata.ClientName,
				"client_version": metadata.ClientVersion,
				"expires_at":     metadata.ExpiresAt.Format(time.RFC3339Nano),
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func (r *Runtime) persistOperationalExecution(ctx context.Context, requestID any, sessionMetadata session.Metadata, toolName string, startedAt, finishedAt time.Time, payload map[string]any) *rpc.Response {
	executionID := mustNewID("exec")

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:     mustNewID("evt"),
			EventType:   "tool.requested",
			OccurredAt:  startedAt.UTC(),
			SessionID:   sessionMetadata.SessionID,
			ExecutionID: executionID,
			ToolName:    toolName,
			Payload:     payload,
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordExecution(ctx, storage.ExecutionRecord{
			ExecutionID: executionID,
			SessionID:   sessionMetadata.SessionID,
			ToolName:    toolName,
			Status:      "succeeded",
			StartedAt:   startedAt.UTC(),
			FinishedAt:  finishedAt.UTC(),
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist execution state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:     mustNewID("evt"),
			EventType:   "tool.completed",
			OccurredAt:  finishedAt.UTC(),
			SessionID:   sessionMetadata.SessionID,
			ExecutionID: executionID,
			ToolName:    toolName,
			Payload: map[string]any{
				"status": "succeeded",
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func (r *Runtime) persistExecutorExecution(ctx context.Context, requestID any, sessionMetadata session.Metadata, result executor.Result) *rpc.Response {
	if response := r.appendToolAuditEvent(ctx, requestID, "tool.requested", sessionMetadata.SessionID, result.ExecutionID, result.ToolResult.ToolName, result.StartedAt, map[string]any{
		"status": "started",
	}); response != nil {
		return response
	}

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordExecution(ctx, storage.ExecutionRecord{
			ExecutionID: result.ExecutionID,
			SessionID:   sessionMetadata.SessionID,
			ToolName:    result.ToolResult.ToolName,
			Status:      "succeeded",
			StartedAt:   result.StartedAt.UTC(),
			FinishedAt:  result.FinishedAt.UTC(),
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist execution state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return r.appendToolAuditEvent(ctx, requestID, "tool.completed", sessionMetadata.SessionID, result.ExecutionID, result.ToolResult.ToolName, result.FinishedAt, map[string]any{
		"status": "succeeded",
	})
}

func (r *Runtime) persistExecutionFailure(ctx context.Context, requestID any, sessionMetadata session.Metadata, toolName string, executionID string, startedAt, finishedAt time.Time, code int, message string, details map[string]any) *rpc.Response {
	if executionID == "" {
		executionID = mustNewID("exec")
	}

	if response := r.appendToolAuditEvent(ctx, requestID, "tool.requested", sessionMetadata.SessionID, executionID, toolName, startedAt, map[string]any{
		"status": "started",
	}); response != nil {
		return response
	}

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordExecution(ctx, storage.ExecutionRecord{
			ExecutionID: executionID,
			SessionID:   sessionMetadata.SessionID,
			ToolName:    toolName,
			Status:      "failed",
			StartedAt:   startedAt.UTC(),
			FinishedAt:  finishedAt.UTC(),
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist execution state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Storage != nil {
		errorDetails := map[string]any{
			"tool_name": toolName,
		}

		for key, value := range details {
			errorDetails[key] = value
		}

		if err := r.dependencies.Storage.RecordError(ctx, storage.ErrorRecord{
			ErrorID:     mustNewID("err"),
			SessionID:   sessionMetadata.SessionID,
			ExecutionID: executionID,
			Code:        code,
			Message:     message,
			Details:     errorDetails,
			OccurredAt:  finishedAt.UTC(),
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist execution error", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	auditPayload := map[string]any{
		"status": "failed",
		"reason": message,
	}

	for key, value := range details {
		auditPayload[key] = value
	}

	return r.appendToolAuditEvent(ctx, requestID, "tool.failed", sessionMetadata.SessionID, executionID, toolName, finishedAt, auditPayload)
}

func (r *Runtime) persistPolicyDenied(ctx context.Context, requestID any, sessionMetadata session.Metadata, action string, reason string) *rpc.Response {
	occurredAt := time.Now().UTC()

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordError(ctx, storage.ErrorRecord{
			ErrorID:    mustNewID("err"),
			SessionID:  sessionMetadata.SessionID,
			Code:       rpc.CodePolicyDenied,
			Message:    "policy denied request",
			Details:    map[string]any{"action": action, "reason": reason},
			OccurredAt: occurredAt,
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist policy denial", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:    mustNewID("evt"),
			EventType:  "policy.denied",
			OccurredAt: occurredAt,
			SessionID:  sessionMetadata.SessionID,
			ToolName:   action,
			Payload: map[string]any{
				"reason": reason,
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func (r *Runtime) persistApprovalRequired(ctx context.Context, requestID any, sessionMetadata session.Metadata, action string, result policy.Result) *rpc.Response {
	createdAt := time.Now().UTC()

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordApproval(ctx, storage.ApprovalRecord{
			ApprovalID: result.ApprovalID,
			SessionID:  sessionMetadata.SessionID,
			Decision:   string(policy.DecisionApprovalRequired),
			Reason:     result.Reason,
			CreatedAt:  createdAt,
			ResolvedAt: createdAt,
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist approval record", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:    mustNewID("evt"),
			EventType:  "approval.required",
			OccurredAt: createdAt,
			SessionID:  sessionMetadata.SessionID,
			ToolName:   action,
			Payload: map[string]any{
				"reason":      result.Reason,
				"approval_id": result.ApprovalID,
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func (r *Runtime) appendToolAuditEvent(ctx context.Context, requestID any, eventType string, sessionID string, executionID string, toolName string, occurredAt time.Time, payload map[string]any) *rpc.Response {
	if r.dependencies.Audit == nil {
		return nil
	}

	if err := r.dependencies.Audit.Append(ctx, audit.Event{
		EventID:     mustNewID("evt"),
		EventType:   eventType,
		OccurredAt:  occurredAt.UTC(),
		SessionID:   sessionID,
		ExecutionID: executionID,
		ToolName:    toolName,
		Payload:     payload,
	}); err != nil {
		response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
			"reason": err.Error(),
		})
		return &response
	}

	return nil
}

func (r *Runtime) persistTerminalSessionStarted(ctx context.Context, requestID any, sessionID string, payload terminal.StartSessionResponse, metadata map[string]any) *rpc.Response {
	shellProfile, _ := metadata["shell_profile"].(string)

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordTerminalSession(ctx, storage.TerminalSessionRecord{
			TerminalSessionID: payload.TerminalSessionID,
			SessionID:         sessionID,
			WorkingDirectory:  payload.WorkingDirectory,
			ShellProfile:      shellProfile,
			Status:            "active",
			StartedAt:         payload.StartedAt.UTC(),
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist terminal session state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:           mustNewID("evt"),
			EventType:         "terminal.session.started",
			OccurredAt:        payload.StartedAt.UTC(),
			SessionID:         sessionID,
			TerminalSessionID: payload.TerminalSessionID,
			ToolName:          terminal.ToolStartSession,
			Payload: map[string]any{
				"working_directory": payload.WorkingDirectory,
				"shell_profile":     shellProfile,
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func (r *Runtime) persistTerminalSessionEnded(ctx context.Context, requestID any, sessionID string, terminalSessionID string, metadata map[string]any) *rpc.Response {
	startedAt, err := metadataTime(metadata, "started_at")
	if err != nil {
		response := rpc.NewErrorResponse(requestID, rpc.CodeInternalError, "invalid terminal session metadata", map[string]any{
			"reason": err.Error(),
		})
		return &response
	}

	endedAt, err := metadataTime(metadata, "ended_at")
	if err != nil {
		response := rpc.NewErrorResponse(requestID, rpc.CodeInternalError, "invalid terminal session metadata", map[string]any{
			"reason": err.Error(),
		})
		return &response
	}

	workingDirectory, _ := metadata["working_directory"].(string)
	shellProfile, _ := metadata["shell_profile"].(string)

	if r.dependencies.Storage != nil {
		if err := r.dependencies.Storage.RecordTerminalSession(ctx, storage.TerminalSessionRecord{
			TerminalSessionID: terminalSessionID,
			SessionID:         sessionID,
			WorkingDirectory:  workingDirectory,
			ShellProfile:      shellProfile,
			Status:            "ended",
			StartedAt:         startedAt.UTC(),
			EndedAt:           &endedAt,
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to persist terminal session state", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	if r.dependencies.Audit != nil {
		if err := r.dependencies.Audit.Append(ctx, audit.Event{
			EventID:           mustNewID("evt"),
			EventType:         "terminal.session.ended",
			OccurredAt:        endedAt.UTC(),
			SessionID:         sessionID,
			TerminalSessionID: terminalSessionID,
			ToolName:          terminal.ToolEndSession,
			Payload: map[string]any{
				"working_directory": workingDirectory,
				"shell_profile":     shellProfile,
			},
		}); err != nil {
			response := rpc.NewErrorResponse(requestID, rpc.CodeStorageError, "failed to append audit event", map[string]any{
				"reason": err.Error(),
			})
			return &response
		}
	}

	return nil
}

func metadataTime(metadata map[string]any, key string) (time.Time, error) {
	value, ok := metadata[key].(string)
	if !ok || value == "" {
		return time.Time{}, fmt.Errorf("missing %s", key)
	}

	return time.Parse(time.RFC3339Nano, value)
}

func mustNewID(prefix string) string {
	var payload [12]byte
	if _, err := rand.Read(payload[:]); err != nil {
		panic(err)
	}

	return prefix + "_" + hex.EncodeToString(payload[:])
}
