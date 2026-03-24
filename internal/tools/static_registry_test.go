package tools

import (
	"context"
	"testing"
)

type fakeHandler struct {
	definition Definition
}

func (f fakeHandler) Definition() Definition {
	return f.definition
}

func (f fakeHandler) Invoke(context.Context, Call) (Result, error) {
	return Result{
		ToolName: f.definition.Name,
	}, nil
}

func TestStaticRegistryListsSortedDefinitions(t *testing.T) {
	registry := NewStaticRegistry(
		Entry{Definition: Definition{Name: "terminal.exec"}},
		Entry{Definition: Definition{Name: "filesystem.read"}},
	)

	definitions := registry.List()
	if len(definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(definitions))
	}

	if definitions[0].Name != "filesystem.read" || definitions[1].Name != "terminal.exec" {
		t.Fatalf("expected sorted definitions, got %+v", definitions)
	}
}

func TestStaticRegistryLookupReturnsRegisteredHandler(t *testing.T) {
	handler := fakeHandler{
		definition: Definition{Name: "filesystem.read"},
	}

	registry := NewStaticRegistry(Entry{Handler: handler})

	lookedUp, ok := registry.Lookup("filesystem.read")
	if !ok {
		t.Fatal("expected registered handler lookup to succeed")
	}

	if lookedUp.Definition().Name != "filesystem.read" {
		t.Fatalf("unexpected handler returned: %+v", lookedUp.Definition())
	}
}
