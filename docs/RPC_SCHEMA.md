# RPC_SCHEMA.md

## Protocol summary

Nucleus Backend uses a **JSON-RPC 2.0 style** request/response model over **local IPC**.

This file defines the initial protocol direction for V1.

The goals of the RPC schema are:
- stable method naming
- explicit request/response envelopes
- structured errors
- discoverable tools
- clear separation between session methods, runtime methods, tool methods, and log/query methods

---

## Transport

Preferred transport:
- Named Pipes on Windows
- Unix Domain Sockets on macOS/Linux

This schema does not require public HTTP routes.

---

## Envelope format

### Request

```json
{
  "jsonrpc": "2.0",
  "id": "req_123",
  "method": "tools.call",
  "params": {
    "session_token": "...",
    "tool_name": "filesystem.read",
    "arguments": {
      "path": "C:/example/file.txt"
    }
  }
}
```

### Success response

```json
{
  "jsonrpc": "2.0",
  "id": "req_123",
  "result": {
    "ok": true,
    "data": {}
  }
}
```

### Error response

```json
{
  "jsonrpc": "2.0",
  "id": "req_123",
  "error": {
    "code": 40301,
    "message": "policy denied request",
    "data": {
      "reason": "path out of allowed scope"
    }
  }
}
```

---

## Method namespaces

Recommended namespaces:
- `runtime.*`
- `session.*`
- `tools.*`
- `terminal.*`
- `screenshot.*`
- `desktop.*`
- `logs.*`
- `approvals.*`

Do not create random one-off method names.

---

## Initial V1 methods

### `runtime.health`
Returns runtime health metadata.

#### Request
```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "runtime.health",
  "params": {}
}
```

#### Result
```json
{
  "ok": true,
  "data": {
    "service": "nucleusd",
    "status": "ok",
    "version": "0.1.0"
  }
}
```

---

### `session.bootstrap`
Bootstraps a trusted session between the client and runtime.

#### Purpose
- validate the caller bootstrap context
- create or register a session
- return a session token and metadata

#### Request shape
```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "method": "session.bootstrap",
  "params": {
    "client_name": "nucleus-electron",
    "client_version": "0.1.0",
    "bootstrap_token": "short_lived_bootstrap_secret"
  }
}
```

#### Result shape
```json
{
  "ok": true,
  "data": {
    "session_id": "sess_123",
    "session_token": "opaque_runtime_token",
    "expires_at": "2026-03-12T23:59:59Z",
    "trust_level": "trusted_local_client"
  }
}
```

---

### `session.status`
Returns current session metadata.

#### Request params
- `session_token`

#### Result fields
- `session_id`
- `started_at`
- `expires_at`
- `trust_level`
- `approved_scopes`
- `capabilities`

---

### `tools.list`
Returns discoverable tools with schemas and metadata.

#### Request params
- `session_token`

#### Result example
```json
{
  "ok": true,
  "data": {
    "tools": [
      {
        "name": "filesystem.list",
        "risk": "medium",
        "requires_approval": true,
        "input_schema": {
          "type": "object"
        },
        "output_schema": {
          "type": "object"
        }
      },
      {
        "name": "terminal.exec",
        "risk": "high",
        "requires_approval": true,
        "input_schema": {
          "type": "object"
        },
        "output_schema": {
          "type": "object"
        }
      }
    ]
  }
}
```

---

### `tools.call`
Generic tool invocation for tools that do not need specialized lifecycle handling.

#### Request shape
```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "method": "tools.call",
  "params": {
    "session_token": "opaque_runtime_token",
    "tool_name": "filesystem.read",
    "arguments": {
      "path": "C:/allowed/file.txt"
    }
  }
}
```

#### Result envelope
```json
{
  "ok": true,
  "data": {
    "tool_name": "filesystem.read",
    "execution_id": "exec_123",
    "result": {
      "path": "C:/allowed/file.txt",
      "content": "...",
      "encoding": "utf-8"
    }
  }
}
```

---

## Terminal-specific methods

Terminal is modeled with explicit lifecycle methods.

### `terminal.start_session`
Starts a managed terminal session.

#### Request params
- `session_token`
- `working_directory` (optional, policy-scoped)
- `shell_profile` (optional, implementation-defined)

#### Result example
```json
{
  "ok": true,
  "data": {
    "terminal_session_id": "term_123",
    "started_at": "2026-03-12T22:00:00Z",
    "working_directory": "C:/project"
  }
}
```

---

### `terminal.exec`
Executes a single command within a managed terminal session.

#### Request shape
```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "terminal.exec",
  "params": {
    "session_token": "opaque_runtime_token",
    "terminal_session_id": "term_123",
    "command": "go test ./...",
    "timeout_ms": 30000
  }
}
```

#### Result example
```json
{
  "ok": true,
  "data": {
    "execution_id": "exec_456",
    "terminal_session_id": "term_123",
    "command": "go test ./...",
    "stdout": "ok   ./...",
    "stderr": "",
    "exit_code": 0,
    "duration_ms": 1840
  }
}
```

Notes:
- Each terminal command is modeled as one controlled execution.
- Chained shell composition is discouraged by default unless explicitly supported by policy.
- Timeout is mandatory or defaulted by runtime policy.

---

### `terminal.end_session`
Ends a managed terminal session.

#### Request params
- `session_token`
- `terminal_session_id`

#### Result example
```json
{
  "ok": true,
  "data": {
    "terminal_session_id": "term_123",
    "status": "ended"
  }
}
```

---

---

## Screenshot / desktop-state methods

### `screenshot.capture`
Captures a screenshot in supported environments.

#### Request params
- `session_token`
- `display_id` (optional)

#### Result example
```json
{
  "ok": true,
  "data": {
    "execution_id": "exec_789",
    "tool_name": "screenshot.capture",
    "result": {
      "capture_id": "cap_123",
      "mime_type": "image/png",
      "width": 1920,
      "height": 1080,
      "metadata": {
        "display_id": "primary"
      }
    }
  }
}
```

---
### `desktop.get_state`
Returns lightweight desktop/window/display metadata in supported environments.

#### Request params
- `session_token`

#### Result example
```json
{
  "ok": true,
  "data": {
    "execution_id": "exec_790",
    "tool_name": "desktop.get_state",
    "result": {
      "active_window": {
        "title": "Visual Studio Code",
        "app_name": "Code"
      },
      "displays": [
        {
          "display_id": "primary",
          "width": 1920,
          "height": 1080
        }
      ]
    }
  }
}
```

## Approvals

### `approvals.respond`
Allows a trusted client to grant or deny an approval request.

#### Request params
- `session_token`
- `approval_id`
- `decision` (`grant` or `deny`)
- `reason` (optional)

#### Result
- approval resolution metadata

---

## Logs/query methods

### `logs.query`
Returns persisted event/state summaries for authorized clients.

#### Request params
- `session_token`
- `kind` (`events`, `executions`, `errors`, `terminal_sessions`, etc.)
- filter options
- limit/offset options

This method should not expose unrestricted raw storage internals.

---

## Tool result shape guidance

Tool results should generally include:
- `execution_id`
- `tool_name`
- structured `result`
- optional metadata

Avoid returning unstructured blobs unless the tool genuinely requires them.

---

## Error model

Errors should be structured and stable.

### Suggested categories
- parse/protocol errors
- auth/session errors
- policy/permission errors
- validation errors
- execution errors
- timeout errors
- internal runtime errors
- storage errors

### Suggested numeric ranges
- `-32700` to `-32600` for JSON-RPC-level compatibility if used
- `401xx` for auth/session failures
- `403xx` for policy/permission denials
- `404xx` for unknown tools/sessions/resources
- `408xx` for timeouts
- `422xx` for validation/schema errors
- `500xx` for runtime/internal failures
- `507xx` for storage/persistence failures

Example:
```json
{
  "code": 40801,
  "message": "terminal command timed out",
  "data": {
    "timeout_ms": 30000,
    "terminal_session_id": "term_123"
  }
}
```

---


---

## Event schema guidance

Events should be immutable and append-only.

Suggested shared event fields:
- `event_id`
- `event_type`
- `occurred_at`
- `session_id`
- `execution_id` (optional)
- `terminal_session_id` (optional)
- `tool_name` (optional)
- `payload`

Example event:
```json
{
  "event_id": "evt_123",
  "event_type": "terminal.command.completed",
  "occurred_at": "2026-03-12T22:01:00Z",
  "session_id": "sess_123",
  "execution_id": "exec_456",
  "terminal_session_id": "term_123",
  "tool_name": "terminal.exec",
  "payload": {
    "command": "go test ./...",
    "exit_code": 0,
    "duration_ms": 1840
  }
}
```

---

## Schema evolution rules

1. Prefer additive changes over breaking changes.
2. Do not silently rename method namespaces.
3. If result payloads change materially, update docs and tests together.
4. Keep tool schemas discoverable through `tools.list`.
5. Version runtime behavior explicitly in metadata when needed.
