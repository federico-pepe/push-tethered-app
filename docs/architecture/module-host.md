# Module host architecture

**Status:** implemented  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/host/](../../internal/host/), [internal/module/](../../internal/module/)

`pushapp` is a **module host**. It owns the Push hardware and runs one
**module** at a time. No DAW is involved at any layer. A MIDI remapper is
a module, not the product.

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
2. **`Handle` and `Draw` never run concurrently.** One module goroutine
   handles both, so module state needs no mutexes.
3. **Never block in `Handle`.** The host drops the oldest events if the
   module stalls.
4. **The op set is open.** An unknown op is logged and skipped, not
   fatal. `Host.SupportedOps()` lists the ops this host knows.
5. **`NeedsMIDIOut`** in `Meta`: the host refuses activation if it cannot
   open an output port.
6. **`Store()`** persists JSON, keyed by module ID. When a display is
   claimed, it is also keyed by device (`display.Info.ID`). As a result,
   two `pushapp-ui` sessions that run the same module against different
   Push units never share one file
   ([internal/host/store.go](../../internal/host/store.go)).

## Host API (`module.Host`)

- `Device()` — Push 2 vs Push 3 (`pushmap.Device`)
- `SetPad(note, colour byte)` / `SetButton(cc, brightness byte)`
- `SendCC` / `SendNote` / `NoteOff` — to the owned MIDI out port
- `Log`, `Store`, `SupportedOps`, `Theme()`

The port opens **on activation**, never earlier. Release held notes in
`Close`. The host clears LEDs, but it does not release in-flight notes
([modules/thru/](../../modules/thru/)).

## Concurrency model

```
RtMidi thread  →  decode  →  event channel  →  module goroutine
                                                    ↓
                                              Handle / Draw
frame ticker   →  Draw request  ──────────────────────┘
                    ↓
              render ops → USB display @ ~fps
```

LED writes come from the module goroutine. The driver thread never blocks
on module logic.

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
whose name mentions Push. This loops the output back into the decoder.

### Routing through Push 3's External Port instead

`NeedsMIDIIn`/`NeedsMIDIOut` modules normally reach `internal/midiin`/
`internal/midiout`'s own virtual loopback ports (above). `bootstrap.Options`
also has `ExtMIDIInFromPushExternal` and `ExtMIDIOutToPushExternal`
(`internal/bootstrap/bootstrap.go`): set either one, and the same
`OpenMIDIIn`/`OpenMIDIOut` openers instead point at the connected unit's own
**External Port** cable — Push 3's physical MIDI DIN jacks — found by
`findExternalRef` matching `PortRef.Unit` against the already-opened
control-surface port. This is a real, separate MIDI cable, not the
undocumented xPort USB interface (`docs/protocol/xport.md`) — see
`docs/protocol/midi-input.md`'s Ports table.

`midiin.OpenExisting`/`midiout.OpenExisting` attach to that specific cable
by exact name and driver number, deliberately skipping the `isPush` filter
`Open`'s virtual-port path uses — that filter exists to avoid looping our
own output back into Push's control-surface input, which does not apply
here since External Port carries neither. Push 2 has no External Port; a
caller that sets either flag without one connected gets a logged warning
and the virtual loopback port instead, not a failure. `cmd/pushapp`'s
`-ext-port-in`/`-ext-port-out` flags and `pushapp-ui`'s pairing-form
checkboxes (Push-3-only, greyed out otherwise) both set these at connect
time; there is no way to change them on an already-connected session.

## UI

[cmd/pushapp-ui/](../../cmd/pushapp-ui/) — Wails v3 switcher: list, activate,
install/uninstall process modules. Same bootstrap path as CLI `pushapp`.

## Process-loaded modules

Out-of-process modules implement the same contract over JSON. See
[process-modules.md](process-modules.md).

## Related

- [guides/writing-a-go-module.md](../guides/writing-a-go-module.md)
- [stack-and-layout.md](stack-and-layout.md)
