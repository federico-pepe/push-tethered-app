# Writing a process module (overview)

**Status:** living guide  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/host/procmod/](../../internal/host/procmod/)

A process module is any executable the host spawns and talks to over
**newline-delimited JSON** on stdin/stdout. Same behaviour contract as a Go
module, different transport.

Full protocol: [architecture/process-modules.md](../architecture/process-modules.md).

## Quick start

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```

## What you need

1. A directory with `manifest.json` and your script/executable
2. A main loop reading JSON lines from stdin
3. Handlers for `init`, `handle`, `draw`, `close`
4. Responses on stdout — **one JSON object per line, flushed immediately**

## Lifecycle

```
host spawns child
  → init (device, theme, supported_ops) → {}
  → handle notifications (pad press, encoder, …) — no reply
  → draw request → {ops: [...], failed: 0}   [every frame]
  → close → {} then exit
```

## Draw ops

Build an array of ops mirroring Go types:

```json
{"kind": "rect", "params": {"x": 0, "y": 0, "w": 960, "h": 160, "c": {"R":0,"G":0,"B":0,"A":255}}}
{"kind": "text", "params": {"x": 8, "baseline": 80, "s": "hello", "c": {"R":255,"G":255,"B":255,"A":255}}}
```

Check `supported_ops` from `init` before using ops the host might not know.

## Host calls from child

Notifications (no `id`):

```json
{"method": "set_pad", "params": {"note": 36, "colour": 11}}
```

Requests (with `id`, expect response):

```json
{"id": 1, "method": "store_get", "params": {}}
```

MIDI out (`send_cc`, `send_note`, `note_off`) requires
`"needs_midi_out": true` in manifest.

## Common mistakes

| Mistake | Symptom |
|---|---|
| Buffered stdout (Python) | Host hangs on first `draw` |
| Wrong colour type for pads | Use palette **index** for `set_pad`, RGBA for screen ops |
| Blocking on `handle` | Events pile up; host drops oldest |
| Using Image op | Not available over IPC |

## Language-specific guides

- [writing-a-python-module.md](writing-a-python-module.md)
- [writing-a-javascript-module.md](writing-a-javascript-module.md)

Examples index: [examples/modules/README.md](../../examples/modules/README.md).
