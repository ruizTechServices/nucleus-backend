package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

func TestSQLiteStorePersistsAndQueriesState(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	startedAt := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	expiresAt := startedAt.Add(30 * time.Minute)
	endedAt := startedAt.Add(10 * time.Minute)
	executionFinishedAt := startedAt.Add(2 * time.Second)
	approvalResolvedAt := startedAt.Add(5 * time.Second)

	sessionRecord := session.Metadata{
		SessionID:      "sess_123",
		SessionToken:   "st_123",
		ClientName:     "nucleus-electron",
		ClientVersion:  "0.1.0",
		TrustLevel:     session.TrustLevelTrustedLocalClient,
		StartedAt:      startedAt,
		ExpiresAt:      expiresAt,
		ApprovedScopes: []string{"filesystem.read"},
		Capabilities:   []string{"tools.list"},
	}

	if err := store.UpsertSession(context.Background(), sessionRecord); err != nil {
		t.Fatalf("expected session upsert to succeed: %v", err)
	}

	if err := store.RecordTerminalSession(context.Background(), TerminalSessionRecord{
		TerminalSessionID: "term_123",
		SessionID:         sessionRecord.SessionID,
		WorkingDirectory:  "C:/project",
		ShellProfile:      "default",
		Status:            "ended",
		StartedAt:         startedAt,
		EndedAt:           &endedAt,
	}); err != nil {
		t.Fatalf("expected terminal session record to succeed: %v", err)
	}

	if err := store.RecordExecution(context.Background(), ExecutionRecord{
		ExecutionID: "exec_123",
		SessionID:   sessionRecord.SessionID,
		ToolName:    "tools.list",
		Status:      "succeeded",
		StartedAt:   startedAt,
		FinishedAt:  executionFinishedAt,
	}); err != nil {
		t.Fatalf("expected execution record to succeed: %v", err)
	}

	if err := store.RecordApproval(context.Background(), ApprovalRecord{
		ApprovalID: "approval_123",
		SessionID:  sessionRecord.SessionID,
		Decision:   "grant",
		Reason:     "trusted local client",
		CreatedAt:  startedAt,
		ResolvedAt: approvalResolvedAt,
	}); err != nil {
		t.Fatalf("expected approval record to succeed: %v", err)
	}

	if err := store.RecordError(context.Background(), ErrorRecord{
		ErrorID:    "err_123",
		SessionID:  sessionRecord.SessionID,
		Code:       40301,
		Message:    "policy denied request",
		Details:    map[string]any{"reason": "out of scope"},
		OccurredAt: startedAt,
	}); err != nil {
		t.Fatalf("expected error record to succeed: %v", err)
	}

	storedSession, err := store.GetSession(context.Background(), sessionRecord.SessionID)
	if err != nil {
		t.Fatalf("expected session query to succeed: %v", err)
	}

	if storedSession.SessionToken != sessionRecord.SessionToken {
		t.Fatalf("expected stored session token %q, got %q", sessionRecord.SessionToken, storedSession.SessionToken)
	}

	terminalSessions, err := store.ListTerminalSessions(context.Background())
	if err != nil {
		t.Fatalf("expected terminal session query to succeed: %v", err)
	}

	if len(terminalSessions) != 1 || terminalSessions[0].TerminalSessionID != "term_123" {
		t.Fatalf("unexpected terminal session rows: %+v", terminalSessions)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 1 || executions[0].ExecutionID != "exec_123" {
		t.Fatalf("unexpected execution rows: %+v", executions)
	}

	approvals, err := store.ListApprovals(context.Background())
	if err != nil {
		t.Fatalf("expected approval query to succeed: %v", err)
	}

	if len(approvals) != 1 || approvals[0].ApprovalID != "approval_123" {
		t.Fatalf("unexpected approval rows: %+v", approvals)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].ErrorID != "err_123" {
		t.Fatalf("unexpected error rows: %+v", errorsList)
	}
}

func TestSQLiteStoreReturnsNotFoundForMissingSession(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	_, err = store.GetSession(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
