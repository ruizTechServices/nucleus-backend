package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	nucleusruntime "github.com/ruizTechServices/nucleus-backend/internal/runtime"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

func TestRuntimeHealthRoutesThroughCodecAndHandler(t *testing.T) {
	rawRequest := []byte(`{"jsonrpc":"2.0","id":"req_1","method":"runtime.health","params":{}}`)

	request, err := rpc.DecodeRequest(rawRequest)
	if err != nil {
		t.Fatalf("expected request to decode: %v", err)
	}

	rt := nucleusruntime.New(nucleusruntime.Dependencies{}, nucleusruntime.BuildInfo{
		Service: "nucleusd-test",
		Version: "0.2.0",
	})

	response, err := rt.Handle(t.Context(), request)
	if err != nil {
		t.Fatalf("expected handler to return a structured response: %v", err)
	}

	encoded, err := rpc.EncodeResponse(response)
	if err != nil {
		t.Fatalf("expected response to encode: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("expected response JSON to unmarshal: %v", err)
	}

	if decoded["jsonrpc"] != rpc.Version {
		t.Fatalf("expected response JSON-RPC version %q, got %v", rpc.Version, decoded["jsonrpc"])
	}

	result, ok := decoded["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result envelope, got %T", decoded["result"])
	}

	if okValue, ok := result["ok"].(bool); !ok || !okValue {
		t.Fatalf("expected result envelope ok=true, got %v", result["ok"])
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected health data payload, got %T", result["data"])
	}

	if data["service"] != "nucleusd-test" || data["version"] != "0.2.0" || data["status"] != "ok" {
		t.Fatalf("unexpected health payload: %+v", data)
	}
}

func TestSessionBootstrapThenToolsListRoute(t *testing.T) {
	rt := nucleusruntime.New(nucleusruntime.Dependencies{
		Sessions: session.NewMemoryService(session.Config{
			BootstrapToken: "bootstrap-secret",
			Now: func() time.Time {
				return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
			},
		}),
		Policy: policy.NewStaticEngine(policy.Config{
			ActionRules: map[string]policy.Rule{
				"tools.list": {
					Decision: policy.DecisionAllow,
					Reason:   "tool discovery is allowed",
				},
			},
		}),
	}, nucleusruntime.DefaultBuildInfo())

	bootstrapRequest, err := rpc.DecodeRequest([]byte(`{
		"jsonrpc":"2.0",
		"id":"req_bootstrap",
		"method":"session.bootstrap",
		"params":{
			"client_name":"nucleus-electron",
			"client_version":"0.1.0",
			"bootstrap_token":"bootstrap-secret"
		}
	}`))
	if err != nil {
		t.Fatalf("expected bootstrap request to decode: %v", err)
	}

	bootstrapResponse, err := rt.Handle(t.Context(), bootstrapRequest)
	if err != nil {
		t.Fatalf("expected bootstrap handler response: %v", err)
	}

	encodedBootstrap, err := rpc.EncodeResponse(bootstrapResponse)
	if err != nil {
		t.Fatalf("expected bootstrap response to encode: %v", err)
	}

	var bootstrapEnvelope map[string]any
	if err := json.Unmarshal(encodedBootstrap, &bootstrapEnvelope); err != nil {
		t.Fatalf("expected bootstrap response to unmarshal: %v", err)
	}

	bootstrapResult := bootstrapEnvelope["result"].(map[string]any)
	bootstrapData := bootstrapResult["data"].(map[string]any)
	sessionToken := bootstrapData["session_token"].(string)

	toolsRequest, err := rpc.DecodeRequest([]byte(`{
		"jsonrpc":"2.0",
		"id":"req_tools",
		"method":"tools.list",
		"params":{
			"session_token":"` + sessionToken + `"
		}
	}`))
	if err != nil {
		t.Fatalf("expected tools request to decode: %v", err)
	}

	toolsResponse, err := rt.Handle(t.Context(), toolsRequest)
	if err != nil {
		t.Fatalf("expected tools.list handler response: %v", err)
	}

	if toolsResponse.Error != nil {
		t.Fatalf("expected authenticated tools.list request to succeed, got error: %+v", toolsResponse.Error)
	}
}
