# Module host architecture

**Status:** implemented  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/host/](../../internal/host/), [internal/module/](../../internal/module/)

`pushapp` is a **module host**: it owns Push hardware and runs one **module**
at a time. No DAW is involved at any layer; a MIDI remapper is a module, not
the product.

Decision history: [plans/2026-08-17-module-host.md](../../plans/2026-08-17-module-host.md).

## Responsibilities

| Layer | Owns |
|---|---|
| Host (`internal/host`) | USB display, OS MIDI in/out, LED clearing, frame loop, op rendering, module registry, per-module Store |
| Module (`internal/module` or child process) | Screen content (draw ops), input handling, optional MIDI out to other software |

Modules **never** touch USB, open MIDI ports, or draw pixels directly.

## Module contract (Go)

```go
type Module interface {
    Meta() Meta
    Init(h Host) error
    Handle(ev Event)
    Draw(f *Frame)
    Close() error
}
```

Reference implementation: [modules/monitor/](../../modules/monitor/).

### Key rules

1. **Draw builds a display list**, not an image. The host renders ops via
   `core/gfx` + `core/gfx/widgets`.
2. **`Handle` and `Draw` never run concurrently** — one module goroutine; no
   mutexes needed in module state.
3. **Never block in `Handle`.** The host drops oldest events if the module
   stalls.
4. **Op set is open** — unknown ops are logged and skipped, never fatal.
   `Host.SupportedOps()` lists what this host knows.
5. **`NeedsMIDIOut`** in `Meta` — host refuses activation if no output port
   can be opened.
6. **`Store()`** — per-module persisted JSON ([internal/host/store.go](../../internal/host/store.go)).

## Host API (`module.Host`)

- `Device()` — Push 2 vs Push 3 (`pushmap.Device`)
- `SetPad(note, colour byte)` / `SetButton(cc, brightness byte)`
- `SendCC` / `SendNote` / `NoteOff` — to the owned MIDI out port
- `Log`, `Store`, `SupportedOps`, `Theme()`

Port opens **on activation**, never earlier. Release held notes in `Close` —
the host clears LEDs but not in-flight notes ([modules/thru/](../../modules/thru/)).

## Concurrency model

```
RtMidi thread  →  decode  →  event channel  →  module goroutine
                                                    ↓
                                              Handle / Draw
frame ticker   →  Draw request  ──────────────────────┘
                    ↓
              render ops → USB display @ ~fps
```

LED writes from the module goroutine; the driver thread never blocks on module
logic.

## Built-in modules

| ID | Purpose |
|---|---|
| `monitor` | Control-surface mirror (reference) |
| `thru` | Forward controls as MIDI out |
| `seq` | 8-step pad-grid sequencer |
| `remap` | User-editable overrides on `thru` |

## MIDI out to other software

The host **owns a named output port**:

- **macOS / Linux:** creates a virtual port (`OpenVirtualOut`)
- **Windows:** attaches to an existing port (user provides via loopMIDI)

See [platform/windows.md](../platform/windows.md). Never attach to a port
whose name mentions Push — loops output back into the decoder.

## UI

[cmd/pushapp-ui/](../../cmd/pushapp-ui/) — Wails v3 switcher: list, activate,
install/uninstall process modules. Same bootstrap path as CLI `pushapp`.

## Process-loaded modules

Out-of-process modules implement the same contract over JSON — see
[process-modules.md](process-modules.md).

## Related

- [guides/writing-a-go-module.md](../guides/writing-a-go-module.md)
- [stack-and-layout.md](stack-and-layout.md)
