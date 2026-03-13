package runtime

import (
	"nucleus-backend/internal/audit"
	"nucleus-backend/internal/executor"
	"nucleus-backend/internal/policy"
	"nucleus-backend/internal/session"
	"nucleus-backend/internal/storage"
	"nucleus-backend/internal/tools"
	"nucleus-backend/internal/transport"
)

const (
	ServiceName = "nucleusd"
	Version     = "0.1.0-dev"
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
}

type Runtime struct {
	dependencies Dependencies
	buildInfo    BuildInfo
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

	return &Runtime{
		dependencies: dependencies,
		buildInfo:    buildInfo,
	}
}

func (r *Runtime) Dependencies() Dependencies {
	return r.dependencies
}

func (r *Runtime) BuildInfo() BuildInfo {
	return r.buildInfo
}
