package rpc

import (
	"encoding/json"
	"strings"
)

const (
	CodeParseError              = -32700
	CodeInvalidRequest          = -32600
	CodeMethodNotFound          = -32601
	CodeInvalidBootstrapToken   = 40101
	CodeInvalidSessionToken     = 40102
	CodeSessionExpired          = 40103
	CodePolicyDenied            = 40301
	CodeApprovalRequired        = 40302
	CodeToolNotFound            = 40401
	CodeTerminalSessionNotFound = 40402
	CodeResourceExhausted       = 42901
	CodeExecutionTimeout        = 40801
	CodeValidationError         = 42200
	CodeInternalError           = 50000
	CodeExecutionFailed         = 50001
	CodeRuntimeShuttingDown     = 50301
	CodeStorageError            = 50700
)

type ProtocolError struct {
	Code    int
	Message string
	Data    map[string]any
}

func (e *ProtocolError) Error() string {
	return e.Message
}

func DecodeRequest(payload []byte) (Request, error) {
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return Request{}, &ProtocolError{
			Code:    CodeParseError,
			Message: "failed to parse JSON-RPC request",
		}
	}

	if request.JSONRPC != Version {
		return Request{}, &ProtocolError{
			Code:    CodeInvalidRequest,
			Message: "invalid JSON-RPC version",
			Data: map[string]any{
				"expected": Version,
				"actual":   request.JSONRPC,
			},
		}
	}

	if strings.TrimSpace(request.Method) == "" {
		return Request{}, &ProtocolError{
			Code:    CodeInvalidRequest,
			Message: "missing JSON-RPC method",
		}
	}

	return request, nil
}

func EncodeResponse(response Response) ([]byte, error) {
	if response.JSONRPC == "" {
		response.JSONRPC = Version
	}

	return json.Marshal(response)
}

func NewResultResponse(id any, data any) Response {
	return Response{
		JSONRPC: Version,
		ID:      id,
		Result: &ResultEnvelope{
			OK:   true,
			Data: data,
		},
	}
}

func NewErrorResponse(id any, code int, message string, data map[string]any) Response {
	return Response{
		JSONRPC: Version,
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}
