package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	nucleusruntime "github.com/ruizTechServices/nucleus-backend/internal/runtime"
	"github.com/ruizTechServices/nucleus-backend/internal/transport"
)

func TestAppSupportsRuntimeMethodsOverIPC(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "hello.txt"), []byte("hello nucleus"), 0o600); err != nil {
		t.Fatalf("expected fixture file to write: %v", err)
	}

	app, err := newApp(appConfig{
		Endpoint:       transport.DefaultEndpoint(fmt.Sprintf("nucleusd-smoke-%d", time.Now().UnixNano())),
		DataDir:        t.TempDir(),
		BootstrapToken: "bootstrap-secret",
		AllowedRoots:   []string{rootDir},
		BuildInfo: nucleusruntime.BuildInfo{
			Service: "nucleusd-test",
			Version: "phase11",
		},
	})
	if err != nil {
		t.Fatalf("expected app to initialize: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.run(runCtx)
	}()

	client := transport.NewClient(app.startup.Endpoint)

	healthResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_health",
		Method:  "runtime.health",
		Params:  mustMarshal(t, map[string]any{}),
	})
	if err != nil {
		t.Fatalf("expected runtime.health over IPC to succeed: %v", err)
	}

	if healthResponse.Error != nil {
		t.Fatalf("expected runtime.health success, got error: %+v", healthResponse.Error)
	}

	healthData := healthResponse.Result.Data.(map[string]any)
	if healthData["service"] != "nucleusd-test" || healthData["version"] != "phase11" {
		t.Fatalf("unexpected health payload: %+v", healthData)
	}

	bootstrapResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_bootstrap",
		Method:  "session.bootstrap",
		Params: mustMarshal(t, map[string]any{
			"client_name":     "ipc-smoke",
			"client_version":  "0.1.0",
			"bootstrap_token": "bootstrap-secret",
		}),
	})
	if err != nil {
		t.Fatalf("expected session.bootstrap over IPC to succeed: %v", err)
	}

	if bootstrapResponse.Error != nil {
		t.Fatalf("expected session.bootstrap success, got error: %+v", bootstrapResponse.Error)
	}

	bootstrapData := bootstrapResponse.Result.Data.(map[string]any)
	sessionToken := bootstrapData["session_token"].(string)
	sessionID := bootstrapData["session_id"].(string)

	statusResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_status",
		Method:  "session.status",
		Params: mustMarshal(t, map[string]any{
			"session_token": sessionToken,
		}),
	})
	if err != nil {
		t.Fatalf("expected session.status over IPC to succeed: %v", err)
	}

	if statusResponse.Error != nil {
		t.Fatalf("expected session.status success, got error: %+v", statusResponse.Error)
	}

	statusData := statusResponse.Result.Data.(map[string]any)
	if statusData["session_id"] != sessionID {
		t.Fatalf("unexpected session.status payload: %+v", statusData)
	}

	toolsListResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools_list",
		Method:  "tools.list",
		Params: mustMarshal(t, map[string]any{
			"session_token": sessionToken,
		}),
	})
	if err != nil {
		t.Fatalf("expected tools.list over IPC to succeed: %v", err)
	}

	if toolsListResponse.Error != nil {
		t.Fatalf("expected tools.list success, got error: %+v", toolsListResponse.Error)
	}

	toolsData := toolsListResponse.Result.Data.(map[string]any)
	toolsList := toolsData["tools"].([]any)
	if len(toolsList) == 0 {
		t.Fatal("expected discoverable tools over IPC")
	}

	toolsCallResponse, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_tools_call",
		Method:  "tools.call",
		Params: mustMarshal(t, map[string]any{
			"session_token": sessionToken,
			"tool_name":     "filesystem.list",
			"arguments": map[string]any{
				"path": rootDir,
			},
		}),
	})
	if err != nil {
		t.Fatalf("expected tools.call over IPC to succeed: %v", err)
	}

	if toolsCallResponse.Error != nil {
		t.Fatalf("expected tools.call success, got error: %+v", toolsCallResponse.Error)
	}

	toolsCallData := toolsCallResponse.Result.Data.(map[string]any)
	if toolsCallData["tool_name"] != "filesystem.list" {
		t.Fatalf("unexpected tools.call payload: %+v", toolsCallData)
	}

	cancel()
	if err := <-runErrCh; err != nil {
		t.Fatalf("expected app run loop to exit cleanly: %v", err)
	}
}

func TestAppPropagatesStructuredRuntimeErrorsOverIPC(t *testing.T) {
	rootDir := t.TempDir()

	app, err := newApp(appConfig{
		Endpoint:       transport.DefaultEndpoint(fmt.Sprintf("nucleusd-errors-%d", time.Now().UnixNano())),
		DataDir:        t.TempDir(),
		BootstrapToken: "bootstrap-secret",
		AllowedRoots:   []string{rootDir},
	})
	if err != nil {
		t.Fatalf("expected app to initialize: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- app.run(runCtx)
	}()

	client := transport.NewClient(app.startup.Endpoint)
	response, err := callIPC(t, client, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_invalid_session",
		Method:  "session.status",
		Params: mustMarshal(t, map[string]any{
			"session_token": "st_invalid",
		}),
	})
	if err != nil {
		t.Fatalf("expected structured IPC error response: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeInvalidSessionToken {
		t.Fatalf("expected invalid session token response, got %+v", response.Error)
	}

	cancel()
	if err := <-runErrCh; err != nil {
		t.Fatalf("expected app run loop to exit cleanly: %v", err)
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
