package runtime

import (
	"sync"

	"github.com/ruizTechServices/nucleus-backend/internal/audit"
	"github.com/ruizTechServices/nucleus-backend/internal/executor"
	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
	"github.com/ruizTechServices/nucleus-backend/internal/storage"
	"github.com/ruizTechServices/nucleus-backend/internal/tools"
	"github.com/ruizTechServices/nucleus-backend/internal/transport"
)

const (
	ServiceName = "nucleusd"
	Version     = "0.1.0-dev"

	defaultMaxConcurrentRequests = 32
)

type BuildInfo struct {
	Service string
	Version string
}

type Dependencies struct {
	Transport transport.Listener
	Sessions  session.Service
	Policy    policy.Engine
	Registry  tools.Registry
	Executor  executor.Runner
	Audit     audit.EventSink
	Storage   storage.StateStore

	MaxConcurrentRequests int
	Shutdowners           []Shutdowner
}

type Runtime struct {
	dependencies Dependencies
	buildInfo    BuildInfo

	maxConcurrentRequests int

	mu           sync.Mutex
	inFlight     int
	shuttingDown bool
	closed       bool
}

func DefaultBuildInfo() BuildInfo {
	return BuildInfo{
		Service: ServiceName,
		Version: Version,
	}
}

func New(dependencies Dependencies, buildInfo BuildInfo) *Runtime {
	if buildInfo.Service == "" {
		buildInfo.Service = ServiceName
	}

	if buildInfo.Version == "" {
		buildInfo.Version = Version
	}

	maxConcurrentRequests := dependencies.MaxConcurrentRequests
	if maxConcurrentRequests <= 0 {
		maxConcurrentRequests = defaultMaxConcurrentRequests
	}

	return &Runtime{
		dependencies:          dependencies,
		buildInfo:             buildInfo,
		maxConcurrentRequests: maxConcurrentRequests,
	}
}

func (r *Runtime) Dependencies() Dependencies {
	return r.dependencies
}

func (r *Runtime) BuildInfo() BuildInfo {
	return r.buildInfo
}
