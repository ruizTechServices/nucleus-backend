package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/audit"
	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/storage"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/transport"
)

type stubTransport struct{}

func (stubTransport) Serve(context.Context, transport.Handler) error {
	return nil
}

func (stubTransport) Close() error {
	return nil
}

func (stubTransport) Network() string {
	return "local"
}

type trackingTransport struct {
	closeCount int
}

func (*trackingTransport) Serve(context.Context, transport.Handler) error {
	return nil
}

func (t *trackingTransport) Close() error {
	t.closeCount++
	return nil
}

func (*trackingTransport) Network() string {
	return "local"
}

type stubSessionService struct{}

func (stubSessionService) Bootstrap(context.Context, session.BootstrapRequest) (session.Metadata, error) {
	return session.Metadata{}, nil
}

func (stubSessionService) Validate(context.Context, string) (session.Metadata, error) {
	return session.Metadata{}, nil
}

type stubPolicyEngine struct{}

func (stubPolicyEngine) Evaluate(context.Context, policy.Input) (policy.Result, error) {
	return policy.Result{}, nil
}

type stubToolRegistry struct{}

func (stubToolRegistry) List() []tools.Definition {
	return nil
}

func (stubToolRegistry) Lookup(string) (tools.Handler, bool) {
	return nil, false
}

type stubExecutor struct{}

func (stubExecutor) Execute(context.Context, executor.Request) (executor.Result, error) {
	return executor.Result{}, nil
}

type stubAuditSink struct{}

func (stubAuditSink) Append(context.Context, audit.Event) error {
	return nil
}

type trackingAuditSink struct {
	closeCount int
}

func (*trackingAuditSink) Append(context.Context, audit.Event) error {
	return nil
}

func (s *trackingAuditSink) Close() error {
	s.closeCount++
	return nil
}

type stubStateStore struct{}

func (stubStateStore) UpsertSession(context.Context, session.Metadata) error {
	return nil
}

func (stubStateStore) RecordTerminalSession(context.Context, storage.TerminalSessionRecord) error {
	return nil
}

func (stubStateStore) RecordExecution(context.Context, storage.ExecutionRecord) error {
	return nil
}

func (stubStateStore) RecordApproval(context.Context, storage.ApprovalRecord) error {
	return nil
}

func (stubStateStore) RecordError(context.Context, storage.ErrorRecord) error {
	return nil
}

type trackingStateStore struct {
	closeCount int
}

func (*trackingStateStore) UpsertSession(context.Context, session.Metadata) error {
	return nil
}

func (*trackingStateStore) RecordTerminalSession(context.Context, storage.TerminalSessionRecord) error {
	return nil
}

func (*trackingStateStore) RecordExecution(context.Context, storage.ExecutionRecord) error {
	return nil
}

func (*trackingStateStore) RecordApproval(context.Context, storage.ApprovalRecord) error {
	return nil
}

func (*trackingStateStore) RecordError(context.Context, storage.ErrorRecord) error {
	return nil
}

func (s *trackingStateStore) Close() error {
	s.closeCount++
	return nil
}

type stubShutdowner struct {
	shutdown func(context.Context) error
}

func (s stubShutdowner) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
}

type blockingToolHandler struct {
	name    string
	started chan<- struct{}
	waitFor <-chan struct{}
}

func (h blockingToolHandler) Definition() tools.Definition {
	return tools.Definition{Name: h.name, Risk: tools.RiskLow}
}

func (h blockingToolHandler) Invoke(context.Context, tools.Call) (tools.Result, error) {
	if h.started != nil {
		close(h.started)
	}

	<-h.waitFor
	return tools.Result{
		ToolName: h.name,
		Payload: map[string]any{
			"status": "ok",
		},
	}, nil
}

type stubRequestHandler struct{}

func (stubRequestHandler) Handle(context.Context, rpc.Request) (rpc.Response, error) {
	return rpc.Response{}, nil
}

func TestDefaultBuildInfo(t *testing.T) {
	info := DefaultBuildInfo()

	if info.Service != ServiceName {
		t.Fatalf("expected service %q, got %q", ServiceName, info.Service)
	}

	if info.Version != Version {
		t.Fatalf("expected version %q, got %q", Version, info.Version)
	}
}

func TestRuntimeStoresDependencies(t *testing.T) {
	dependencies := Dependencies{
		Transport: stubTransport{},
		Sessions:  stubSessionService{},
		Policy:    stubPolicyEngine{},
		Registry:  stubToolRegistry{},
		Executor:  stubExecutor{},
		Audit:     stubAuditSink{},
		Storage:   stubStateStore{},
	}

	rt := New(dependencies, BuildInfo{
		Service: "nucleusd-test",
		Version: "test",
	})

	if rt.Dependencies().Transport == nil {
		t.Fatal("expected transport dependency to be stored")
	}

	if rt.Dependencies().Sessions == nil {
		t.Fatal("expected session dependency to be stored")
	}

	if rt.Dependencies().Policy == nil {
		t.Fatal("expected policy dependency to be stored")
	}

	if rt.Dependencies().Registry == nil {
		t.Fatal("expected registry dependency to be stored")
	}

	if rt.Dependencies().Executor == nil {
		t.Fatal("expected executor dependency to be stored")
	}

	if rt.Dependencies().Audit == nil {
		t.Fatal("expected audit dependency to be stored")
	}

	if rt.Dependencies().Storage == nil {
		t.Fatal("expected storage dependency to be stored")
	}

	if rt.BuildInfo().Service != "nucleusd-test" {
		t.Fatalf("expected custom service to be preserved, got %q", rt.BuildInfo().Service)
	}

	if rt.BuildInfo().Version != "test" {
		t.Fatalf("expected custom version to be preserved, got %q", rt.BuildInfo().Version)
	}
}

func TestRuntimeContractsRemainUsable(t *testing.T) {
	handler := stubRequestHandler{}
	request := rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_1",
		Method:  "runtime.health",
	}

	if _, err := handler.Handle(context.Background(), request); err != nil {
		t.Fatalf("expected handler contract to be callable: %v", err)
	}

	terminalWindow := 5 * time.Second
	if terminalWindow <= 0 {
		t.Fatal("expected positive duration")
	}
}

func TestRuntimeShutdownDrainsRequestsRejectsNewRequestsAndClosesDependencies(t *testing.T) {
	transportListener := &trackingTransport{}
	auditSink := &trackingAuditSink{}
	stateStore := &trackingStateStore{}
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	registry := tools.NewStaticRegistry(tools.Entry{
		Handler: blockingToolHandler{
			name:    "slow.tool",
			started: requestStarted,
			waitFor: releaseRequest,
		},
	})

	rt := New(Dependencies{
		Transport: transportListener,
		Sessions:  sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"slow.tool": {
					Decision: policy.DecisionAllow,
					Reason:   "slow tool allowed for shutdown test",
				},
			},
		}),
		Registry: registry,
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_shutdown", nil
			},
		}),
		Audit:   auditSink,
		Storage: stateStore,
		Shutdowners: []Shutdowner{
			stubShutdowner{
				shutdown: func(context.Context) error {
					close(releaseRequest)
					return nil
				},
			},
		},
	}, DefaultBuildInfo())

	params, err := json.Marshal(map[string]any{
		"session_token": metadata.SessionToken,
		"tool_name":     "slow.tool",
		"arguments":     map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	responseCh := make(chan rpc.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		response, err := rt.Handle(context.Background(), rpc.Request{
			JSONRPC: rpc.Version,
			ID:      "req_shutdown",
			Method:  "tools.call",
			Params:  params,
		})
		responseCh <- response
		errCh <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected slow request to start")
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected runtime shutdown to succeed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("expected in-flight request to finish without runtime error: %v", err)
	}

	response := <-responseCh
	if response.Error != nil {
		t.Fatalf("expected in-flight request to complete successfully, got %+v", response.Error)
	}

	if transportListener.closeCount != 1 {
		t.Fatalf("expected transport to close once, got %d", transportListener.closeCount)
	}

	if auditSink.closeCount != 1 {
		t.Fatalf("expected audit sink to close once, got %d", auditSink.closeCount)
	}

	if stateStore.closeCount != 1 {
		t.Fatalf("expected state store to close once, got %d", stateStore.closeCount)
	}

	shutdownResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_after_shutdown",
		Method:  "runtime.health",
	})
	if err != nil {
		t.Fatalf("expected post-shutdown response to be structured: %v", err)
	}

	if shutdownResponse.Error == nil || shutdownResponse.Error.Code != rpc.CodeRuntimeShuttingDown {
		t.Fatalf("expected runtime shutting down response, got %+v", shutdownResponse.Error)
	}
}

func TestRuntimeRejectsRequestsWhenConcurrencyLimitIsReached(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	metadata, err := sessionService.Bootstrap(context.Background(), session.BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected session bootstrap to succeed: %v", err)
	}

	registry := tools.NewStaticRegistry(tools.Entry{
		Handler: blockingToolHandler{
			name:    "slow.tool",
			started: requestStarted,
			waitFor: releaseRequest,
		},
	})

	rt := New(Dependencies{
		Sessions:              sessionService,
		MaxConcurrentRequests: 1,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"slow.tool": {
					Decision: policy.DecisionAllow,
					Reason:   "slow tool allowed for concurrency test",
				},
			},
		}),
		Registry: registry,
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_busy", nil
			},
		}),
	}, DefaultBuildInfo())

	params, err := json.Marshal(map[string]any{
		"session_token": metadata.SessionToken,
		"tool_name":     "slow.tool",
		"arguments":     map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected tools.call params to marshal: %v", err)
	}

	firstResponseCh := make(chan rpc.Response, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		response, err := rt.Handle(context.Background(), rpc.Request{
			JSONRPC: rpc.Version,
			ID:      "req_busy_primary",
			Method:  "tools.call",
			Params:  params,
		})
		firstResponseCh <- response
		firstErrCh <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected primary request to start")
	}

	busyResponse, err := rt.Handle(context.Background(), rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_busy_secondary",
		Method:  "runtime.health",
	})
	if err != nil {
		t.Fatalf("expected busy response to be structured: %v", err)
	}

	if busyResponse.Error == nil || busyResponse.Error.Code != rpc.CodeResourceExhausted {
		t.Fatalf("expected resource exhausted response, got %+v", busyResponse.Error)
	}

	close(releaseRequest)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("expected primary request to finish without runtime error: %v", err)
	}

	firstResponse := <-firstResponseCh
	if firstResponse.Error != nil {
		t.Fatalf("expected primary request to complete successfully, got %+v", firstResponse.Error)
	}
}
