package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/audit"
	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/storage"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/desktop"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/filesystem"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/screenshot"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/terminal"
)

func TestRuntimeHandleHealth(t *testing.T) {
	rt := New(Dependencies{}, BuildInfo{
		Service: "nucleusd-test",
		Version: "1.2.3",
	})

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_1",
		Method:  methodRuntimeHealth,
	})
	if err != nil {
		t.Fatalf("expected runtime to handle health request: %v", err)
	}

	if response.Error != nil {
		t.Fatalf("expected successful response, got error: %+v", response.Error)
	}

	if response.Result == nil || !response.Result.OK {
		t.Fatal("expected structured success envelope")
	}

	data, ok := response.Result.Data.(HealthResult)
	if !ok {
		t.Fatalf("expected health result type, got %T", response.Result.Data)
	}

	if data.Service != "nucleusd-test" || data.Version != "1.2.3" || data.Status != "ok" {
		t.Fatalf("unexpected health payload: %+v", data)
	}
}

func TestRuntimeHandleUnknownMethod(t *testing.T) {
	rt := New(Dependencies{}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_2",
		Method:  "runtime.unknown",
	})
	if err != nil {
		t.Fatalf("expected structured error response, got runtime error: %v", err)
	}

	if response.Error == nil {
		t.Fatal("expected error response for unknown method")
	}

	if response.Error.Code != rpc.CodeMethodNotFound {
		t.Fatalf("expected method not found code %d, got %d", rpc.CodeMethodNotFound, response.Error.Code)
	}
}

type staticRegistry struct {
	definitions []tools.Definition
}

func (s staticRegistry) List() []tools.Definition {
	return s.definitions
}

func (staticRegistry) Lookup(string) (tools.Handler, bool) {
	return nil, false
}

type failingStateStore struct{}

func (failingStateStore) UpsertSession(context.Context, session.Metadata) error {
	return errors.New("disk full")
}

func (failingStateStore) RecordTerminalSession(context.Context, storage.TerminalSessionRecord) error {
	return errors.New("disk full")
}

func (failingStateStore) RecordExecution(context.Context, storage.ExecutionRecord) error {
	return errors.New("disk full")
}

func (failingStateStore) RecordApproval(context.Context, storage.ApprovalRecord) error {
	return errors.New("disk full")
}

func (failingStateStore) RecordError(context.Context, storage.ErrorRecord) error {
	return errors.New("disk full")
}

func allowToolsListPolicy() policy.Engine {
	return policy.NewStaticEngine(policy.Config{
		ActionRules: map[string]policy.Rule{
			methodToolsList: {
				Decision: policy.DecisionAllow,
				Reason:   "tool discovery is allowed",
			},
		},
	})
}

func allowFilesystemPolicy(prefix string) policy.Engine {
	return policy.NewStaticEngine(policy.Config{
		ActionRules: map[string]policy.Rule{
			filesystem.ToolList: {
				Decision: policy.DecisionAllow,
				Reason:   "filesystem list is allowed",
			},
			filesystem.ToolRead: {
				Decision: policy.DecisionAllow,
				Reason:   "filesystem read is allowed",
			},
		},
		PathRules: []policy.PathRule{
			{
				Prefix: filepath.ToSlash(prefix),
				Rule: policy.Rule{
					Decision: policy.DecisionAllow,
					Reason:   "filesystem path is allowed by policy",
				},
			},
		},
	})
}

func allowTerminalPolicy(prefix string) policy.Engine {
	return policy.NewStaticEngine(policy.Config{
		ActionRules: map[string]policy.Rule{
			terminal.ToolStartSession: {
				Decision: policy.DecisionAllow,
				Reason:   "terminal sessions are allowed for tests",
			},
			terminal.ToolExec: {
				Decision: policy.DecisionAllow,
				Reason:   "terminal exec is allowed for tests",
			},
			terminal.ToolEndSession: {
				Decision: policy.DecisionAllow,
				Reason:   "terminal session end is allowed for tests",
			},
		},
		PathRules: []policy.PathRule{
			{
				Prefix: filepath.ToSlash(prefix),
				Rule: policy.Rule{
					Decision: policy.DecisionAllow,
					Reason:   "terminal working directory is allowed by policy",
				},
			},
		},
		MaxTimeoutMS: 1000,
	})
}

func allowVisualPolicy() policy.Engine {
	return policy.NewStaticEngine(policy.Config{
		ActionRules: map[string]policy.Rule{
			screenshot.ToolCapture: {
				Decision: policy.DecisionAllow,
				Reason:   "screenshot capture is allowed for tests",
			},
			desktop.ToolGetState: {
				Decision: policy.DecisionAllow,
				Reason:   "desktop state is allowed for tests",
			},
		},
	})
}

type fakeToolHandler struct {
	definition tools.Definition
	invoke     func(context.Context, tools.Call) (tools.Result, error)
}

func (f fakeToolHandler) Definition() tools.Definition {
	return f.definition
}

func (f fakeToolHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	return f.invoke(ctx, call)
}

type fakeTerminalRunner struct {
	run func(context.Context, terminal.CommandRequest) (terminal.CommandResult, error)
}

func (f fakeTerminalRunner) Run(ctx context.Context, request terminal.CommandRequest) (terminal.CommandResult, error) {
	return f.run(ctx, request)
}

type fakeScreenshotProvider struct {
	capture func(context.Context, screenshot.CaptureRequest) (screenshot.CaptureResponse, error)
}

func (f fakeScreenshotProvider) Capture(ctx context.Context, request screenshot.CaptureRequest) (screenshot.CaptureResponse, error) {
	return f.capture(ctx, request)
}

type fakeDesktopProvider struct {
	getState func(context.Context) (desktop.GetStateResponse, error)
}

func (f fakeDesktopProvider) GetState(ctx context.Context) (desktop.GetStateResponse, error) {
	return f.getState(ctx)
}

func TestRuntimeHandleSessionBootstrap(t *testing.T) {
	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		},
	})

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowToolsListPolicy(),
	}, DefaultBuildInfo())

	params, err := json.Marshal(session.BootstrapRequest{
		ClientName:     "nucleus-electron",
		ClientVersion:  "0.1.0",
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected bootstrap params to marshal: %v", err)
	}

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_bootstrap",
		Method:  methodSessionBootstrap,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to return a structured response: %v", err)
	}

	if response.Error != nil {
		t.Fatalf("expected bootstrap success, got error: %+v", response.Error)
	}

	metadata, ok := response.Result.Data.(session.Metadata)
	if !ok {
		t.Fatalf("expected session metadata payload, got %T", response.Result.Data)
	}

	if metadata.SessionToken == "" || metadata.SessionID == "" {
		t.Fatalf("expected bootstrap to issue session credentials: %+v", metadata)
	}
}

func TestRuntimeRejectsUnauthenticatedToolsList(t *testing.T) {
	rt := New(Dependencies{
		Sessions: session.NewMemoryService(session.Config{
			BootstrapToken: "bootstrap-secret",
		}),
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools",
		Method:  methodToolsList,
	})
	if err != nil {
		t.Fatalf("expected tools.list auth failure to be structured: %v", err)
	}

	if response.Error == nil {
		t.Fatal("expected unauthenticated tools.list request to be rejected")
	}

	if response.Error.Code != rpc.CodeInvalidSessionToken {
		t.Fatalf("expected invalid session code %d, got %d", rpc.CodeInvalidSessionToken, response.Error.Code)
	}
}

func TestRuntimeAllowsAuthenticatedToolsList(t *testing.T) {
	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected test bootstrap to succeed: %v", err)
	}

	params, err := json.Marshal(sessionTokenParams{
		SessionToken: metadata.SessionToken,
	})
	if err != nil {
		t.Fatalf("expected session params to marshal: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Registry: staticRegistry{
			definitions: []tools.Definition{
				{Name: "filesystem.read", Risk: tools.RiskMedium},
			},
		},
		Policy: allowToolsListPolicy(),
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools",
		Method:  methodToolsList,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected authenticated tools.list request to succeed: %v", err)
	}

	if response.Error != nil {
		t.Fatalf("expected successful tools.list response, got error: %+v", response.Error)
	}

	data, ok := response.Result.Data.(toolsListResult)
	if !ok {
		t.Fatalf("expected tools list payload, got %T", response.Result.Data)
	}

	if len(data.Tools) != 1 || data.Tools[0].Name != "filesystem.read" {
		t.Fatalf("unexpected tools list payload: %+v", data)
	}
}

func TestRuntimePersistsSessionBootstrapAndToolsList(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected file sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected file sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	rt := New(Dependencies{
		Sessions: sessionService,
		Registry: staticRegistry{
			definitions: []tools.Definition{
				{Name: "filesystem.read", Risk: tools.RiskMedium},
			},
		},
		Audit:   sink,
		Storage: store,
		Policy:  allowToolsListPolicy(),
	}, DefaultBuildInfo())

	bootstrapParams, err := json.Marshal(session.BootstrapRequest{
		ClientName:     "nucleus-electron",
		ClientVersion:  "0.1.0",
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected bootstrap params to marshal: %v", err)
	}

	bootstrapResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_bootstrap",
		Method:  methodSessionBootstrap,
		Params:  bootstrapParams,
	})
	if err != nil {
		t.Fatalf("expected bootstrap request to succeed: %v", err)
	}

	metadata := bootstrapResponse.Result.Data.(session.Metadata)

	storedSession, err := store.GetSession(context.Background(), metadata.SessionID)
	if err != nil {
		t.Fatalf("expected session to persist to sqlite: %v", err)
	}

	if storedSession.SessionToken != metadata.SessionToken {
		t.Fatalf("expected persisted session token %q, got %q", metadata.SessionToken, storedSession.SessionToken)
	}

	toolsParams, err := json.Marshal(sessionTokenParams{
		SessionToken: metadata.SessionToken,
	})
	if err != nil {
		t.Fatalf("expected tools params to marshal: %v", err)
	}

	toolsResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools",
		Method:  methodToolsList,
		Params:  toolsParams,
	})
	if err != nil {
		t.Fatalf("expected tools.list request to succeed: %v", err)
	}

	if toolsResponse.Error != nil {
		t.Fatalf("expected tools.list success, got error: %+v", toolsResponse.Error)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution state to query: %v", err)
	}

	if len(executions) != 1 || executions[0].ToolName != methodToolsList {
		t.Fatalf("unexpected persisted executions: %+v", executions)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit log to load: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected session bootstrap + tool requested + tool completed events, got %d", len(events))
	}
}

func TestRuntimeSurfacesStorageFailuresExplicitly(t *testing.T) {
	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	rt := New(Dependencies{
		Sessions: sessionService,
		Storage:  failingStateStore{},
	}, DefaultBuildInfo())

	params, err := json.Marshal(session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected bootstrap params to marshal: %v", err)
	}

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_bootstrap",
		Method:  methodSessionBootstrap,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected structured storage error response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("expected bootstrap to fail when persistence fails")
	}

	if response.Error.Code != rpc.CodeStorageError {
		t.Fatalf("expected storage error code %d, got %d", rpc.CodeStorageError, response.Error.Code)
	}
}

func TestRuntimeDeniesPolicyBlockedToolsList(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist for policy test: %v", err)
	}

	params, err := json.Marshal(sessionTokenParams{
		SessionToken: metadata.SessionToken,
	})
	if err != nil {
		t.Fatalf("expected session params to marshal: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				methodToolsList: {
					Decision: policy.DecisionDeny,
					Reason:   "tool discovery disabled by policy",
				},
			},
		}),
		Storage: store,
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools",
		Method:  methodToolsList,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected policy denial to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodePolicyDenied {
		t.Fatalf("expected policy denied response, got %+v", response.Error)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 0 {
		t.Fatalf("expected denied request to avoid execution persistence, got %+v", executions)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].Code != rpc.CodePolicyDenied {
		t.Fatalf("unexpected persisted policy errors: %+v", errorsList)
	}
}

func TestRuntimeReturnsApprovalRequiredWhenPolicyRequiresIt(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist for policy test: %v", err)
	}

	params, err := json.Marshal(sessionTokenParams{
		SessionToken: metadata.SessionToken,
	})
	if err != nil {
		t.Fatalf("expected session params to marshal: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				methodToolsList: {
					Decision: policy.DecisionApprovalRequired,
					Reason:   "tool discovery requires approval",
				},
			},
		}),
		Storage: store,
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools",
		Method:  methodToolsList,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected approval-required response to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeApprovalRequired {
		t.Fatalf("expected approval-required response, got %+v", response.Error)
	}

	approvals, err := store.ListApprovals(context.Background())
	if err != nil {
		t.Fatalf("expected approvals query to succeed: %v", err)
	}

	if len(approvals) != 1 || approvals[0].Decision != string(policy.DecisionApprovalRequired) {
		t.Fatalf("unexpected persisted approvals: %+v", approvals)
	}
}

func TestRuntimeExecutesToolsCallThroughRegistryAndExecutor(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist for executor test: %v", err)
	}

	registry := tools.NewStaticRegistry(tools.Entry{
		Handler: fakeToolHandler{
			definition: tools.Definition{Name: "filesystem.read", Risk: tools.RiskMedium},
			invoke: func(_ context.Context, call tools.Call) (tools.Result, error) {
				return tools.Result{
					Payload: map[string]any{
						"path": call.Arguments["path"],
					},
					Metadata: map[string]any{
						"encoding": "utf-8",
					},
				}, nil
			},
		},
	})

	params, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     "filesystem.read",
		Arguments: map[string]any{
			"path": "C:/allowed/file.txt",
		},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"filesystem.read": {
					Decision: policy.DecisionAllow,
					Reason:   "filesystem read is allowed",
				},
			},
			PathRules: []policy.PathRule{
				{
					Prefix: "C:/allowed",
					Rule: policy.Rule{
						Decision: policy.DecisionAllow,
						Reason:   "allowed path",
					},
				},
			},
		}),
		Registry: registry,
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_tools_call", nil
			},
		}),
		Storage: store,
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools_call",
		Method:  methodToolsCall,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected tools.call request to succeed: %v", err)
	}

	if response.Error != nil {
		t.Fatalf("expected tools.call success, got error: %+v", response.Error)
	}

	payload, ok := response.Result.Data.(toolsCallResult)
	if !ok {
		t.Fatalf("expected tools.call result payload, got %T", response.Result.Data)
	}

	if payload.ToolName != "filesystem.read" || payload.ExecutionID != "exec_tools_call" {
		t.Fatalf("unexpected tools.call payload: %+v", payload)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 1 || executions[0].ExecutionID != "exec_tools_call" {
		t.Fatalf("unexpected persisted executions: %+v", executions)
	}
}

func TestRuntimeExecutesFilesystemListAndReadAndPersistsResults(t *testing.T) {
	baseDir := t.TempDir()
	allowedRoot := filepath.Join(baseDir, "allowed")
	if err := os.Mkdir(allowedRoot, 0o755); err != nil {
		t.Fatalf("expected allowed root to be created: %v", err)
	}

	nestedDir := filepath.Join(allowedRoot, "nested")
	if err := os.Mkdir(nestedDir, 0o755); err != nil {
		t.Fatalf("expected nested directory to be created: %v", err)
	}

	filePath := filepath.Join(allowedRoot, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello nucleus"), 0o644); err != nil {
		t.Fatalf("expected text fixture to be written: %v", err)
	}

	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	filesystemService, err := filesystem.NewService(filesystem.Config{
		AllowedRoots: []string{allowedRoot},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	ids := []string{"exec_filesystem_list", "exec_filesystem_read"}
	nextID := 0

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowFilesystemPolicy(baseDir),
		Registry: tools.NewStaticRegistry(filesystemService.Entries()...),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				id := ids[nextID]
				nextID++
				return id, nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	listParams, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     filesystem.ToolList,
		Arguments: map[string]any{
			"path": allowedRoot,
		},
	})
	if err != nil {
		t.Fatalf("expected list params to marshal: %v", err)
	}

	listResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_filesystem_list",
		Method:  methodToolsCall,
		Params:  listParams,
	})
	if err != nil {
		t.Fatalf("expected filesystem.list request to succeed: %v", err)
	}

	if listResponse.Error != nil {
		t.Fatalf("expected filesystem.list success, got error: %+v", listResponse.Error)
	}

	listPayload, ok := listResponse.Result.Data.(toolsCallResult)
	if !ok {
		t.Fatalf("expected tools.call list payload, got %T", listResponse.Result.Data)
	}

	if listPayload.ToolName != filesystem.ToolList || listPayload.ExecutionID != "exec_filesystem_list" {
		t.Fatalf("unexpected filesystem.list payload: %+v", listPayload)
	}

	listResult, ok := listPayload.Result.(filesystem.ListResponse)
	if !ok {
		t.Fatalf("expected filesystem list result payload, got %T", listPayload.Result)
	}

	if len(listResult.Entries) != 2 {
		t.Fatalf("expected two filesystem list entries, got %+v", listResult.Entries)
	}

	readParams, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     filesystem.ToolRead,
		Arguments: map[string]any{
			"path": filePath,
		},
	})
	if err != nil {
		t.Fatalf("expected read params to marshal: %v", err)
	}

	readResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_filesystem_read",
		Method:  methodToolsCall,
		Params:  readParams,
	})
	if err != nil {
		t.Fatalf("expected filesystem.read request to succeed: %v", err)
	}

	if readResponse.Error != nil {
		t.Fatalf("expected filesystem.read success, got error: %+v", readResponse.Error)
	}

	readPayload, ok := readResponse.Result.Data.(toolsCallResult)
	if !ok {
		t.Fatalf("expected tools.call read payload, got %T", readResponse.Result.Data)
	}

	readResult, ok := readPayload.Result.(filesystem.ReadResponse)
	if !ok {
		t.Fatalf("expected filesystem read result payload, got %T", readPayload.Result)
	}

	if readResult.Content != "hello nucleus" || readResult.Encoding != "utf-8" {
		t.Fatalf("unexpected filesystem.read result payload: %+v", readResult)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 2 {
		t.Fatalf("expected two persisted executions, got %+v", executions)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit events to load: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected requested/completed audit events for both filesystem calls, got %d", len(events))
	}
}

func TestRuntimeRejectsOutOfScopeFilesystemReadAndPersistsFailure(t *testing.T) {
	baseDir := t.TempDir()
	allowedRoot := filepath.Join(baseDir, "allowed")
	if err := os.Mkdir(allowedRoot, 0o755); err != nil {
		t.Fatalf("expected allowed root to be created: %v", err)
	}

	deniedRoot := filepath.Join(baseDir, "denied")
	if err := os.Mkdir(deniedRoot, 0o755); err != nil {
		t.Fatalf("expected denied root to be created: %v", err)
	}

	deniedPath := filepath.Join(deniedRoot, "secret.txt")
	if err := os.WriteFile(deniedPath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("expected denied fixture to be written: %v", err)
	}

	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	filesystemService, err := filesystem.NewService(filesystem.Config{
		AllowedRoots: []string{allowedRoot},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowFilesystemPolicy(baseDir),
		Registry: tools.NewStaticRegistry(filesystemService.Entries()...),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_filesystem_denied", nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	params, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     filesystem.ToolRead,
		Arguments: map[string]any{
			"path": deniedPath,
		},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_filesystem_denied",
		Method:  methodToolsCall,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected denied filesystem.read response to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodePolicyDenied {
		t.Fatalf("expected filesystem denial response, got %+v", response.Error)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 1 || executions[0].Status != "failed" {
		t.Fatalf("expected one failed execution record, got %+v", executions)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].Code != rpc.CodePolicyDenied {
		t.Fatalf("unexpected persisted filesystem denial errors: %+v", errorsList)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit events to load: %v", err)
	}

	if len(events) != 2 || events[1].EventType != "tool.failed" {
		t.Fatalf("unexpected filesystem denial audit events: %+v", events)
	}
}

func TestRuntimeRejectsFilesystemValidationErrorsAndPersistsFailure(t *testing.T) {
	baseDir := t.TempDir()
	allowedRoot := filepath.Join(baseDir, "allowed")
	if err := os.Mkdir(allowedRoot, 0o755); err != nil {
		t.Fatalf("expected allowed root to be created: %v", err)
	}

	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	filesystemService, err := filesystem.NewService(filesystem.Config{
		AllowedRoots: []string{allowedRoot},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowFilesystemPolicy(baseDir),
		Registry: tools.NewStaticRegistry(filesystemService.Entries()...),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_filesystem_validation", nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	params, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     filesystem.ToolRead,
		Arguments: map[string]any{
			"path": allowedRoot,
		},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_filesystem_validation",
		Method:  methodToolsCall,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected filesystem validation response to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeValidationError {
		t.Fatalf("expected filesystem validation response, got %+v", response.Error)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 1 || executions[0].Status != "failed" {
		t.Fatalf("expected one failed execution record, got %+v", executions)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].Code != rpc.CodeValidationError {
		t.Fatalf("unexpected persisted filesystem validation errors: %+v", errorsList)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit events to load: %v", err)
	}

	if len(events) != 2 || events[1].EventType != "tool.failed" {
		t.Fatalf("unexpected filesystem validation audit events: %+v", events)
	}
}

func TestRuntimeHandlesTerminalLifecycleAndPersistsResults(t *testing.T) {
	baseDir := t.TempDir()
	workingDirectory := filepath.Join(baseDir, "workspace")
	if err := os.Mkdir(workingDirectory, 0o755); err != nil {
		t.Fatalf("expected working directory to be created: %v", err)
	}

	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	terminalService, err := terminal.NewService(terminal.Config{
		DefaultWorkingDirectory: workingDirectory,
		GenerateSessionID: func() (string, error) {
			return "term_lifecycle", nil
		},
		Runner: fakeTerminalRunner{
			run: func(_ context.Context, request terminal.CommandRequest) (terminal.CommandResult, error) {
				return terminal.CommandResult{
					Stdout:   "ok:" + request.Command,
					ExitCode: 0,
					Duration: 15 * time.Millisecond,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected terminal service to initialize: %v", err)
	}

	ids := []string{"exec_terminal_start", "exec_terminal_exec_1", "exec_terminal_exec_2", "exec_terminal_end"}
	nextID := 0

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowTerminalPolicy(baseDir),
		Registry: tools.NewStaticRegistry(terminalService.Entries()...),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				id := ids[nextID]
				nextID++
				return id, nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	startParams, err := json.Marshal(terminalStartSessionParams{
		SessionToken:     metadata.SessionToken,
		WorkingDirectory: workingDirectory,
		ShellProfile:     "default",
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session params to marshal: %v", err)
	}

	startResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_terminal_start",
		Method:  methodTerminalStart,
		Params:  startParams,
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session request to succeed: %v", err)
	}

	if startResponse.Error != nil {
		t.Fatalf("expected terminal.start_session success, got error: %+v", startResponse.Error)
	}

	startPayload, ok := startResponse.Result.Data.(terminal.StartSessionResponse)
	if !ok {
		t.Fatalf("expected terminal.start_session payload, got %T", startResponse.Result.Data)
	}

	for index, command := range []string{"go version", "go env GOROOT"} {
		execParams, err := json.Marshal(terminalExecParams{
			SessionToken:      metadata.SessionToken,
			TerminalSessionID: startPayload.TerminalSessionID,
			Command:           command,
			TimeoutMS:         100,
		})
		if err != nil {
			t.Fatalf("expected terminal.exec params to marshal: %v", err)
		}

		execResponse, err := rt.Handle(context.Background(), rpc.Request{
			JSONRPC: rpc.Version,
			ID:      "req_terminal_exec",
			Method:  methodTerminalExec,
			Params:  execParams,
		})
		if err != nil {
			t.Fatalf("expected terminal.exec request to succeed: %v", err)
		}

		if execResponse.Error != nil {
			t.Fatalf("expected terminal.exec success, got error: %+v", execResponse.Error)
		}

		execPayload, ok := execResponse.Result.Data.(terminal.ExecResponse)
		if !ok {
			t.Fatalf("expected terminal.exec payload, got %T", execResponse.Result.Data)
		}

		if execPayload.ExecutionID != ids[index+1] || execPayload.Stdout != "ok:"+command {
			t.Fatalf("unexpected terminal.exec payload: %+v", execPayload)
		}
	}

	endParams, err := json.Marshal(terminalEndSessionParams{
		SessionToken:      metadata.SessionToken,
		TerminalSessionID: startPayload.TerminalSessionID,
	})
	if err != nil {
		t.Fatalf("expected terminal.end_session params to marshal: %v", err)
	}

	endResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_terminal_end",
		Method:  methodTerminalEnd,
		Params:  endParams,
	})
	if err != nil {
		t.Fatalf("expected terminal.end_session request to succeed: %v", err)
	}

	if endResponse.Error != nil {
		t.Fatalf("expected terminal.end_session success, got error: %+v", endResponse.Error)
	}

	terminalSessions, err := store.ListTerminalSessions(context.Background())
	if err != nil {
		t.Fatalf("expected terminal session query to succeed: %v", err)
	}

	if len(terminalSessions) != 1 || terminalSessions[0].Status != "ended" {
		t.Fatalf("unexpected persisted terminal sessions: %+v", terminalSessions)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 4 {
		t.Fatalf("expected four persisted terminal executions, got %+v", executions)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit events to load: %v", err)
	}

	if len(events) != 10 {
		t.Fatalf("expected generic execution events plus terminal lifecycle events, got %d", len(events))
	}
}

func TestRuntimeReturnsTimeoutForTerminalExecAndPersistsFailure(t *testing.T) {
	baseDir := t.TempDir()
	workingDirectory := filepath.Join(baseDir, "workspace")
	if err := os.Mkdir(workingDirectory, 0o755); err != nil {
		t.Fatalf("expected working directory to be created: %v", err)
	}

	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	terminalService, err := terminal.NewService(terminal.Config{
		DefaultWorkingDirectory: workingDirectory,
		GenerateSessionID: func() (string, error) {
			return "term_timeout", nil
		},
		Runner: fakeTerminalRunner{
			run: func(ctx context.Context, _ terminal.CommandRequest) (terminal.CommandResult, error) {
				<-ctx.Done()
				return terminal.CommandResult{}, ctx.Err()
			},
		},
	})
	if err != nil {
		t.Fatalf("expected terminal service to initialize: %v", err)
	}

	ids := []string{"exec_terminal_start", "exec_terminal_timeout"}
	nextID := 0

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowTerminalPolicy(baseDir),
		Registry: tools.NewStaticRegistry(terminalService.Entries()...),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				id := ids[nextID]
				nextID++
				return id, nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	startParams, err := json.Marshal(terminalStartSessionParams{
		SessionToken:     metadata.SessionToken,
		WorkingDirectory: workingDirectory,
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session params to marshal: %v", err)
	}

	startResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_terminal_start",
		Method:  methodTerminalStart,
		Params:  startParams,
	})
	if err != nil {
		t.Fatalf("expected terminal.start_session request to succeed: %v", err)
	}

	startPayload := startResponse.Result.Data.(terminal.StartSessionResponse)

	execParams, err := json.Marshal(terminalExecParams{
		SessionToken:      metadata.SessionToken,
		TerminalSessionID: startPayload.TerminalSessionID,
		Command:           "go version",
		TimeoutMS:         5,
	})
	if err != nil {
		t.Fatalf("expected terminal.exec params to marshal: %v", err)
	}

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_terminal_exec",
		Method:  methodTerminalExec,
		Params:  execParams,
	})
	if err != nil {
		t.Fatalf("expected timeout response to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeExecutionTimeout {
		t.Fatalf("expected terminal timeout response, got %+v", response.Error)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].Code != rpc.CodeExecutionTimeout {
		t.Fatalf("unexpected persisted terminal timeout errors: %+v", errorsList)
	}

	terminalSessions, err := store.ListTerminalSessions(context.Background())
	if err != nil {
		t.Fatalf("expected terminal session query to succeed: %v", err)
	}

	if len(terminalSessions) != 1 || terminalSessions[0].Status != "active" {
		t.Fatalf("expected active terminal session after timed out command, got %+v", terminalSessions)
	}
}

func TestRuntimeHandlesScreenshotAndDesktopStateRequests(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	auditPath := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("expected audit sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected audit sink to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist: %v", err)
	}

	screenshotService, err := screenshot.NewService(screenshot.Config{
		Provider: fakeScreenshotProvider{
			capture: func(_ context.Context, request screenshot.CaptureRequest) (screenshot.CaptureResponse, error) {
				return screenshot.CaptureResponse{
					CaptureID: "cap_visual",
					MIMEType:  "image/png",
					Width:     1920,
					Height:    1080,
					Metadata: map[string]any{
						"display_id": request.DisplayID,
						"path":       "C:/captures/cap_visual.png",
					},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected screenshot service to initialize: %v", err)
	}

	desktopService := desktop.NewService(desktop.Config{
		Provider: fakeDesktopProvider{
			getState: func(context.Context) (desktop.GetStateResponse, error) {
				return desktop.GetStateResponse{
					ActiveWindow: &desktop.ActiveWindow{
						Title:   "Visual Studio Code",
						AppName: "Code",
					},
					Displays: []desktop.Display{
						{
							DisplayID: "primary",
							Width:     1920,
							Height:    1080,
						},
					},
				}, nil
			},
		},
	})

	nextVisualID := 0

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy:   allowVisualPolicy(),
		Registry: tools.NewStaticRegistry(
			append(screenshotService.Entries(), desktopService.Entries()...)...,
		),
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				nextVisualID++
				if nextVisualID == 1 {
					return "exec_screenshot", nil
				}
				return "exec_desktop", nil
			},
		}),
		Audit:   sink,
		Storage: store,
	}, DefaultBuildInfo())

	screenshotParams, err := json.Marshal(screenshotCaptureParams{
		SessionToken: metadata.SessionToken,
		DisplayID:    "primary",
	})
	if err != nil {
		t.Fatalf("expected screenshot.capture params to marshal: %v", err)
	}

	screenshotResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_screenshot",
		Method:  methodScreenshotCapture,
		Params:  screenshotParams,
	})
	if err != nil {
		t.Fatalf("expected screenshot.capture request to succeed: %v", err)
	}

	if screenshotResponse.Error != nil {
		t.Fatalf("expected screenshot.capture success, got error: %+v", screenshotResponse.Error)
	}

	screenshotPayload, ok := screenshotResponse.Result.Data.(toolsCallResult)
	if !ok {
		t.Fatalf("expected screenshot tools.call payload, got %T", screenshotResponse.Result.Data)
	}

	if screenshotPayload.ToolName != screenshot.ToolCapture || screenshotPayload.ExecutionID != "exec_screenshot" {
		t.Fatalf("unexpected screenshot payload: %+v", screenshotPayload)
	}

	desktopParams, err := json.Marshal(desktopGetStateParams{
		SessionToken: metadata.SessionToken,
	})
	if err != nil {
		t.Fatalf("expected desktop.get_state params to marshal: %v", err)
	}

	desktopResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_desktop",
		Method:  methodDesktopGetState,
		Params:  desktopParams,
	})
	if err != nil {
		t.Fatalf("expected desktop.get_state request to succeed: %v", err)
	}

	if desktopResponse.Error != nil {
		t.Fatalf("expected desktop.get_state success, got error: %+v", desktopResponse.Error)
	}

	desktopPayload, ok := desktopResponse.Result.Data.(toolsCallResult)
	if !ok {
		t.Fatalf("expected desktop tools.call payload, got %T", desktopResponse.Result.Data)
	}

	if desktopPayload.ToolName != desktop.ToolGetState || desktopPayload.ExecutionID != "exec_desktop" {
		t.Fatalf("unexpected desktop payload: %+v", desktopPayload)
	}

	executions, err := store.ListExecutions(context.Background())
	if err != nil {
		t.Fatalf("expected execution query to succeed: %v", err)
	}

	if len(executions) != 2 {
		t.Fatalf("expected two persisted visual executions, got %+v", executions)
	}

	events, err := audit.LoadEvents(auditPath)
	if err != nil {
		t.Fatalf("expected audit events to load: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected requested/completed events for screenshot and desktop calls, got %d", len(events))
	}
}

func TestRuntimeReturnsTimeoutForExecutorTimeout(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("expected sqlite store to initialize: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("expected sqlite store to close cleanly: %v", err)
		}
	}()

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	if err := store.UpsertSession(context.Background(), metadata); err != nil {
		t.Fatalf("expected session state to persist for executor test: %v", err)
	}

	registry := tools.NewStaticRegistry(tools.Entry{
		Handler: fakeToolHandler{
			definition: tools.Definition{Name: "terminal.exec", Risk: tools.RiskHigh},
			invoke: func(ctx context.Context, _ tools.Call) (tools.Result, error) {
				<-ctx.Done()
				return tools.Result{}, ctx.Err()
			},
		},
	})

	params, err := json.Marshal(toolCallParams{
		SessionToken: metadata.SessionToken,
		ToolName:     "terminal.exec",
		Arguments: map[string]any{
			"command":    "echo test",
			"timeout_ms": 5,
		},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	rt := New(Dependencies{
		Sessions: sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"terminal.exec": {
					Decision: policy.DecisionAllow,
					Reason:   "terminal exec allowed for timeout test",
				},
			},
			MaxTimeoutMS: 100,
		}),
		Registry: registry,
		Executor: executor.NewDefaultRunner(executor.Config{
			DefaultTimeout: 5 * time.Millisecond,
			GenerateID: func() (string, error) {
				return "exec_timeout_case", nil
			},
		}),
		Storage: store,
	}, DefaultBuildInfo())

	response, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools_call",
		Method:  methodToolsCall,
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected timeout response to be structured: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeExecutionTimeout {
		t.Fatalf("expected execution timeout response, got %+v", response.Error)
	}

	errorsList, err := store.ListErrors(context.Background())
	if err != nil {
		t.Fatalf("expected error query to succeed: %v", err)
	}

	if len(errorsList) != 1 || errorsList[0].Code != rpc.CodeExecutionTimeout {
		t.Fatalf("unexpected persisted execution timeout errors: %+v", errorsList)
	}
}
