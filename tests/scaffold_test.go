package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/desktop"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/filesystem"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/screenshot"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/terminal"
)

func TestRPCRequestUsesJSONRPCVersion(t *testing.T) {
	request := rpc.Request{
		JSONRPC: rpc.Version,
		ID:      "req_1",
		Method:  terminal.ToolExec,
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("expected request to marshal: %v", err)
	}

	if !strings.Contains(string(encoded), "\"jsonrpc\":\"2.0\"") {
		t.Fatalf("expected encoded request to include JSON-RPC version, got %s", string(encoded))
	}
}

func TestRPCRequestSupportsNumericID(t *testing.T) {
	request := rpc.Request{
		JSONRPC: rpc.Version,
		ID:      1,
		Method:  terminal.ToolExec,
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("expected numeric-id request to marshal: %v", err)
	}

	if !strings.Contains(string(encoded), "\"id\":1") {
		t.Fatalf("expected encoded request to include numeric id, got %s", string(encoded))
	}
}

func TestPhaseOneToolContractsExist(t *testing.T) {
	if filesystem.ToolList == "" {
		t.Fatal("expected filesystem list tool name to be defined")
	}

	if filesystem.ToolRead == "" {
		t.Fatal("expected filesystem read tool name to be defined")
	}

	if terminal.ToolStartSession == "" || terminal.ToolExec == "" || terminal.ToolEndSession == "" {
		t.Fatal("expected terminal lifecycle tool names to be defined")
	}

	if screenshot.ToolCapture == "" {
		t.Fatal("expected screenshot capture tool name to be defined")
	}

	if desktop.ToolGetState == "" {
		t.Fatal("expected desktop get_state tool name to be defined")
	}

	_ = screenshot.CaptureRequest{}
	_ = screenshot.CaptureResponse{}
	_ = desktop.GetStateRequest{}
	_ = desktop.GetStateResponse{}
}
