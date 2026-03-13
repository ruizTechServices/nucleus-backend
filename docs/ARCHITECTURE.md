# ARCHITECTURE.md

## Architectural summary

Nucleus Backend is a **local, UI-agnostic runtime** implemented as a Go sidecar service.

It should be designed so that the runtime core remains stable while multiple interfaces may connect to it over time.

The runtime is the source of truth for:
- session trust
- policy and permission decisions
- tool availability
- tool execution lifecycle
- audit events
- local persistent operational state

The runtime must not be architecturally dependent on any particular frontend.

---

## Top-level model

```text
Trusted Client (initially Electron)
        │
        │  local IPC + JSON-RPC 2.0
        ▼
Nucleus Sidecar Runtime (Go)
        ├── transport
        ├── auth/session
        ├── policy
        ├── tool registry
        ├── executor
        ├── audit/events
        ├── storage
        └── tool implementations
                ├── filesystem
                ├── terminal
                └── screenshot
```

Future adapters may be added later, but they should connect to the same runtime core rather than reimplementing backend logic.

---

## Request pipeline

This is the canonical logical pipeline and should be preserved:

```text
transport -> auth/session -> policy -> tool registry -> executor
```

### 1. Transport
Responsibilities:
- accept local-only requests
- parse and validate the outer request envelope
- normalize protocol-level input
- forward the request to the runtime pipeline

Transport must not contain business logic or tool implementation details.

Preferred transport:
- Named Pipes on Windows
- Unix Domain Sockets on macOS/Linux

Preferred protocol model:
- JSON-RPC 2.0 style envelopes

### 2. Auth / Session
Responsibilities:
- bootstrap trusted local sessions
- validate caller/session identity
- associate requests with session context
- enforce session expiry or invalidation

The runtime must not accept operational requests from unauthenticated local callers.

### 3. Policy
Responsibilities:
- determine whether an action is allowed, denied, or requires approval
- evaluate tool-level restrictions
- evaluate path, timeout, or command rules
- keep execution decisions explicit and reproducible

Policy must execute before tool execution.

### 4. Tool Registry
Responsibilities:
- define what tools exist
- describe their schemas and metadata
- map tool names to handlers
- expose discoverability to clients

The registry knows *what* can be called, not *whether* a request is allowed in context.

### 5. Executor
Responsibilities:
- invoke the selected tool handler
- enforce timeouts and execution lifecycle
- capture structured results
- emit execution events
- guarantee cleanup behavior where required

The executor must be the single disciplined path through which tools run.

---

## Persistence model

V1 includes two persistence layers:

### 1. Append-only event history
The audit/event layer stores immutable operational events.

Examples:
- session bootstrapped
- tool requested
- tool denied
- approval required
- approval granted/denied
- terminal session started
- terminal command completed
- screenshot captured
- error emitted

Use cases:
- audit history
- debugging
- replay/reconstruction
- trust and observability

### 2. Queryable local state
SQLite stores queryable operational state derived from direct writes and/or event projection.

Examples:
- session metadata
- terminal session metadata
- execution summaries
- recent tool results
- approval records
- error index

Use cases:
- local dashboards/views
- search/filtering
- summaries
- debugging

### Design rule
Persistence is required in V1, but the runtime core should not become tightly coupled to storage implementation details.

Tools should emit structured results and events rather than directly controlling the storage layer.

---

## Tool architecture

Each tool should be a bounded subsystem with:
- input schema
- output schema
- policy hooks
- handler implementation
- event emission
- tests

### V1 tools

#### Filesystem
Initial operations:
- list directory
- read file

Expected constraints:
- normalized paths
- path scope checks
- read-first posture
- detailed error handling

#### Terminal
Model terminal access as a **managed session**, not a raw shell free-for-all.

Recommended operations:
- `terminal.start_session`
- `terminal.exec`
- `terminal.end_session`

Important properties:
- ephemeral lifecycle
- structured command execution
- stdout/stderr/exit code capture
- timeout enforcement
- strong cleanup
- no default persistent shell

#### Screenshot / desktop-state
Read-only perception tool.

Expected properties:
- structured result envelope
- optional metadata about active window/monitor where practical
- limited scope in V1

---

## Terminal session model

Terminal interaction in V1 is intentionally permissive enough to be useful, but controlled enough to remain safe and auditable.

The core primitive is a **managed terminal session**, not a visible terminal window.

A terminal session should support this lifecycle:

```text
start session -> execute commands -> capture results -> end session -> cleanup
```

The runtime may later support a visible terminal adapter if needed, but the core backend model should remain transport- and UI-agnostic.

### Recommended terminal flow
1. Client requests `terminal.start_session`
2. Runtime validates session and policy
3. Runtime creates a terminal session ID and shell process context
4. Client issues one or more `terminal.exec` requests against that session
5. Runtime captures structured execution results for each command
6. Client or runtime ends the session via `terminal.end_session`
7. Runtime terminates the shell process and records closure events

### Why this model
It allows:
- multiple command steps in one task
- strong observability
- strong cleanup behavior
- future UI flexibility
- future protocol adaptability

---

## Storage boundary rule

Tools must not directly depend on SQLite or concrete event-log writers.

Preferred pattern:
- tool returns structured result
- executor/runtime emits structured event(s)
- audit/storage layers persist those events and summaries

This preserves modularity and testability.

---

## Package design guidance

A suitable package structure is expected to resemble:

```text
cmd/
  nucleusd/
internal/
  transport/
  rpc/
  session/
  policy/
  permissions/
  tools/
    filesystem/
    terminal/
    screenshot/
  executor/
  runtime/
  audit/
  storage/
  security/
```

### Boundary guidance
- `transport` should know protocol and IPC details
- `session` should know session bootstrap/validation
- `policy` should evaluate allow/deny/approval decisions
- `tools` should implement bounded capabilities
- `executor` should manage execution lifecycle
- `audit` should define/emit/persist immutable events
- `storage` should manage queryable local persistence
- `runtime` should compose the system

---

## Operating mode in V1

Preferred V1 operating mode:
- runtime is started by a trusted client per application session
- runtime is not installed as a persistent background daemon by default
- runtime exits cleanly when the client session ends or when explicitly requested

This keeps lifecycle reasoning simpler in V1.

---

## Future compatibility goals

The architecture should make future additions possible without rewriting the core runtime:
- additional frontends
- MCP/tool protocol adapters
- richer policy systems
- additional tools
- improved approval workflows
- richer desktop-state/context tools

The runtime core should remain sovereign regardless of which adapter is speaking to it.
