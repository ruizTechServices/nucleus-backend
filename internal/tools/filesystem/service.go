package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type Config struct {
	AllowedRoots []string
}

type Service struct {
	allowedRoots []string
}

func NewService(config Config) (*Service, error) {
	if len(config.AllowedRoots) == 0 {
		return nil, fmt.Errorf("filesystem allowed roots are required")
	}

	allowedRoots := make([]string, 0, len(config.AllowedRoots))
	for _, root := range config.AllowedRoots {
		normalizedRoot, err := normalizeRoot(root)
		if err != nil {
			return nil, err
		}

		allowedRoots = append(allowedRoots, normalizedRoot)
	}

	return &Service{
		allowedRoots: allowedRoots,
	}, nil
}

func (s *Service) Entries() []tools.Entry {
	if s == nil {
		return nil
	}

	return []tools.Entry{
		{Handler: listHandler{service: s}},
		{Handler: readHandler{service: s}},
	}
}

type listHandler struct {
	service *Service
}

func (h listHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolList,
		Risk:             tools.RiskMedium,
		RequiresApproval: true,
		InputSchema:      ListRequest{},
		OutputSchema:     ListResponse{},
	}
}

func (h listHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[ListRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	resolvedPath, err := h.service.resolvePath(request.Path)
	if err != nil {
		return tools.Result{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tools.Result{}, pathValidationError("filesystem path does not exist", request.Path)
		}

		return tools.Result{}, fmt.Errorf("stat directory %q: %w", resolvedPath, err)
	}

	if !info.IsDir() {
		return tools.Result{}, pathValidationError("filesystem path must be a directory", request.Path)
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list directory %q: %w", resolvedPath, err)
	}

	responseEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		responseEntries = append(responseEntries, Entry{
			Name:  entry.Name(),
			Path:  displayPath(filepath.Join(resolvedPath, entry.Name())),
			IsDir: entry.IsDir(),
		})
	}

	return tools.Result{
		ToolName: ToolList,
		Payload: ListResponse{
			Path:    displayPath(resolvedPath),
			Entries: responseEntries,
		},
		Metadata: map[string]any{
			"entry_count": len(responseEntries),
		},
	}, nil
}

type readHandler struct {
	service *Service
}

func (h readHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolRead,
		Risk:             tools.RiskMedium,
		RequiresApproval: true,
		InputSchema:      ReadRequest{},
		OutputSchema:     ReadResponse{},
	}
}

func (h readHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[ReadRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	resolvedPath, err := h.service.resolvePath(request.Path)
	if err != nil {
		return tools.Result{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tools.Result{}, pathValidationError("filesystem path does not exist", request.Path)
		}

		return tools.Result{}, fmt.Errorf("stat file %q: %w", resolvedPath, err)
	}

	if info.IsDir() {
		return tools.Result{}, pathValidationError("filesystem path must be a regular file", request.Path)
	}

	if !info.Mode().IsRegular() {
		return tools.Result{}, pathValidationError("filesystem path must be a regular file", request.Path)
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return tools.Result{}, fmt.Errorf("read file %q: %w", resolvedPath, err)
	}

	if !utf8.Valid(content) {
		return tools.Result{}, pathValidationError("filesystem content is not valid utf-8", request.Path)
	}

	return tools.Result{
		ToolName: ToolRead,
		Payload: ReadResponse{
			Path:     displayPath(resolvedPath),
			Content:  string(content),
			Encoding: "utf-8",
		},
		Metadata: map[string]any{
			"size_bytes": len(content),
		},
	}, nil
}

func (s *Service) resolvePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", pathValidationError("filesystem path is required", raw)
	}

	if strings.ContainsRune(trimmed, rune(0)) {
		return "", pathValidationError("filesystem path contains invalid null byte", raw)
	}

	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		return "", pathValidationError("filesystem path must be absolute", raw)
	}

	resolvedPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", pathValidationError("filesystem path could not be normalized", raw)
	}

	if canonicalPath, err := filepath.EvalSymlinks(resolvedPath); err == nil {
		resolvedPath = canonicalPath
	}

	if !s.pathAllowed(resolvedPath) {
		return "", tools.NewDeniedError("filesystem path is outside allowed scope", map[string]any{
			"path": displayPath(cleaned),
		})
	}

	return resolvedPath, nil
}

func (s *Service) pathAllowed(path string) bool {
	for _, root := range s.allowedRoots {
		if pathWithinScope(path, root) {
			return true
		}
	}

	return false
}

func normalizeRoot(root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", fmt.Errorf("filesystem allowed root cannot be empty")
	}

	if strings.ContainsRune(trimmed, rune(0)) {
		return "", fmt.Errorf("filesystem allowed root contains invalid null byte")
	}

	absoluteRoot, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", fmt.Errorf("filesystem allowed root could not be normalized: %w", err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err == nil {
		absoluteRoot = resolvedRoot
	}

	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return "", fmt.Errorf("filesystem allowed root %q is not accessible: %w", absoluteRoot, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("filesystem allowed root %q is not a directory", absoluteRoot)
	}

	return absoluteRoot, nil
}

func pathWithinScope(path string, root string) bool {
	normalizedPath := comparablePath(path)
	normalizedRoot := comparablePath(root)

	if normalizedPath == normalizedRoot {
		return true
	}

	return strings.HasPrefix(normalizedPath, normalizedRoot+"/")
}

func comparablePath(path string) string {
	normalized := filepath.ToSlash(filepath.Clean(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(normalized)
	}

	return normalized
}

func displayPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	return filepath.ToSlash(filepath.Clean(path))
}

func decodeArguments[T any](arguments map[string]any) (T, error) {
	if arguments == nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments are required", nil)
	}

	payload, err := json.Marshal(arguments)
	if err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	return decoded, nil
}

func pathValidationError(message string, path string) error {
	return tools.NewValidationError(message, map[string]any{
		"path": displayPath(path),
	})
}
