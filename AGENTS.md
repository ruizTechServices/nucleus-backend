# AGENTS.md

## Repository purpose

This repository contains the **backend/runtime only** for **Nucleus**, a local AI runtime implemented as a **Go sidecar service**.

Nucleus is **not** a web backend, SaaS API, or frontend application in this repository.

The backend is responsible for:
- running as a **local sidecar/runtime** on the user's machine
- exposing a **local-only**, structured tool surface for trusted clients
- enforcing **authentication/session**, **policy**, **permissions**, and **execution safety**
- executing controlled local capabilities such as **filesystem**, **terminal**, and **screenshot/desktop state**
- persisting **append-only events** and **queryable local state** for auditability and debugging

The backend is **not** responsible for:
- Electron UI implementation
- web frontend implementation
- remote cloud APIs
- public internet-facing HTTP services
- multi-user SaaS account systems
- model-provider orchestration logic in V1 unless explicitly added later

Read these files before making changes:
- `docs/PRD.md`
- `docs/ARCHITECTURE.md`
- `docs/RPC_SCHEMA.md`
- `docs/SECURITY_MODEL.md`
- `docs/ACCEPTANCE_CRITERIA.md`

Do not start coding until you have read them.

---

## Product definition

Nucleus backend is a **local, sovereign execution runtime**.

It must support:
- one or more trusted local clients
- one runtime core
- many future adapter surfaces

The architectural principle is:

> Build Nucleus once as the local runtime, then let many interfaces talk to it.

For V1, this repository implements only the backend/runtime needed to support that direction.

---

## Non-negotiable constraints

1. **Backend only**
   - Do not add frontend assets, Electron renderer code, HTML, CSS, React, Vue, or Tailwind.
   - UI concerns should be represented only through interfaces/events/contracts where necessary.

2. **Go sidecar runtime**
   - The backend must be implemented in Go.
   - Prefer the Go standard library where practical.
   - Keep dependencies lean and justified.

3. **Local-first, local-only by default**
   - Default communication must be **OS-native local IPC**, not public network exposure.
   - Prefer:
     - **Named Pipes** on Windows
     - **Unix Domain Sockets** on macOS/Linux
   - Do not default to localhost TCP/HTTP listeners.

4. **JSON-RPC 2.0 request model**
   - Use JSON-RPC 2.0 style request/response envelopes internally.
   - The RPC contract is defined in `docs/RPC_SCHEMA.md`.

5. **Strict request pipeline**
   - Preserve this logical flow:

```text
transport -> auth/session -> policy -> tool registry -> executor
```

   - Do not bypass policy checks.
   - Do not let tools call transport/UI/storage directly.

6. **Deny by default**
   - Every tool/action must be denied unless explicitly allowed by policy.
   - If approval is required, surface that requirement through structured events/results.

7. **Ephemeral terminal sessions**
   - Terminal access is allowed in V1, but must be controlled.
   - No persistent shell sessions by default.
   - No arbitrary background jobs by default.
   - No unrestricted `cmd /c`, `powershell -Command`, `sh -c`, or chained shell composition by default unless explicitly and safely modeled.
   - Terminal interaction must use a **managed terminal session** abstraction, not a raw unsupervised shell.

8. **Persistence exists in V1**
   - V1 must include:
     - append-only event persistence
     - queryable local state persistence
   - Prefer:
     - append-only event log for immutable audit history
     - SQLite for queryable state/read models
   - Runtime execution should remain as DB-independent as practical; storage failures should not silently corrupt runtime logic.

9. **Security over convenience**
   - No admin/root requirement by default.
   - No hidden background execution.
   - No wide-open execution surfaces.
   - No undocumented control channels.

10. **Modular separation of concerns**
   - Keep transport, session/auth, policy, tool registry, executor, tools, storage, and events separate.
   - Avoid fat handlers and mixed responsibilities.

---

## Expected repository structure

Use or preserve a structure close to this:

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
schemas/
tests/
docs/
```

This is guidance, not a prison. However, preserve the architectural boundaries.

---

## Initial V1 tools

The first-class V1 tools are:
1. `filesystem`
2. `terminal`
3. `screenshot` / desktop-state

### Filesystem tool constraints
- Start with read-only operations first.
- Prioritize:
  - list directory
  - read file
- Do not begin with destructive operations.
- Enforce path normalization and directory scope restrictions.

### Terminal tool constraints
- Model terminal interactions as:
  - `terminal.start_session`
  - `terminal.exec`
  - `terminal.end_session`
- Each command execution must return structured output:
  - stdout
  - stderr
  - exit code
  - timing metadata
- Enforce timeouts.
- Ensure cleanup when the session ends or errors.

### Screenshot tool constraints
- Provide a controlled, read-only capability.
- Return structured metadata with the capture when available.
- Avoid broad desktop control in V1.

---

## Coding standards

1. Keep functions small and composable.
2. Prefer interfaces at subsystem boundaries, not everywhere unnecessarily.
3. Use explicit types and descriptive names.
4. Do not hide risky behavior behind convenience wrappers.
5. Avoid global mutable state.
6. Return structured errors.
7. Use context propagation where appropriate.
8. Write code that is testable without a UI.
9. Keep OS-specific behavior isolated behind adapters.
10. Use comments to explain **why**, not to restate obvious code.

---

## Testing requirements

Every meaningful implementation change should include or update tests.

At minimum, prefer these test layers:
- unit tests for policy decisions, schema validation, and executor behavior
- subsystem tests for terminal/filesystem/screenshot adapters where possible
- integration tests for JSON-RPC request routing and request pipeline behavior
- persistence tests for event writes and SQLite projections

Add tests for:
- allow/deny logic
- approval-required behavior
- timeout enforcement
- session lifecycle behavior
- cleanup behavior on failure
- schema validation
- storage projection correctness

---

## Documentation requirements

Any change that affects behavior, architecture, security, protocol, or tool contracts must update the relevant docs in `docs/`.

At minimum:
- protocol changes -> update `docs/RPC_SCHEMA.md`
- security changes -> update `docs/SECURITY_MODEL.md`
- scope or feature changes -> update `docs/PRD.md`
- architectural boundary changes -> update `docs/ARCHITECTURE.md`
- completion/status expectations -> update `docs/ACCEPTANCE_CRITERIA.md`

Do not silently drift the code away from the docs.

---

## What to avoid

Do not introduce any of the following without explicit instruction:
- frontend code
- browser automation
- internet-exposed APIs
- remote SaaS auth/account systems
- cloud dependencies
- background daemon/service install as the default V1 behavior
- unrestricted shell passthrough
- hidden long-running tasks
- broad file mutation primitives
- over-engineered plugin systems
- unnecessary frameworks

Do not replace the architecture with a generic web server pattern.

---

## Implementation style when assigned a task

When asked to implement work in this repository:
1. Read the relevant docs first.
2. Restate the scope in concrete backend/runtime terms.
3. Make the smallest correct change set that satisfies the phase/task.
4. Preserve architectural boundaries.
5. Add or update tests.
6. Update docs if the behavior or contract changed.
7. Stop at the assigned phase boundary.

Do not “helpfully” expand scope.

---

## Phase discipline

Prefer bounded milestone completion over broad speculative generation.

When asked to implement a phase:
- complete only that phase
- list what was added
- list what remains
- list any assumptions made
- do not proceed into later phases unless explicitly asked

---

## Definition of success for this repository

A successful implementation is one that:
- respects the local-runtime architecture
- is secure by default
- keeps runtime core independent from UI
- supports the V1 tools cleanly
- persists audit/state locally
- is testable and documented
- is suitable to expose through future adapters without rewriting the runtime core
