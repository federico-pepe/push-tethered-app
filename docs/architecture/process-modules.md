# Process-loaded modules

**Status:** implemented  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/host/procmod/](../../internal/host/procmod/), [internal/host/procinstall.go](../../internal/host/procinstall.go)

A module can be **any executable** — Python, Node.js, Rust, and more — that
speaks JSON-over-stdio. The host spawns it as a child process. `procmod.Proc`
implements `module.Module`, so the runtime treats it like an in-tree module.

Decision history: [plans/2026-08-17-process-loader.md](../../plans/2026-08-17-process-loader.md).

## Directory layout

```
~/.config/push-tethered-app/modules/<id>/
  manifest.json
  <executable and assets>
```

Installed via:

```bash
go run ./cmd/pushapp -install path/to/your-module
go run ./cmd/pushapp -uninstall your-module-id
go run ./cmd/pushapp -list    # [installed] marker
go run ./cmd/pushapp -module your-module-id
```

## manifest.json

```json
{
  "id": "hello-py",
  "name": "Hello (Python example)",
  "version": "1.0.0",
  "author": "someone",
  "needs_midi_out": false,
  "exec": "python3 run.py"
}
```

`exec` is resolved relative to the module directory. On Windows, name the
full command the shell needs, for example `python.exe run.py`.

## Wire protocol

One JSON object per line on stdin (host→child) and stdout (child→host).
Messages must not contain literal newlines.

```json
{"id": 1, "method": "draw", "params": {}}
{"id": 1, "result": {"ops": [...], "failed": 0}}
```

| Field | Role |
|---|---|
| `id` | Request/response correlation; omit for notifications |
| `method` | Present on requests |
| `params` / `result` / `error` | Payload |

Not full JSON-RPC — no batching, no version field. Two peers, fixed method set.

### Host → child

| Method | Params | Response | Notes |
|---|---|---|---|
| `init` | `{device, theme, supported_ops}` | `{}` or error | Once, before anything else |
| `handle` | `{event}` | *(notification)* | Never blocks host |
| `draw` | `{}` | `{ops, failed}` | Must round-trip each frame |
| `close` | `{}` | `{}` or error | Child releases notes, then exits |

### Child → host

| Method | Params | Response |
|---|---|---|
| `set_pad` | `{note, colour}` | notification |
| `set_button` | `{cc, brightness}` | notification |
| `send_cc` | `{ch, cc, val}` | `{}` or error |
| `send_note` | `{ch, note, vel}` | `{}` or error |
| `note_off` | `{ch, note}` | `{}` or error |
| `log` | `{message}` | notification |
| `store_get` | `{}` | `{doc}` |
| `store_set` | `{doc}` | `{}` or error |

`send_*` requires `"needs_midi_out": true` in manifest.

## Draw ops

These ops use the same JSON shapes as the Go `internal/module` types.
Colour: `{"R":255,"G":255,"B":255,"A":255}` (Go `color.NRGBA` encoding).

**The image op is not available** over IPC. Raw pixels need an in-tree Go
module.

## Supervisor lifecycle

1. **Init:** spawn process, wire stdin/stdout, stderr → host log, send `init`
2. **Handle:** write notification, never wait
3. **Draw:** request/response with bounded timeout. On timeout: empty frame, logged
4. **Close:** send `close`, grace period, then kill if needed
5. **Crash:** reader detects closed pipe. Further calls no-op until re-activation

## Critical: flush stdout

The host reads **one line at a time**. Python buffers stdout when it is not
a TTY. **Flush after every line**, or the host appears hung. Node on POSIX
writes synchronously to pipes, but the same discipline applies everywhere.

## Language guides

- [writing-a-python-module.md](../guides/writing-a-python-module.md)
- [writing-a-javascript-module.md](../guides/writing-a-javascript-module.md)
- [writing-a-process-module.md](../guides/writing-a-process-module.md) — overview

Examples: [examples/modules/](../../examples/modules/).
