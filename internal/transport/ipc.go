package transport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
)

const maxFrameSize = 16 << 20

type Client struct {
	endpoint string
}

func NewClient(endpoint string) *Client {
	return &Client{endpoint: endpoint}
}

func (c *Client) RoundTrip(ctx context.Context, payload []byte) ([]byte, error) {
	if c == nil || strings.TrimSpace(c.endpoint) == "" {
		return nil, fmt.Errorf("transport client endpoint is required")
	}

	connection, err := dialEndpoint(ctx, c.endpoint)
	if err != nil {
		return nil, err
	}
	defer connection.Close()

	if err := writeFrame(connection, payload); err != nil {
		return nil, err
	}

	return readFrame(connection)
}

func (c *Client) Call(ctx context.Context, request rpc.Request) (rpc.Response, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return rpc.Response{}, err
	}

	responsePayload, err := c.RoundTrip(ctx, payload)
	if err != nil {
		return rpc.Response{}, err
	}

	var response rpc.Response
	if err := json.Unmarshal(responsePayload, &response); err != nil {
		return rpc.Response{}, err
	}

	return response, nil
}

func sanitizeName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "nucleusd"
	}

	var builder strings.Builder
	for _, ch := range trimmed {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '_' || ch == '-' || ch == '.':
			builder.WriteRune(ch)
		default:
			builder.WriteRune('_')
		}
	}

	return builder.String()
}

func handleConnection(connection io.ReadWriteCloser, handler Handler) {
	defer connection.Close()

	requestPayload, err := readFrame(connection)
	if err != nil {
		return
	}

	responsePayload, err := handlePayload(requestPayload, handler)
	if err != nil {
		fallback, encodeErr := rpc.EncodeResponse(rpc.NewErrorResponse(nil, rpc.CodeInternalError, "transport response encoding failed", map[string]any{
			"reason": err.Error(),
		}))
		if encodeErr == nil {
			_ = writeFrame(connection, fallback)
		}
		return
	}

	_ = writeFrame(connection, responsePayload)
}

func handlePayload(payload []byte, handler Handler) ([]byte, error) {
	request, err := rpc.DecodeRequest(payload)
	if err != nil {
		if protocolErr, ok := err.(*rpc.ProtocolError); ok {
			return rpc.EncodeResponse(rpc.NewErrorResponse(nil, protocolErr.Code, protocolErr.Message, protocolErr.Data))
		}

		return nil, err
	}

	response, err := handler.Handle(context.Background(), request)
	if err != nil {
		response = rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "transport handler failure", map[string]any{
			"reason": err.Error(),
		})
	}

	return rpc.EncodeResponse(response)
}

func readFrame(reader io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return nil, err
	}

	if size == 0 {
		return nil, fmt.Errorf("transport frame cannot be empty")
	}

	if size > maxFrameSize {
		return nil, fmt.Errorf("transport frame exceeds %d bytes", maxFrameSize)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func writeFrame(writer io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("transport frame payload is required")
	}

	if len(payload) > maxFrameSize {
		return fmt.Errorf("transport frame exceeds %d bytes", maxFrameSize)
	}

	if err := binary.Write(writer, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}

	_, err := writer.Write(payload)
	return err
}
