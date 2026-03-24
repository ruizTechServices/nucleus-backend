# Nucleus Backend

Phase 11 local IPC transport and runtime composition for the local Nucleus runtime.

This repository contains only the Go backend/runtime sidecar. It does not expose public HTTP endpoints and it does not include any frontend code.

## Current scope

- concrete `cmd/nucleusd` runtime composition
- internal package boundaries for the runtime subsystems
- JSON-RPC request decoding and response encoding helpers
- runtime method routing skeleton with `runtime.health`
- in-memory session bootstrap and validation flow
- authenticated gating for session-aware operational namespaces
- file-backed append-only audit event persistence
- SQLite-backed queryable state persistence for sessions, terminal sessions, executions, approvals, and errors
- runtime persistence wiring for session bootstrap and authenticated operational requests
- deny-by-default policy engine with allow, deny, and approval-required decisions
- policy evaluation hooks for authenticated operational requests before handler execution
- concrete static tool registry with discoverability and lookup behavior
- concrete executor with timeout enforcement and execution lifecycle events
- `tools.call` routing through auth, policy, registry lookup, and executor execution
- concrete read-only filesystem handlers for `filesystem.list` and `filesystem.read`
- filesystem path normalization, scope enforcement, and structured validation/denial behavior
- concrete managed terminal session handlers for `terminal.start_session`, `terminal.exec`, and `terminal.end_session`
- structured terminal command execution with stdout, stderr, exit code, duration, timeout, and cleanup semantics
- provider-backed read-only handlers for `screenshot.capture` and `desktop.get_state`
- runtime routing and persistence for screenshot and desktop-state usage
- runtime shutdown lifecycle with request draining and dependency cleanup
- configurable runtime concurrency admission limits with structured busy/shutdown responses
- terminal service shutdown cleanup for active sessions and in-flight commands
- default local IPC transport with:
  - Windows Named Pipes
  - macOS/Linux Unix Domain Sockets
- one-request-per-connection framed JSON-RPC transport handling
- startup metadata emission from `cmd/nucleusd` for trusted launcher handoff
- IPC smoke/integration coverage for roundtrips, shutdown, and structured error propagation
- tests for request parsing, routing, persistence, policy, registry, executor, filesystem, terminal, screenshot, and desktop-state behavior

## Run the runtime

Install Go 1.24 or newer, then run:

```powershell
go test ./...
```

To start the composed sidecar locally:

```powershell
$env:NUCLEUS_ALLOWED_ROOTS = (Get-Location).Path
go run ./cmd/nucleusd
```

`cmd/nucleusd` writes a startup JSON document to stdout that includes the local IPC endpoint, bootstrap token, allowed roots, and data directory for the trusted launcher session.

The next step after this backend milestone is client/Electron integration against that local IPC contract.
