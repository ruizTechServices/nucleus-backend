package runtime

import (
	"context"
	"testing"
	"time"

	"nucleus-backend/internal/audit"
	"nucleus-backend/internal/executor"
	"nucleus-backend/internal/policy"
	"nucleus-backend/internal/rpc"
	"nucleus-backend/internal/session"
	"nucleus-backend/internal/storage"
	"nucleus-backend/internal/tools"
	"nucleus-backend/internal/transport"
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

type stubStateStore struct{}

func (stubStateStore) UpsertSession(context.Context, session.Metadata) error {
	return nil
}

func (stubStateStore) RecordExecution(context.Context, storage.ExecutionRecord) error {
	return nil
}

func (stubStateStore) RecordApproval(context.Context, storage.ApprovalRecord) error {
	return nil
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
