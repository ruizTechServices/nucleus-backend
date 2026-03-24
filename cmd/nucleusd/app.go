package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ruizTechServices/nucleus-backend/internal/audit"
	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	nucleusruntime "github.com/ruizTechServices/nucleus-backend/internal/runtime"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/storage"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/desktop"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/filesystem"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/screenshot"
	"github.com/ruizTechServices/nucleus-backend/internal/tools/terminal"
	"github.com/ruizTechServices/nucleus-backend/internal/transport"
)

type appConfig struct {
	Endpoint              string
	DataDir               string
	BootstrapToken        string
	AllowedRoots          []string
	MaxConcurrentRequests int
	BuildInfo             nucleusruntime.BuildInfo
}

type app struct {
	runtime   *nucleusruntime.Runtime
	transport transport.Listener
	startup   startupInfo
}

type startupInfo struct {
	Service        string   `json:"service"`
	Version        string   `json:"version"`
	Transport      string   `json:"transport"`
	Endpoint       string   `json:"endpoint"`
	DataDir        string   `json:"data_dir"`
	BootstrapToken string   `json:"bootstrap_token"`
	AllowedRoots   []string `json:"allowed_roots"`
}

func newApp(config appConfig) (*app, error) {
	buildInfo := config.BuildInfo
	if buildInfo.Service == "" || buildInfo.Version == "" {
		buildInfo = nucleusruntime.DefaultBuildInfo()
	}

	bootstrapToken := strings.TrimSpace(config.BootstrapToken)
	if bootstrapToken == "" {
		var err error
		bootstrapToken, err = randomToken("bootstrap")
		if err != nil {
			return nil, err
		}
	}

	allowedRoots, err := resolveAllowedRoots(config.AllowedRoots)
	if err != nil {
		return nil, err
	}

	dataDir, err := resolveDataDir(config.DataDir)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = transport.DefaultEndpoint(buildInfo.Service)
	}

	listener, err := transport.NewLocalListener(endpoint)
	if err != nil {
		return nil, err
	}

	auditSink, err := audit.NewFileSink(filepath.Join(dataDir, "audit.jsonl"))
	if err != nil {
		return nil, err
	}

	stateStore, err := storage.NewSQLiteStore(filepath.Join(dataDir, "state.db"))
	if err != nil {
		_ = auditSink.Close()
		return nil, err
	}

	filesystemService, err := filesystem.NewService(filesystem.Config{
		AllowedRoots: allowedRoots,
	})
	if err != nil {
		_ = stateStore.Close()
		_ = auditSink.Close()
		return nil, err
	}

	terminalService, err := terminal.NewService(terminal.Config{
		DefaultWorkingDirectory: allowedRoots[0],
	})
	if err != nil {
		_ = stateStore.Close()
		_ = auditSink.Close()
		return nil, err
	}

	screenshotService, err := screenshot.NewService(screenshot.Config{
		CaptureDirectory: filepath.Join(dataDir, "captures"),
	})
	if err != nil {
		_ = stateStore.Close()
		_ = auditSink.Close()
		return nil, err
	}

	desktopService := desktop.NewService(desktop.Config{})

	registryEntries := make([]tools.Entry, 0)
	registryEntries = append(registryEntries, filesystemService.Entries()...)
	registryEntries = append(registryEntries, terminalService.Entries()...)
	registryEntries = append(registryEntries, screenshotService.Entries()...)
	registryEntries = append(registryEntries, desktopService.Entries()...)

	policyConfig := policy.DefaultConfig()
	policyConfig.PathRules = allowPathRules(allowedRoots)

	rt := nucleusruntime.New(nucleusruntime.Dependencies{
		Transport: listener,
		Sessions: session.NewMemoryService(session.Config{
			BootstrapToken: bootstrapToken,
		}),
		Policy:                policy.NewStaticEngine(policyConfig),
		Registry:              tools.NewStaticRegistry(registryEntries...),
		Executor:              executor.NewDefaultRunner(executor.Config{}),
		Audit:                 auditSink,
		Storage:               stateStore,
		MaxConcurrentRequests: config.MaxConcurrentRequests,
		Shutdowners:           []nucleusruntime.Shutdowner{terminalService},
	}, buildInfo)

	return &app{
		runtime:   rt,
		transport: listener,
		startup: startupInfo{
			Service:        rt.BuildInfo().Service,
			Version:        rt.BuildInfo().Version,
			Transport:      listener.Network(),
			Endpoint:       endpoint,
			DataDir:        dataDir,
			BootstrapToken: bootstrapToken,
			AllowedRoots:   allowedRoots,
		},
	}, nil
}

func (a *app) run(ctx context.Context) error {
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- a.transport.Serve(context.Background(), a.runtime)
	}()

	select {
	case err := <-serveErrCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		shutdownErr := a.runtime.Shutdown(shutdownCtx)
		serveErr := <-serveErrCh
		return errors.Join(shutdownErr, serveErr)
	}
}

func (a *app) writeStartupInfo(writer io.Writer) error {
	return json.NewEncoder(writer).Encode(a.startup)
}

func loadAppConfigFromEnv() (appConfig, error) {
	maxConcurrentRequests := 0
	if raw := strings.TrimSpace(os.Getenv("NUCLEUS_MAX_CONCURRENT_REQUESTS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return appConfig{}, fmt.Errorf("parse NUCLEUS_MAX_CONCURRENT_REQUESTS: %w", err)
		}
		maxConcurrentRequests = parsed
	}

	return appConfig{
		Endpoint:              strings.TrimSpace(os.Getenv("NUCLEUS_IPC_ENDPOINT")),
		DataDir:               strings.TrimSpace(os.Getenv("NUCLEUS_DATA_DIR")),
		BootstrapToken:        strings.TrimSpace(os.Getenv("NUCLEUS_BOOTSTRAP_TOKEN")),
		AllowedRoots:          splitRoots(os.Getenv("NUCLEUS_ALLOWED_ROOTS")),
		MaxConcurrentRequests: maxConcurrentRequests,
	}, nil
}

func resolveAllowedRoots(roots []string) ([]string, error) {
	filtered := make([]string, 0, len(roots))
	for _, root := range roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}

	if len(filtered) == 0 {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		filtered = []string{workingDirectory}
	}

	return filtered, nil
}

func resolveDataDir(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return filepath.Abs(configured)
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		baseDir = os.TempDir()
	}

	return filepath.Join(baseDir, "nucleus"), nil
}

func allowPathRules(roots []string) []policy.PathRule {
	rules := make([]policy.PathRule, 0, len(roots))
	for _, root := range roots {
		rules = append(rules, policy.PathRule{
			Prefix: filepath.ToSlash(filepath.Clean(root)),
			Rule: policy.Rule{
				Decision: policy.DecisionAllow,
				Reason:   "path is within configured runtime scope",
			},
		})
	}

	return rules
}

func splitRoots(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, string(os.PathListSeparator))
	roots := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			roots = append(roots, trimmed)
		}
	}

	return roots
}

func randomToken(prefix string) (string, error) {
	var payload [16]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}

	return prefix + "_" + hex.EncodeToString(payload[:]), nil
}
