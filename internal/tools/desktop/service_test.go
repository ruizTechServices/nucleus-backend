package desktop

import (
	"context"
	"testing"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type fakeStateProvider struct {
	getState func(context.Context) (GetStateResponse, error)
}

func (f fakeStateProvider) GetState(ctx context.Context) (GetStateResponse, error) {
	return f.getState(ctx)
}

func TestServiceInvokesStateProvider(t *testing.T) {
	service := NewService(Config{
		Provider: fakeStateProvider{
			getState: func(context.Context) (GetStateResponse, error) {
				return GetStateResponse{
					ActiveWindow: &ActiveWindow{
						Title:   "Visual Studio Code",
						AppName: "Code",
					},
					Displays: []Display{
						{
							DisplayID: "primary",
							Width:     1920,
							Height:    1080,
						},
					},
				}, nil
			},
		},
	})

	registry := tools.NewStaticRegistry(service.Entries()...)
	handler, ok := registry.Lookup(ToolGetState)
	if !ok {
		t.Fatal("expected desktop.get_state handler to be registered")
	}

	result, err := handler.Invoke(context.Background(), tools.Call{
		ToolName:  ToolGetState,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected desktop.get_state to succeed: %v", err)
	}

	payload, ok := result.Payload.(GetStateResponse)
	if !ok {
		t.Fatalf("expected desktop state payload, got %T", result.Payload)
	}

	if payload.ActiveWindow == nil || payload.ActiveWindow.Title != "Visual Studio Code" {
		t.Fatalf("unexpected desktop state payload: %+v", payload)
	}
}
