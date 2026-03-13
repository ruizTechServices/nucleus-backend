# Nucleus Backend

Phase 1 scaffold for the local Nucleus runtime.

This repository contains only the Go backend/runtime sidecar. It does not expose public HTTP endpoints and it does not include any frontend code.

## Current scope

- `cmd/nucleusd` entrypoint scaffold
- internal package boundaries for the runtime subsystems
- placeholder contracts for transport, RPC, session, policy, tools, executor, audit, storage, and runtime composition
- basic tests intended to verify package wiring once Go is available

## Run the scaffold

Install Go 1.22 or newer, then run:

```powershell
go test ./...
```

Later phases will add local IPC transport, JSON-RPC routing, session bootstrap, persistence, policy enforcement, and tool implementations.
