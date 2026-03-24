# ACCEPTANCE_CRITERIA.md

## Purpose

This document defines the acceptance criteria for the Nucleus Backend V1 implementation.

These criteria are intended to keep implementation disciplined and phase-bounded. A phase is not complete merely because code exists; it is complete when the stated acceptance checks are satisfied.

---

## Global acceptance rules

The backend is acceptable only if all of the following remain true:
- it stays backend-only
- it remains implemented in Go
- it preserves the request pipeline
- it is local-only by default
- it uses JSON-RPC 2.0 style request/response envelopes
- it includes local persistence in V1
- it supports the three V1 tools
- it stays documented and testable

---

## Phase 1 - Repository and runtime scaffold

### Required outcomes
- [x] Go module initialized
- [x] `cmd/nucleusd` entrypoint exists
- [x] internal package boundaries exist for core subsystems
- [x] project builds successfully
- [x] README or run instructions exist for the scaffold

### Acceptance checks
- [x] repository compiles without frontend code
- [x] package structure reflects architectural separation
- [x] no generic web-server architecture has replaced the intended sidecar design

---

## Phase 2 - Transport and protocol skeleton

### Required outcomes
- [x] local transport abstraction exists
- [x] JSON-RPC 2.0 style envelope handling exists
- [x] method routing skeleton exists
- [x] `runtime.health` works end-to-end through the request path

### Acceptance checks
- [x] runtime can accept and parse a local request
- [x] runtime returns structured success and structured error responses
- [x] implementation does not require public HTTP exposure
- [x] tests exist for request parsing/routing behavior

---

## Phase 3 - Session bootstrap and validation

### Required outcomes
- [x] `session.bootstrap` exists
- [x] runtime can issue a session token
- [x] runtime can reject requests without a valid session token where required
- [x] session metadata model exists

### Acceptance checks
- [x] unauthenticated tool requests are rejected
- [x] authenticated requests can proceed to later pipeline stages
- [x] session lifecycle behavior is tested
- [x] trust/session state is explicit rather than implicit

---

## Phase 4 - Persistence foundation

### Required outcomes
- [x] append-only event persistence exists
- [x] SQLite-backed queryable local state exists
- [x] event/state schemas are defined for sessions, executions, approvals, and errors
- [x] persistence path is wired into runtime behavior

### Acceptance checks
- [x] session lifecycle events are persisted
- [x] tool execution events are persisted
- [x] terminal session metadata can be queried from SQLite
- [x] storage failures are surfaced explicitly
- [x] tests cover event write and SQLite projection/storage behavior

---

## Phase 5 - Policy engine

### Required outcomes
- [x] allow/deny/approval-required decision model exists
- [x] policy checks occur before tool execution
- [x] policy can evaluate at least:
  - tool-level rules
  - path rules
  - timeout rules
  - command rules or placeholders for them

### Acceptance checks
- [x] denied requests never reach execution
- [x] approval-required requests are surfaced cleanly
- [x] policy decisions are testable and deterministic
- [x] tests exist for allow, deny, and approval-required paths

---

## Phase 6 - Tool registry and executor

### Required outcomes
- [x] tool registry exists
- [x] tools are discoverable through `tools.list`
- [x] executor exists as the disciplined execution path
- [x] executor enforces timeout and captures structured results
- [x] executor emits events

### Acceptance checks
- [x] tools can be discovered with metadata and schemas
- [x] executor behavior is reusable across tools
- [x] execution results use consistent structured envelopes
- [x] tests cover registry lookup and executor lifecycle behavior

---

## Phase 7 - Filesystem tool

### Required outcomes
- [x] `filesystem.list` exists
- [x] `filesystem.read` exists
- [x] path normalization and scoping checks exist
- [x] read-only posture is preserved initially

### Acceptance checks
- [x] allowed directory listing works
- [x] allowed file read works
- [x] out-of-scope path access is denied
- [x] malformed paths are rejected
- [x] executions and denials are persisted as events/state
- [x] tests cover success and failure cases

---

## Phase 8 - Terminal tool

### Required outcomes
- [x] `terminal.start_session` exists
- [x] `terminal.exec` exists
- [x] `terminal.end_session` exists
- [x] terminal session lifecycle is managed and ephemeral
- [x] stdout, stderr, exit code, and duration are captured
- [x] timeout enforcement exists
- [x] cleanup occurs on completion or failure

### Acceptance checks
- [x] terminal session can be created and ended cleanly
- [x] multiple commands can be executed within a managed session
- [x] command timeout behavior is enforced
- [x] session end cleans up backing process resources
- [x] terminal activity is persisted in events and queryable state
- [x] tests cover success, timeout, and cleanup paths

---

## Phase 9 - Screenshot / desktop-state tool

### Required outcomes
- [x] `screenshot.capture` exists
- [x] `desktop.get_state` exists
- [x] both capabilities are modeled as read-only
- [x] structured results are returned
- [x] event persistence exists for usage

### Acceptance checks
- [x] `screenshot.capture` can be invoked successfully in supported environments
- [x] `desktop.get_state` can be invoked successfully in supported environments
- [x] permission/session checks occur before execution
- [x] activity is logged and queryable
- [x] tests cover at least request routing and result schema behavior for both methods

---

## Phase 10 - Runtime hardening

### Required outcomes
- [x] graceful shutdown behavior exists
- [x] cleanup behavior on runtime or client termination is defined
- [x] concurrency/resource limits exist where needed
- [x] failures are surfaced explicitly
- [x] docs reflect implemented behavior

### Acceptance checks
- [x] runtime does not leave obvious orphaned terminal sessions after normal end flows
- [x] runtime returns structured errors under failure conditions
- [x] docs and tests match the implemented contracts
- [x] implementation remains adapter-friendly rather than UI-coupled

---

## Phase 11 - Local IPC transport and runtime composition

### Required outcomes
- [x] local IPC transport is implemented as the default runtime transport
- [x] Windows transport uses Named Pipes
- [x] macOS/Linux transport uses Unix Domain Sockets
- [x] `cmd/nucleusd` composes the concrete runtime stack instead of remaining a scaffold-only entrypoint
- [x] runtime accepts real JSON-RPC requests over local IPC
- [x] startup and shutdown of the composed runtime/transport stack are implemented

### Acceptance checks
- [x] a local client can call `runtime.health` over IPC
- [x] a local client can call `session.bootstrap` over IPC and receive a valid session token
- [x] a local client can call `tools.list` over IPC with an authenticated session
- [x] a local client can call `tools.call` over IPC with an authenticated session
- [x] default operation does not require any public TCP or HTTP listener
- [x] shutdown rejects new requests explicitly while allowing in-flight request handling to follow the runtime lifecycle rules
- [x] tests cover IPC roundtrips, startup, shutdown, and structured error propagation

---

## Final V1 acceptance checklist

V1 is complete only when all of the following are true:

### Core runtime
- [x] Go sidecar builds and runs locally
- [x] local IPC transport is used by default
- [x] JSON-RPC 2.0 style routing is implemented
- [x] request pipeline is preserved

### Trust and security
- [x] session bootstrap exists
- [x] unauthenticated requests are rejected
- [x] deny-by-default policy is enforced
- [x] risky actions can require approval
- [x] no default internet-facing API is required

### Persistence
- [x] append-only event history exists
- [x] SQLite queryable state exists
- [x] sessions, executions, approvals, and errors are persisted

### Tools
- [x] filesystem tool works for list/read flows
- [x] terminal tool supports managed ephemeral sessions
- [x] screenshot/desktop-state tool works in V1 scope

### Quality
- [x] tests cover key pipeline and tool behaviors
- [x] docs are updated to match implementation
- [x] backend remains frontend-agnostic
- [x] implementation is modular and suitable for future adapters
