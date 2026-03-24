package rpc

import (
	"testing"
)

func TestDecodeRequestAcceptsValidEnvelope(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"jsonrpc":"2.0","id":"req_1","method":"runtime.health","params":{}}`))
	if err != nil {
		t.Fatalf("expected valid request to decode: %v", err)
	}

	if request.JSONRPC != Version {
		t.Fatalf("expected JSON-RPC version %q, got %q", Version, request.JSONRPC)
	}

	if request.Method != "runtime.health" {
		t.Fatalf("expected method to be preserved, got %q", request.Method)
	}
}

func TestDecodeRequestRejectsInvalidVersion(t *testing.T) {
	_, err := DecodeRequest([]byte(`{"jsonrpc":"1.0","id":"req_1","method":"runtime.health"}`))
	if err == nil {
		t.Fatal("expected invalid version to be rejected")
	}

	protocolErr, ok := err.(*ProtocolError)
	if !ok {
		t.Fatalf("expected protocol error, got %T", err)
	}

	if protocolErr.Code != CodeInvalidRequest {
		t.Fatalf("expected invalid request code %d, got %d", CodeInvalidRequest, protocolErr.Code)
	}
}

func TestEncodeResponseDefaultsJSONRPCVersion(t *testing.T) {
	encoded, err := EncodeResponse(Response{
		ID: "req_1",
		Result: &ResultEnvelope{
			OK: true,
		},
	})
	if err != nil {
		t.Fatalf("expected response to encode: %v", err)
	}

	if string(encoded) != `{"jsonrpc":"2.0","id":"req_1","result":{"ok":true}}` {
		t.Fatalf("unexpected encoded response: %s", string(encoded))
	}
}
