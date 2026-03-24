# SECURITY_MODEL.md

## Security objective

Nucleus Backend must be **secure by default**.

The backend is a local execution runtime with access to sensitive computer capabilities. That makes security a primary product requirement, not an optional hardening pass.

The goal of the security model is to ensure that:
- only trusted local clients can use the runtime
- every action is mediated by policy
- risky capabilities remain bounded and observable
- execution is auditable
- the runtime does not accidentally become a wide-open local attack surface

---

## Security principles

### 1. Local-first, local-only by default
The runtime must not default to public or semi-public network exposure.

Preferred transport:
- Named Pipes on Windows
- Unix Domain Sockets on macOS/Linux

Avoid defaulting to:
- open TCP listeners
- public HTTP APIs
- internet-reachable services

### 2. Deny by default
No tool should execute unless:
- the request is valid
- the session is trusted
- policy allows it
- any required approval is granted

### 3. Explicit mediation
All tool execution must pass through the canonical request pipeline:

```text
transport -> auth/session -> policy -> tool registry -> executor
```

No subsystem should bypass policy.

Current implementation status:
- authenticated operational requests are evaluated by a deny-by-default policy engine before handler execution continues
- the current engine supports allow, deny, and approval-required decisions
- policy evaluation currently covers tool/action rules plus path, timeout, and command attributes

### 4. Least privilege
The runtime should run as the current user without requiring admin/root by default.

Do not assume elevated privileges.

### 5. Observability
Security-relevant actions must be visible in local persistent history.

### 6. Bounded capabilities
Capabilities such as terminal and filesystem access must be intentionally shaped and constrained.

### 7. Clean lifecycle management
Short-lived sessions, explicit cleanup, and strong teardown behavior are part of the security model.

---

## Trust model

### Trusted client assumption
In V1, the primary trusted client is a first-party local client that starts or connects to the runtime.

The runtime must not assume that every local process is trusted just because it is on the same machine.

### Session bootstrap
The runtime should require a trusted bootstrap handshake before accepting operational requests.

Suggested model:
1. trusted client starts the runtime
2. trusted client provides a short-lived bootstrap secret or equivalent handshake material
3. runtime validates and creates a session
4. runtime issues an opaque session token
5. all subsequent operational requests require that token

### Session scope
Sessions should have explicit metadata such as:
- session ID
- trust level
- created/expiry timestamps
- approved scopes
- client identity metadata

---

## Threat considerations

The backend should be designed with at least the following local threats in mind:
- unauthorized local processes attempting to call the runtime
- accidental overexposure through localhost/network transport
- unsafe shell invocation patterns
- path traversal or scope escape in filesystem operations
- uncontrolled long-running child processes
- lack of cleanup after failures
- runtime drift where tools become more permissive than intended
- silent security regressions due to undocumented behavior changes

---

## Filesystem security rules

### Required controls
- normalize and validate all paths
- reject malformed or ambiguous paths
- apply path scope checks before execution
- prefer read-only capability first
- deny sensitive path access unless explicitly allowed

### V1 posture
Start with:
- list directory
- read file

Avoid starting with:
- delete file
- move file
- arbitrary write surfaces
- recursive destructive operations

Current implementation status:
- `filesystem.list` and `filesystem.read` execute through the standard auth -> policy -> registry -> executor pipeline
- tool handlers require absolute paths, normalize them before access, and enforce configured allowed-root scope checks
- out-of-scope access returns a structured denial and malformed paths return structured validation errors

---

## Terminal security rules

Terminal access is intentionally supported in V1, but must be tightly modeled.

### Core rule
Terminal access must use a **managed terminal session** abstraction.

### Required controls
- explicit session start
- structured per-command execution
- timeout enforcement
- stdout/stderr/exit code capture
- lifecycle cleanup
- event logging
- policy checks on command execution

### Strong defaults
- no persistent terminal session by default
- no arbitrary background jobs by default
- no invisible long-lived shell survival after task completion
- no unrestricted raw shell passthrough by default

### Additional guidance
The backend should avoid naïve shell invocation patterns that create excessive injection or composition risk.

Even if command input remains string-based initially, execution must remain:
- observable
- timeout-bound
- policy-aware
- session-scoped

Current implementation status:
- terminal lifecycle is implemented with `terminal.start_session`, `terminal.exec`, and `terminal.end_session`
- terminal sessions are managed in-memory and scoped to the authenticated runtime session that created them
- commands execute directly as binaries/arguments rather than through `cmd /c`, `powershell -Command`, `sh -c`, or similar shell wrappers
- `terminal.end_session` cancels active command contexts so session cleanup does not leave intentional long-running commands behind by default
- terminal service shutdown also cancels active command contexts and marks live in-memory sessions ended when the runtime is terminating

---

## Screenshot / desktop-state security rules

### Required controls
- treat as read-only capability
- require session validation
- log captures in event history
- constrain scope where possible

V1 should avoid expanding this into broad desktop control.

Current implementation status:
- `screenshot.capture` and `desktop.get_state` execute through the standard auth -> policy -> registry -> executor pipeline
- both capabilities are read-only and return structured metadata/results rather than control primitives
- screenshot usage and desktop-state usage are persisted in audit history and execution state
- the built-in adapters currently target Windows; unsupported platforms require a different provider or return execution failure

---

## Approval model

Some actions may be:
- allowed automatically
- denied automatically
- blocked pending approval

Approval should be a first-class concept, not an ad hoc UI-only behavior.

Approval records should be persisted.

Examples of actions likely to require approval in V1:
- terminal session start
- high-risk command execution
- access outside previously approved filesystem scope
- screenshot capture if not preapproved

Current implementation status:
- approval-required policy decisions are surfaced as structured runtime errors with approval metadata
- approval-required requests are also persisted in local state and audit storage when those persistence layers are configured

---

## Persistence and security

Persistence is part of the security model because it provides auditability.

### Event log
The append-only event log should record security-relevant operations such as:
- bootstrap attempts
- session creation and closure
- tool requests
- policy deny decisions
- approval-required events
- approval decisions
- terminal command completion/failure
- storage failures

### SQLite state
SQLite should support local review of:
- sessions
- terminal sessions
- execution records
- errors
- approvals

### Design rule
A failure to persist should be surfaced explicitly. It should not silently disappear.

---

## Transport security guidance

Preferred:
- local IPC over OS-native mechanisms
- authenticated session handshake
- no broad bind/listen surfaces by default

Avoid:
- unauthenticated localhost APIs
- public ports
- silent fallback to insecure transport

Current implementation status:
- local IPC is the default runtime transport
- Windows uses Named Pipes with local-only rejection of remote pipe clients
- macOS/Linux use Unix Domain Sockets with local socket file hygiene and stale socket cleanup
- the runtime does not fall back to localhost TCP or public HTTP listeners for convenience
- `cmd/nucleusd` emits endpoint/bootstrap startup metadata to the trusted launcher over process stdout rather than network discovery
- the transport cooperates with runtime shutdown so new requests receive structured shutdown responses before final listener close

---

## Process and child-process management

The runtime must treat child process management as a security concern.

Required goals:
- enforce timeout/termination policies
- clean up terminal/session child processes
- prevent orphaned execution where practical
- avoid hidden background execution by default

The runtime should not leave behind long-running shell processes after terminal sessions end.

Current implementation status:
- runtime shutdown stops accepting new requests, invokes configured shutdown hooks, waits for in-flight requests to drain, and closes persistence dependencies afterward
- graceful IPC listeners stay available during drain so new callers can receive a structured `50301` shutdown response before transport close
- runtime admission is bounded by a configurable maximum number of concurrent in-flight requests
- overload and shutdown conditions are surfaced through structured RPC errors rather than silent drops

---

## Dependency posture

Use a minimal, justified dependency set.

Security-sensitive behavior should not rely on a large stack of unnecessary third-party libraries when the Go standard library or small focused libraries suffice.

---

## Secure development rules

When changing security-relevant code:
- update tests
- update this document if behavior changes
- avoid convenience shortcuts that widen privileges
- preserve deny-by-default semantics
- document any new execution surface explicitly

---

## V1 security definition

For V1, “secure” means at minimum:
- local-only IPC by default
- authenticated trusted-client bootstrap
- deny-by-default policy enforcement
- no admin/root requirement by default
- managed terminal sessions instead of unrestricted shell exposure
- strong timeout and cleanup behavior
- persisted audit history
- queryable local execution/error state
- explicit approval pathways for risky actions
- documented behavior and contracts
