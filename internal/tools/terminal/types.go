package terminal

import "time"

const (
	ToolStartSession = "terminal.start_session"
	ToolExec         = "terminal.exec"
	ToolEndSession   = "terminal.end_session"
)

type StartSessionRequest struct {
	WorkingDirectory string `json:"working_directory,omitempty"`
	ShellProfile     string `json:"shell_profile,omitempty"`
}

type StartSessionResponse struct {
	TerminalSessionID string    `json:"terminal_session_id"`
	StartedAt         time.Time `json:"started_at"`
	WorkingDirectory  string    `json:"working_directory,omitempty"`
}

type ExecRequest struct {
	TerminalSessionID string `json:"terminal_session_id"`
	Command           string `json:"command"`
	TimeoutMS         int    `json:"timeout_ms"`
}

type ExecResponse struct {
	ExecutionID       string `json:"execution_id"`
	TerminalSessionID string `json:"terminal_session_id"`
	Command           string `json:"command"`
	Stdout            string `json:"stdout"`
	Stderr            string `json:"stderr"`
	ExitCode          int    `json:"exit_code"`
	DurationMS        int64  `json:"duration_ms"`
}

type EndSessionRequest struct {
	TerminalSessionID string `json:"terminal_session_id"`
}

type EndSessionResponse struct {
	TerminalSessionID string `json:"terminal_session_id"`
	Status            string `json:"status"`
}
