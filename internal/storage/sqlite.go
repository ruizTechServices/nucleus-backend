package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

var ErrNotFound = errors.New("record not found")

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

func (s *SQLiteStore) UpsertSession(ctx context.Context, metadata session.Metadata) error {
	approvedScopes, err := json.Marshal(metadata.ApprovedScopes)
	if err != nil {
		return err
	}

	capabilities, err := json.Marshal(metadata.Capabilities)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			session_id, session_token, client_name, client_version, trust_level,
			started_at, expires_at, approved_scopes, capabilities
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			session_token = excluded.session_token,
			client_name = excluded.client_name,
			client_version = excluded.client_version,
			trust_level = excluded.trust_level,
			started_at = excluded.started_at,
			expires_at = excluded.expires_at,
			approved_scopes = excluded.approved_scopes,
			capabilities = excluded.capabilities
	`,
		metadata.SessionID,
		metadata.SessionToken,
		metadata.ClientName,
		metadata.ClientVersion,
		string(metadata.TrustLevel),
		formatTime(metadata.StartedAt),
		formatTime(metadata.ExpiresAt),
		string(approvedScopes),
		string(capabilities),
	)

	return err
}

func (s *SQLiteStore) RecordTerminalSession(ctx context.Context, record TerminalSessionRecord) error {
	var endedAt any
	if record.EndedAt != nil {
		endedAt = formatTime(*record.EndedAt)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO terminal_sessions (
			terminal_session_id, session_id, working_directory, shell_profile,
			status, started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(terminal_session_id) DO UPDATE SET
			session_id = excluded.session_id,
			working_directory = excluded.working_directory,
			shell_profile = excluded.shell_profile,
			status = excluded.status,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at
	`,
		record.TerminalSessionID,
		record.SessionID,
		record.WorkingDirectory,
		record.ShellProfile,
		record.Status,
		formatTime(record.StartedAt),
		endedAt,
	)

	return err
}

func (s *SQLiteStore) RecordExecution(ctx context.Context, record ExecutionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO executions (
			execution_id, session_id, tool_name, status, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		record.ExecutionID,
		record.SessionID,
		record.ToolName,
		record.Status,
		formatTime(record.StartedAt),
		formatTime(record.FinishedAt),
	)

	return err
}

func (s *SQLiteStore) RecordApproval(ctx context.Context, record ApprovalRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approvals (
			approval_id, session_id, decision, reason, created_at, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		record.ApprovalID,
		record.SessionID,
		record.Decision,
		record.Reason,
		formatTime(record.CreatedAt),
		formatTime(record.ResolvedAt),
	)

	return err
}

func (s *SQLiteStore) RecordError(ctx context.Context, record ErrorRecord) error {
	details, err := json.Marshal(record.Details)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO errors (
			error_id, session_id, execution_id, code, message, details, occurred_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		record.ErrorID,
		nullString(record.SessionID),
		nullString(record.ExecutionID),
		record.Code,
		record.Message,
		string(details),
		formatTime(record.OccurredAt),
	)

	return err
}

func (s *SQLiteStore) GetSession(ctx context.Context, sessionID string) (session.Metadata, error) {
	var metadata session.Metadata
	var trustLevel string
	var approvedScopes string
	var capabilities string
	var startedAt string
	var expiresAt string

	row := s.db.QueryRowContext(ctx, `
		SELECT
			session_id, session_token, client_name, client_version, trust_level,
			started_at, expires_at, approved_scopes, capabilities
		FROM sessions
		WHERE session_id = ?
	`, sessionID)

	if err := row.Scan(
		&metadata.SessionID,
		&metadata.SessionToken,
		&metadata.ClientName,
		&metadata.ClientVersion,
		&trustLevel,
		&startedAt,
		&expiresAt,
		&approvedScopes,
		&capabilities,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.Metadata{}, ErrNotFound
		}
		return session.Metadata{}, err
	}

	metadata.TrustLevel = session.TrustLevel(trustLevel)

	var err error
	metadata.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return session.Metadata{}, err
	}

	metadata.ExpiresAt, err = parseTime(expiresAt)
	if err != nil {
		return session.Metadata{}, err
	}

	if err := json.Unmarshal([]byte(approvedScopes), &metadata.ApprovedScopes); err != nil {
		return session.Metadata{}, err
	}

	if err := json.Unmarshal([]byte(capabilities), &metadata.Capabilities); err != nil {
		return session.Metadata{}, err
	}

	return metadata, nil
}

func (s *SQLiteStore) ListTerminalSessions(ctx context.Context) ([]TerminalSessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT terminal_session_id, session_id, working_directory, shell_profile, status, started_at, ended_at
		FROM terminal_sessions
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TerminalSessionRecord
	for rows.Next() {
		var record TerminalSessionRecord
		var startedAt string
		var endedAt sql.NullString

		if err := rows.Scan(
			&record.TerminalSessionID,
			&record.SessionID,
			&record.WorkingDirectory,
			&record.ShellProfile,
			&record.Status,
			&startedAt,
			&endedAt,
		); err != nil {
			return nil, err
		}

		record.StartedAt, err = parseTime(startedAt)
		if err != nil {
			return nil, err
		}

		if endedAt.Valid {
			parsedEndedAt, err := parseTime(endedAt.String)
			if err != nil {
				return nil, err
			}
			record.EndedAt = &parsedEndedAt
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

func (s *SQLiteStore) ListExecutions(ctx context.Context) ([]ExecutionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT execution_id, session_id, tool_name, status, started_at, finished_at
		FROM executions
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ExecutionRecord
	for rows.Next() {
		var record ExecutionRecord
		var startedAt string
		var finishedAt string

		if err := rows.Scan(
			&record.ExecutionID,
			&record.SessionID,
			&record.ToolName,
			&record.Status,
			&startedAt,
			&finishedAt,
		); err != nil {
			return nil, err
		}

		record.StartedAt, err = parseTime(startedAt)
		if err != nil {
			return nil, err
		}

		record.FinishedAt, err = parseTime(finishedAt)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

func (s *SQLiteStore) ListApprovals(ctx context.Context) ([]ApprovalRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT approval_id, session_id, decision, reason, created_at, resolved_at
		FROM approvals
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ApprovalRecord
	for rows.Next() {
		var record ApprovalRecord
		var createdAt string
		var resolvedAt string

		if err := rows.Scan(
			&record.ApprovalID,
			&record.SessionID,
			&record.Decision,
			&record.Reason,
			&createdAt,
			&resolvedAt,
		); err != nil {
			return nil, err
		}

		record.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}

		record.ResolvedAt, err = parseTime(resolvedAt)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

func (s *SQLiteStore) ListErrors(ctx context.Context) ([]ErrorRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT error_id, session_id, execution_id, code, message, details, occurred_at
		FROM errors
		ORDER BY occurred_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ErrorRecord
	for rows.Next() {
		var record ErrorRecord
		var sessionID sql.NullString
		var executionID sql.NullString
		var details string
		var occurredAt string

		if err := rows.Scan(
			&record.ErrorID,
			&sessionID,
			&executionID,
			&record.Code,
			&record.Message,
			&details,
			&occurredAt,
		); err != nil {
			return nil, err
		}

		record.SessionID = sessionID.String
		record.ExecutionID = executionID.String
		record.OccurredAt, err = parseTime(occurredAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(details), &record.Details); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			session_token TEXT NOT NULL UNIQUE,
			client_name TEXT NOT NULL,
			client_version TEXT NOT NULL,
			trust_level TEXT NOT NULL,
			started_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			approved_scopes TEXT NOT NULL,
			capabilities TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS terminal_sessions (
			terminal_session_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			working_directory TEXT NOT NULL,
			shell_profile TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			FOREIGN KEY(session_id) REFERENCES sessions(session_id)
		);`,
		`CREATE TABLE IF NOT EXISTS executions (
			execution_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(session_id)
		);`,
		`CREATE TABLE IF NOT EXISTS approvals (
			approval_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			decision TEXT NOT NULL,
			reason TEXT NOT NULL,
			created_at TEXT NOT NULL,
			resolved_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(session_id)
		);`,
		`CREATE TABLE IF NOT EXISTS errors (
			error_id TEXT PRIMARY KEY,
			session_id TEXT,
			execution_id TEXT,
			code INTEGER NOT NULL,
			message TEXT NOT NULL,
			details TEXT NOT NULL,
			occurred_at TEXT NOT NULL
		);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func nullString(value string) any {
	if value == "" {
		return nil
	}

	return value
}
