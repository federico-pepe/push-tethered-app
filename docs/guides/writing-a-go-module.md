# Writing a Go module

**Status:** living guide
**Last verified:** 2026-08-20
**Authoritative code:** [internal/module/module.go](../../internal/module/module.go), [modules/monitor/](../../modules/monitor/)

The `pushapp` binary compiles in Go modules. A Go module implements
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

Read the package doc on `internal/module`. It explains the three
load-bearing design choices: the display list, the serialized Handle/Draw
sequence, and the open op set.

## Rules

1. Do not draw pixels directly. Append ops to `*module.Frame` instead. The
   host renders these ops. Use typed methods (`f.Rect`, `f.Text`, `f.List`,
   and more). Reserve `AppendRaw` for tests and the process loader.
2. Do not use mutexes. `Handle` and `Draw` never run at the same time.
3. Do not block inside `Handle`.
4. Use ASCII only in user-visible strings. Non-ASCII text renders as glyph
   boxes.
5. If you call `SendCC` or `SendNote`, declare `NeedsMIDIOut: true`. Without
   this, the host refuses to activate the module if no output port is
   available.
6. If you want `ExternalMIDI` events, declare `NeedsMIDIIn: true`.
   `ExternalMIDI` carries raw MIDI from other software or hardware, not from
   Push itself — for example, a clock to sync to, or a controller. Unlike
   `NeedsMIDIOut`, a missing input port is never fatal. The module still
   activates, but it never receives `ExternalMIDI`. Decode `ev.Raw`
   yourself. The host does not interpret it. See
   [internal/midiin](../../internal/midiin/midiin.go).
7. Release held notes in `Close`. The host clears pad LEDs on exit, but it
   does not clear MIDI notes still in flight.

## Registration

Built-in modules live under `modules/`. They register in the host's module
list. See how `monitor`, `thru`, `seq`, and `remap` are wired.

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

The host uses one JSON file per module ID. It logs the path on activation.
This is useful when you edit the file by hand — see `modules/remap`.

### Module-internal UI modes

A module can have more than one on-screen mode, for example a normal view
plus an editor. To do this, keep a small state enum on the module and
switch both `Handle` and `Draw` on it. `modules/remap` is the first, and so
far the only, example of this pattern. A bottom-screen button arms an edit
mode. The next pad, button, or encoder event is then captured as a target
instead of being passed through. A handful of top-row encoders become field
editors until the user selects Save, Clear, or Cancel.

There is no shared framework for this pattern yet. One module is not enough
to justify extracting one. Read `modules/remap/remap.go`'s `uiState`
handling directly if you need the same shape.

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

`moduletest.Host` records every LED write and MIDI write.

## Reference modules

| Module | Demonstrates |
|---|---|
| [monitor/](../../modules/monitor/) | Full UI, all event types, reference Draw |
| [thru/](../../modules/thru/) | MIDI out, held-note tracking |
| [seq/](../../modules/seq/) | Store, wall-clock timing, MIDI out |
| [remap/](../../modules/remap/) | Store + on-device rule editor, module-internal UI modes |

## Not Go?

See [writing-a-process-module.md](writing-a-process-module.md) and the
Python and JavaScript guides for modules that run out of process.

## Related

- [debugging.md](debugging.md)
- [protocol/led-output.md](../protocol/led-output.md) — palette indices
