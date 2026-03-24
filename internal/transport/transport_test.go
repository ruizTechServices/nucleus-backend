package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
)

type stubHandler struct {
	handle func(context.Context, rpc.Request) (rpc.Response, error)
}

func (s stubHandler) Handle(ctx context.Context, request rpc.Request) (rpc.Response, error) {
	return s.handle(ctx, request)
}

func TestLocalTransportRoundTrip(t *testing.T) {
	endpoint := DefaultEndpoint(fmt.Sprintf("transport-roundtrip-%d", time.Now().UnixNano()))
	listener, err := NewLocalListener(endpoint)
	if err != nil {
		t.Fatalf("expected local listener to initialize: %v", err)
	}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- listener.Serve(context.Background(), stubHandler{
			handle: func(_ context.Context, request rpc.Request) (rpc.Response, error) {
				return rpc.NewResultResponse(request.ID, map[string]any{
					"method": request.Method,
				}), nil
			},
		})
	}()

	client := NewClient(endpoint)
	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("expected params to marshal: %v", err)
	}

	response, err := client.Call(callCtx, rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_transport_roundtrip",
		Method:  "runtime.health",
		Params:  params,
	})
	if err != nil {
		t.Fatalf("expected transport roundtrip to succeed: %v", err)
	}

	if response.Error != nil {
		t.Fatalf("expected successful response, got error: %+v", response.Error)
	}

	data, ok := response.Result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data map, got %T", response.Result.Data)
	}

	if data["method"] != "runtime.health" {
		t.Fatalf("unexpected transport data payload: %+v", data)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("expected listener to close cleanly: %v", err)
	}

	if err := <-serveErrCh; err != nil {
		t.Fatalf("expected listener serve loop to exit cleanly: %v", err)
	}
}

func TestLocalTransportPropagatesStructuredParseErrors(t *testing.T) {
	endpoint := DefaultEndpoint(fmt.Sprintf("transport-parse-%d", time.Now().UnixNano()))
	listener, err := NewLocalListener(endpoint)
	if err != nil {
		t.Fatalf("expected local listener to initialize: %v", err)
	}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- listener.Serve(context.Background(), stubHandler{
			handle: func(_ context.Context, request rpc.Request) (rpc.Response, error) {
				return rpc.NewResultResponse(request.ID, nil), nil
			},
		})
	}()

	client := NewClient(endpoint)
	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	responsePayload, err := client.RoundTrip(callCtx, []byte(`{"jsonrpc":"2.0","id":"bad","method":`))
	if err != nil {
		t.Fatalf("expected malformed request to receive a structured response: %v", err)
	}

	var response rpc.Response
	if err := json.Unmarshal(responsePayload, &response); err != nil {
		t.Fatalf("expected response payload to decode: %v", err)
	}

	if response.Error == nil || response.Error.Code != rpc.CodeParseError {
		t.Fatalf("expected parse error response, got %+v", response.Error)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("expected listener to close cleanly: %v", err)
	}

	if err := <-serveErrCh; err != nil {
		t.Fatalf("expected listener serve loop to exit cleanly: %v", err)
	}
}
