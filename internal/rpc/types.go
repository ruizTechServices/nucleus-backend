package rpc

import "encoding/json"

const Version = "2.0"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id"`
	Result  *ResultEnvelope `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type ResultEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data,omitempty"`
}

type Error struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}
