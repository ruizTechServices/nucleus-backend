package screenshot

import (
	"context"
	"testing"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type fakeCaptureProvider struct {
	capture func(context.Context, CaptureRequest) (CaptureResponse, error)
}

func (f fakeCaptureProvider) Capture(ctx context.Context, request CaptureRequest) (CaptureResponse, error) {
	return f.capture(ctx, request)
}

func TestServiceInvokesCaptureProvider(t *testing.T) {
	service, err := NewService(Config{
		Provider: fakeCaptureProvider{
			capture: func(_ context.Context, request CaptureRequest) (CaptureResponse, error) {
				return CaptureResponse{
					CaptureID: "cap_test",
					MIMEType:  "image/png",
					Width:     1920,
					Height:    1080,
					Metadata: map[string]any{
						"display_id": request.DisplayID,
					},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected screenshot service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	handler, ok := registry.Lookup(ToolCapture)
	if !ok {
		t.Fatal("expected screenshot.capture handler to be registered")
	}

	result, err := handler.Invoke(context.Background(), tools.Call{
		ToolName: ToolCapture,
		Arguments: map[string]any{
			"display_id": "primary",
		},
	})
	if err != nil {
		t.Fatalf("expected screenshot.capture to succeed: %v", err)
	}

	payload, ok := result.Payload.(CaptureResponse)
	if !ok {
		t.Fatalf("expected capture response payload, got %T", result.Payload)
	}

	if payload.CaptureID != "cap_test" || payload.Metadata["display_id"] != "primary" {
		t.Fatalf("unexpected capture payload: %+v", payload)
	}
}
