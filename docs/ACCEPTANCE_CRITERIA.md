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

## Phase 1 — Repository and runtime scaffold

### Required outcomes
- Go module initialized
- `cmd/nucleusd` entrypoint exists
- internal package boundaries exist for core subsystems
- project builds successfully
- README or run instructions exist for the scaffold

### Acceptance checks
- repository compiles without frontend code
- package structure reflects architectural separation
- no generic web-server architecture has replaced the intended sidecar design

---

## Phase 2 — Transport and protocol skeleton

### Required outcomes
- local transport abstraction exists
- JSON-RPC 2.0 style envelope handling exists
- method routing skeleton exists
- `runtime.health` works end-to-end through the request path

### Acceptance checks
- runtime can accept and parse a local request
- runtime returns structured success and structured error responses
- implementation does not require public HTTP exposure
- tests exist for request parsing/routing behavior

---

## Phase 3 — Session bootstrap and validation

### Required outcomes
- `session.bootstrap` exists
- runtime can issue a session token
- runtime can reject requests without a valid session token where required
- session metadata model exists

### Acceptance checks
- unauthenticated tool requests are rejected
- authenticated requests can proceed to later pipeline stages
- session lifecycle behavior is tested
- trust/session state is explicit rather than implicit

---

## Phase 4 — Persistence foundation

### Required outcomes
- append-only event persistence exists
- SQLite-backed queryable local state exists
- event/state schemas are defined for sessions, executions, approvals, and errors
- persistence path is wired into runtime behavior

### Acceptance checks
- session lifecycle events are persisted
- tool execution events are persisted
- terminal session metadata can be queried from SQLite
- storage failures are surfaced explicitly
- tests cover event write and SQLite projection/storage behavior

---

## Phase 5 — Policy engine

### Required outcomes
- allow/deny/approval-required decision model exists
- policy checks occur before tool execution
- policy can evaluate at least:
  - tool-level rules
  - path rules
  - timeout rules
  - command rules or placeholders for them

### Acceptance checks
- denied requests never reach execution
- approval-required requests are surfaced cleanly
- policy decisions are testable and deterministic
- tests exist for allow, deny, and approval-required paths

---

## Phase 6 — Tool registry and executor

### Required outcomes
- tool registry exists
- tools are discoverable through `tools.list`
- executor exists as the disciplined execution path
- executor enforces timeout and captures structured results
- executor emits events

### Acceptance checks
- tools can be discovered with metadata and schemas
- executor behavior is reusable across tools
- execution results use consistent structured envelopes
- tests cover registry lookup and executor lifecycle behavior

---

## Phase 7 — Filesystem tool

### Required outcomes
- `filesystem.list` exists
- `filesystem.read` exists
- path normalization and scoping checks exist
- read-only posture is preserved initially

### Acceptance checks
- allowed directory listing works
- allowed file read works
- out-of-scope path access is denied
- malformed paths are rejected
- executions and denials are persisted as events/state
- tests cover success and failure cases

---

## Phase 8 — Terminal tool

### Required outcomes
- `terminal.start_session` exists
- `terminal.exec` exists
- `terminal.end_session` exists
- terminal session lifecycle is managed and ephemeral
- stdout, stderr, exit code, and duration are captured
- timeout enforcement exists
- cleanup occurs on completion or failure

### Acceptance checks
- terminal session can be created and ended cleanly
- multiple commands can be executed within a managed session
- command timeout behavior is enforced
- session end cleans up backing process resources
- terminal activity is persisted in events and queryable state
- tests cover success, timeout, and cleanup paths

---

## Phase 9 — Screenshot / desktop-state tool

### Required outcomes
- `screenshot.capture` exists
- `desktop.get_state` exists
- both capabilities are modeled as read-only
- structured results are returned
- event persistence exists for usage

### Acceptance checks
- `screenshot.capture` can be invoked successfully in supported environments
- `desktop.get_state` can be invoked successfully in supported environments
- permission/session checks occur before execution
- activity is logged and queryable
- tests cover at least request routing and result schema behavior for both methods
---

## Phase 10 — Runtime hardening

### Required outcomes
- graceful shutdown behavior exists
- cleanup behavior on runtime or client termination is defined
- concurrency/resource limits exist where needed
- failures are surfaced explicitly
- docs reflect implemented behavior

### Acceptance checks
- runtime does not leave obvious orphaned terminal sessions after normal end flows
- runtime returns structured errors under failure conditions
- docs and tests match the implemented contracts
- implementation remains adapter-friendly rather than UI-coupled

---

## Final V1 acceptance checklist

V1 is complete only when all of the following are true:

### Core runtime
- [ ] Go sidecar builds and runs locally
- [ ] local IPC transport is used by default
- [ ] JSON-RPC 2.0 style routing is implemented
- [ ] request pipeline is preserved

### Trust and security
- [ ] session bootstrap exists
- [ ] unauthenticated requests are rejected
- [ ] deny-by-default policy is enforced
- [ ] risky actions can require approval
- [ ] no default internet-facing API is required

### Persistence
- [ ] append-only event history exists
- [ ] SQLite queryable state exists
- [ ] sessions, executions, approvals, and errors are persisted

### Tools
- [ ] filesystem tool works for list/read flows
- [ ] terminal tool supports managed ephemeral sessions
- [ ] screenshot/desktop-state tool works in V1 scope

### Quality
- [ ] tests cover key pipeline and tool behaviors
- [ ] docs are updated to match implementation
- [ ] backend remains frontend-agnostic
- [ ] implementation is modular and suitable for future adapters
