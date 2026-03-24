package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

func TestServiceListAndReadWithinAllowedRoot(t *testing.T) {
	root := t.TempDir()
	nestedDir := filepath.Join(root, "nested")
	if err := os.Mkdir(nestedDir, 0o755); err != nil {
		t.Fatalf("expected nested directory to be created: %v", err)
	}

	filePath := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello nucleus"), 0o644); err != nil {
		t.Fatalf("expected text fixture to be written: %v", err)
	}

	service, err := NewService(Config{
		AllowedRoots: []string{root},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	listRegistry := tools.NewStaticRegistry(service.Entries()...)
	listHandler, ok := listRegistry.Lookup(ToolList)
	if !ok {
		t.Fatal("expected filesystem.list handler to be registered")
	}

	listResult, err := listHandler.Invoke(context.Background(), tools.Call{
		ToolName: ToolList,
		Arguments: map[string]any{
			"path": root,
		},
	})
	if err != nil {
		t.Fatalf("expected filesystem.list to succeed: %v", err)
	}

	listResponse, ok := listResult.Payload.(ListResponse)
	if !ok {
		t.Fatalf("expected list response payload, got %T", listResult.Payload)
	}

	if listResponse.Path != filepath.ToSlash(root) {
		t.Fatalf("expected normalized list path %q, got %q", filepath.ToSlash(root), listResponse.Path)
	}

	if len(listResponse.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %+v", listResponse.Entries)
	}

	readHandler, ok := listRegistry.Lookup(ToolRead)
	if !ok {
		t.Fatal("expected filesystem.read handler to be registered")
	}

	readResult, err := readHandler.Invoke(context.Background(), tools.Call{
		ToolName: ToolRead,
		Arguments: map[string]any{
			"path": filePath,
		},
	})
	if err != nil {
		t.Fatalf("expected filesystem.read to succeed: %v", err)
	}

	readResponse, ok := readResult.Payload.(ReadResponse)
	if !ok {
		t.Fatalf("expected read response payload, got %T", readResult.Payload)
	}

	if readResponse.Path != filepath.ToSlash(filePath) {
		t.Fatalf("expected normalized read path %q, got %q", filepath.ToSlash(filePath), readResponse.Path)
	}

	if readResponse.Content != "hello nucleus" || readResponse.Encoding != "utf-8" {
		t.Fatalf("unexpected read response payload: %+v", readResponse)
	}
}

func TestServiceRejectsMalformedRelativePath(t *testing.T) {
	service, err := NewService(Config{
		AllowedRoots: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	handler, ok := registry.Lookup(ToolRead)
	if !ok {
		t.Fatal("expected filesystem.read handler to be registered")
	}

	_, err = handler.Invoke(context.Background(), tools.Call{
		ToolName: ToolRead,
		Arguments: map[string]any{
			"path": "..\\relative.txt",
		},
	})
	if err == nil {
		t.Fatal("expected relative path to be rejected")
	}

	var callErr *tools.CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("expected tools.CallError, got %T", err)
	}

	if callErr.Kind != tools.ErrorKindValidation {
		t.Fatalf("expected validation error kind, got %q", callErr.Kind)
	}
}

func TestServiceRejectsOutOfScopePath(t *testing.T) {
	allowedRoot := t.TempDir()
	deniedRoot := t.TempDir()
	deniedPath := filepath.Join(deniedRoot, "secret.txt")
	if err := os.WriteFile(deniedPath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("expected denied fixture to be written: %v", err)
	}

	service, err := NewService(Config{
		AllowedRoots: []string{allowedRoot},
	})
	if err != nil {
		t.Fatalf("expected filesystem service to initialize: %v", err)
	}

	registry := tools.NewStaticRegistry(service.Entries()...)
	handler, ok := registry.Lookup(ToolRead)
	if !ok {
		t.Fatal("expected filesystem.read handler to be registered")
	}

	_, err = handler.Invoke(context.Background(), tools.Call{
		ToolName: ToolRead,
		Arguments: map[string]any{
			"path": deniedPath,
		},
	})
	if err == nil {
		t.Fatal("expected out-of-scope path to be rejected")
	}

	var callErr *tools.CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("expected tools.CallError, got %T", err)
	}

	if callErr.Kind != tools.ErrorKindDenied {
		t.Fatalf("expected denied error kind, got %q", callErr.Kind)
	}
}
