# Writing a Go module

**Status:** living guide  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/module/module.go](../../internal/module/module.go), [modules/monitor/](../../modules/monitor/)

Go modules are compiled into the `pushapp` binary. They implement
`internal/module.Module` directly, in-process.

Architecture: [architecture/module-host.md](../architecture/module-host.md).

## The interface

```go
type Module interface {
    Meta() Meta
    Init(h Host) error
    Handle(ev Event)
    Draw(f *Frame)
    Close() error
}
```

Read the package doc on `internal/module` — it explains the three load-bearing
design choices (display list, serialised Handle/Draw, open op set).

## Rules

1. **Never draw pixels.** Append ops to `*module.Frame`; the host renders them.
   Use typed methods (`f.Rect`, `f.Text`, `f.List`, …). Reserve `AppendRaw` for
   tests and the process loader.
2. **No mutexes** — `Handle` and `Draw` never run concurrently.
3. **Never block in `Handle`.**
4. **ASCII only** in user-visible strings — non-ASCII renders as glyph boxes.
5. **Declare `NeedsMIDIOut: true`** if you call `SendCC` / `SendNote`. The host
   refuses activation without an output port.
6. **Release held notes in `Close`.** The host clears pad LEDs on exit but not
   MIDI notes in flight.

## Registration

Built-in modules live under `modules/` and register in the host's module list.
See how `monitor`, `thru`, `seq`, and `remap` are wired.

Run:

```bash
go run ./cmd/pushapp -list
go run ./cmd/pushapp -module monitor
```

## Persistence

```go
store := h.Store()
var cfg MyConfig
_ = store.Get(&cfg)  // defaults preserved if nothing stored yet
// ... mutate cfg ...
_ = store.Set(&cfg)
```

One JSON file per module ID. The host logs the path on activation (handy for
hand-editing — see `modules/remap`).

## Testing without hardware

```go
h := moduletest.NewHost()
m := &MyModule{}
_ = m.Init(h)
m.Handle(someEvent)
var f module.Frame
m.Draw(&f)
moduletest.NonASCIIStrings(&f)  // catch non-ASCII in Draw tests
```

`moduletest.Host` records every LED and MIDI write.

## Reference modules

| Module | Demonstrates |
|---|---|
| [monitor/](../../modules/monitor/) | Full UI, all event types, reference Draw |
| [thru/](../../modules/thru/) | MIDI out, held-note tracking |
| [seq/](../../modules/seq/) | Store, wall-clock timing, MIDI out |
| [remap/](../../modules/remap/) | Store + user-editable overrides |

## Not Go?

See [writing-a-process-module.md](writing-a-process-module.md) and the Python/JS
guides for out-of-process modules.

## Related

- [debugging.md](debugging.md)
- [protocol/led-output.md](../protocol/led-output.md) — palette indices
