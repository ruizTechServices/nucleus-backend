package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	nucleusruntime "github.com/ruizTechServices/nucleus-backend/internal/runtime"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/transport"
)

type blockingHandler struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (h blockingHandler) Definition() tools.Definition {
	return tools.Definition{Name: "slow.tool", Risk: tools.RiskLow}
}

func (h blockingHandler) Invoke(context.Context, tools.Call) (tools.Result, error) {
	close(h.started)
	<-h.release
	return tools.Result{
		ToolName: "slow.tool",
		Payload: map[string]any{
			"status": "ok",
		},
	}, nil
}

func TestRuntimeShutdownRejectsNewIPCRequestsExplicitly(t *testing.T) {
	endpoint := transport.DefaultEndpoint(fmt.Sprintf("runtime-shutdown-%d", time.Now().UnixNano()))
	listener, err := transport.NewLocalListener(endpoint)
	if err != nil {
		t.Fatalf("expected listener to initialize: %v", err)
	}

	sessionService := session.NewMemoryService(session.Config{
		BootstrapToken: "bootstrap-secret",
	})

	started := make(chan struct{})
	release := make(chan struct{})
	registry := tools.NewStaticRegistry(tools.Entry{
		Handler: blockingHandler{
			started: started,
			release: release,
		},
	})

	rt := nucleusruntime.New(nucleusruntime.Dependencies{
		Transport: listener,
		Sessions:  sessionService,
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"slow.tool": {
					Decision: policy.DecisionAllow,
					Reason:   "slow tool allowed for IPC shutdown test",
				},
			},
		}),
		Registry: registry,
		Executor: executor.NewDefaultRunner(executor.Config{
			GenerateID: func() (string, error) {
				return "exec_slow", nil
			},
		}),
	}, nucleusruntime.DefaultBuildInfo())

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- listener.Serve(context.Background(), rt)
	}()

	client := transport.NewClient(endpoint)

	bootstrapResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_bootstrap",
		Method:  "session.bootstrap",
		Params:  mustMarshal(t, map[string]any{"client_name": "ipc-shutdown", "client_version": "0.1.0", "bootstrap_token": "bootstrap-secret"}),
	})
	if err != nil {
		t.Fatalf("expected bootstrap over IPC to succeed: %v", err)
	}

	sessionToken := bootstrapResponse.Result.Data.(map[string]any)["session_token"].(string)

	firstResponseCh := make(chan rpc.Response, 1)
	firstErrCh := make(chan error, 1)
	go func() {
		response, err := callIPC(t, client, rpc.Request{
			JSONRPC: rpc.Version,
			ID:      "req_slow",
			Method:  "tools.call",
			Params: mustMarshal(t, map[string]any{
				"session_token": sessionToken,
				"tool_name":     "slow.tool",
				"arguments":     map[string]any{},
			}),
		})
		firstResponseCh <- response
		firstErrCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected slow IPC request to start")
	}

	shutdownErrCh := make(chan error, 1)
	go func() {
		shutdownErrCh <- rt.Shutdown(context.Background())
	}()

	shutdownResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_during_shutdown",
		Method:  "runtime.health",
		Params:  mustMarshal(t, map[string]any{}),
	})
	if err != nil {
		t.Fatalf("expected structured shutdown response over IPC: %v", err)
	}

	if shutdownResponse.Error == nil || shutdownResponse.Error.Code != rpc.CodeRuntimeShuttingDown {
		t.Fatalf("expected runtime shutting down error over IPC, got %+v", shutdownResponse.Error)
	}

	close(release)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("expected in-flight IPC request to complete cleanly: %v", err)
	}

	firstResponse := <-firstResponseCh
	if firstResponse.Error != nil {
		t.Fatalf("expected in-flight IPC request success, got %+v", firstResponse.Error)
	}

	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("expected runtime shutdown to complete cleanly: %v", err)
	}

	if err := <-serveErrCh; err != nil {
		t.Fatalf("expected transport serve loop to exit cleanly: %v", err)
	}
}

func mustMarshal(t *testing.T, value any) json.RawMessage {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("expected value to marshal: %v", err)
	}

	return payload
}

func callIPC(t *testing.T, client *transport.Client, request rpc.Request) (rpc.Response, error) {
	t.Helper()

	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.Call(callCtx, request)
}
