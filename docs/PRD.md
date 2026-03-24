# PRD.md

## Product name

Nucleus Backend (V1)

## Product summary

Nucleus Backend is a **local Go sidecar runtime** that executes controlled computer capabilities on a user's machine for trusted clients.

It is the backend/runtime layer of the Nucleus system. It is intentionally UI-agnostic and must be usable by a native Electron client first, with future support for additional local clients and protocol/tool adapters.

V1 focuses on proving the local-runtime architecture with three foundational capabilities:
- filesystem access
- terminal interaction
- screenshot / desktop-state capture

The backend must be secure, modular, auditable, and extensible.

---

## Problem statement

Most AI-driven computer control projects are architecturally weak because they:
- couple execution logic directly to a single UI
- expose unsafe or overly broad system control
- lack clear policy boundaries
- lack durable audit logs and local observability
- depend on brittle browser hacks or public website behavior
- treat execution, approvals, transport, and storage as one undifferentiated blob

Nucleus Backend exists to solve this by providing a **single local runtime** that can be trusted, audited, and reused across multiple interfaces.

---

## Product vision

Build Nucleus once as the local runtime, then let many interfaces talk to it.

The backend should become the stable local execution layer for:
- a native desktop client
- future protocol/tool adapters
- future external model-aware clients using supported integrations
- future first-party frontends

The runtime must remain the source of truth for:
- tool availability
- permissions and policy
- execution lifecycle
- local audit history
- persistent operational state

---

## Primary goals for V1

1. Prove the backend architecture with a real local runtime.
2. Implement a secure local request pipeline:

```text
transport -> auth/session -> policy -> tool registry -> executor
```

3. Support the first three foundational tools:
   - filesystem
   - terminal
   - screenshot / desktop-state

4. Persist immutable and queryable local operational history.
5. Keep the backend independent from any specific frontend implementation.
6. Make the runtime suitable for future protocol/tool adapters without redesigning the core.

---

## Non-goals for V1

The following are explicitly out of scope for V1 unless later specified:
- frontend implementation
- browser automation
- full desktop automation beyond screenshot/desktop-state
- internet-facing APIs
- cloud-hosted multi-user orchestration
- persistent daemon install as the default operating mode
- provider-specific model orchestration
- unrestricted shell automation
- destructive file operations as a starting point
- plugin marketplace or public extension platform

---

## Target user and usage model

### Primary user
A user running Nucleus locally on their own machine through a trusted local client.

### Operational model
- the user launches a client
- the client starts or connects to the local Nucleus backend
- the runtime returns local startup metadata such as IPC endpoint and bootstrap material to the trusted launcher out of band
- the client authenticates/session-bootstrap with the runtime
- the client discovers available tools
- the client requests tool execution through structured RPC methods
- the runtime evaluates policy and permissions
- the runtime executes or rejects the request
- the runtime emits events and persists history locally

---

## Core capabilities in V1

### 1. Filesystem capability
The runtime must support safe, structured filesystem access.

Initial operations:
- list directory contents
- read file contents

Initial constraints:
- path normalization
- scoped access rules
- deny by default
- strong error handling
- read-first posture

### 2. Terminal capability
The runtime must support managed, ephemeral terminal sessions.

Initial operations:
- start session
- execute command in session
- end session

Initial constraints:
- controlled lifecycle
- structured stdout/stderr/exit metadata
- timeouts
- cleanup guarantees
- no hidden persistent sessions
- no arbitrary background jobs by default

### 3. Screenshot / desktop-state capability
The runtime must support read-only system perception through screenshot capture and related metadata where practical.

Initial constraints:
- read-only
- permission-aware
- limited scope
- structured output

---

## Persistence requirements in V1

V1 must include local persistence for operational observability.

Two persistence modes are required:

### Append-only event history
Store immutable events such as:
- session created
- session ended
- tool requested
- tool allowed/denied
- approval required
- approval granted/denied
- command executed
- command completed
- error emitted

### Queryable local state
Store structured records such as:
- session metadata
- terminal session metadata
- execution summaries
- approvals
- error summaries
- recent operation history

Preferred implementation:
- append-only event log for audit history
- SQLite for queryable state

The runtime should remain operationally resilient if storage is degraded, to the extent safely possible.

---

## Security and trust requirements

The backend must be secure by default.

Core expectations:
- local-only communication by default
- authenticated session bootstrap
- deny-by-default tool access
- explicit policy enforcement before execution
- approval-aware workflow for risky actions
- no raw unrestricted shell exposure
- strong cleanup behavior
- no admin/root requirement by default
- no implicit internet exposure

See `docs/SECURITY_MODEL.md` for the normative security design.

---

## Success criteria for V1

V1 is successful when:
- a trusted local client can start the runtime
- a session can be bootstrapped and validated
- tools can be discovered through structured RPC methods
- the runtime can securely execute the three V1 capabilities
- terminal sessions are managed and ephemeral
- policy and permission checks occur before execution
- event history is persisted locally
- queryable operational state is persisted locally
- the codebase remains modular and adapter-friendly

---

## Quality requirements

The backend should be:
- modular
- testable
- predictable
- secure by default
- documented
- resilient under failure
- explicit in contracts and boundaries

---

## V1 deliverables

The backend V1 should deliver:
- a Go sidecar executable (`nucleusd` or equivalent)
- local IPC transport
- JSON-RPC 2.0 style request routing
- session bootstrap and validation
- policy engine
- tool registry
- executor
- filesystem tool
- terminal session tool
- screenshot/desktop-state tool
- append-only event persistence
- SQLite-backed queryable state
- tests and documentation

### Phase 11 implementation outcome

Phase 11 turns the transport abstraction into the default runtime boundary. The sidecar now uses concrete local IPC transport, with Named Pipes on Windows and Unix Domain Sockets on macOS/Linux, and `cmd/nucleusd` now composes the real runtime stack so a trusted local client can start the sidecar, receive startup metadata, bootstrap a session, and invoke runtime methods over JSON-RPC without any public TCP or HTTP surface. Phase 11 does not add new tool families; it operationalizes the existing runtime through the intended local process boundary.
