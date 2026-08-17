# Push Tethered App

Cross-platform desktop app that owns an **Ableton Push 2 / Push 3 in tethered
(controller) mode** — display, pads, buttons, encoders, LEDs — and turns it
into a platform you can write your own tools for, independent of any DAW.

> **Status: pre-alpha, but running.** `cmd/pushapp` is a module host: one Go
> binary that holds the screen at 30fps, reads the control surface, and runs
> whichever module is active. Confirmed on both Push 2 and Push 3 hardware
> from the same unmodified binary. Modules can be Go compiled into the binary,
> or **any executable in any language** — see
> [plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md) and
> `examples/modules/` for working Python and Node.js modules, both confirmed
> end-to-end on real hardware. Full design in
> [plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md); protocol
> facts and MIDI/LED behaviour live in [CLAUDE.md](CLAUDE.md); what's still
> open is in [docs/open-questions.md](docs/open-questions.md).

```bash
go run ./cmd/pushapp                          # host + first module
go run ./cmd/pushapp -list                    # what modules are available
go run ./cmd/pushapp -capture demo.mp4        # ...and record the screen
go run ./cmd/midiouttest                      # prove MIDI reaches other apps
```

A minimal desktop UI for switching modules lives in
[cmd/pushapp-ui](cmd/pushapp-ui) (Wails v3 — its own Go module, see
CLAUDE.md's "Cross-platform builds" for why):

```bash
cd cmd/pushapp-ui && wails3 dev     # hot-reload window
```

## Writing a module

```go
type Module interface {
    Meta() Meta            // id, name, whether it sends MIDI
    Init(h Host) error     // called on activation; h is the hardware
    Handle(ev Event)       // pads, buttons, encoders, touch, MPE
    Draw(f *Frame)         // append draw ops for one frame
    Close() error
}
```

A module never touches USB, never opens a MIDI port and never draws pixels —
it appends ops to a `Frame` and the host renders them with the shared
`core/gfx` widget toolkit. `Handle` and `Draw` are guaranteed never to run
concurrently, so module state needs no locks. `modules/monitor` is the
reference, and `internal/module/moduletest` provides a fake host so modules
can be unit-tested with no Push attached.

Not writing Go? A module can be any executable, speaking a small JSON protocol
over stdin/stdout — `examples/modules/hello-py` and `examples/modules/hello-js`
are working references:

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```

## Why this can work

The hard part — driving Push's 960×160 screen — is already solved in the
sibling project [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack).
Its `core/` Go module is a complete, device- and transport-agnostic Push
screen toolkit written against plain `image.NRGBA`; this project reuses it
directly and swaps the transport (shared-memory writer → USB bulk writer).
Push 3's display protocol turned out to be **byte-identical to Push 2's
public spec** (`Ableton/push-interface`).

See [CLAUDE.md](CLAUDE.md) for the full set of measured protocol/MIDI/LED
facts, or [docs/archive/feasibility.md](docs/archive/feasibility.md) for the
original measurements behind them.

## Push 2 vs Push 3

Both run from the same binary — display, pad grid and LED palette are
identical. Only these differ:

| | Push 2 | Push 3 |
|---|---|---|
| USB interfaces | 3 | 7 |
| MIDI ports | 2 (Live, User) | 3 (+External) |
| Audio | none | class-compliant 16×16 |
| `xPort` | absent | present |
| MPE | no — pads on ch1 | usually |
| Button CCs | 5 differ (see CLAUDE.md) | — |

## What this is — a module host

Decided 2026-08-17: `pushapp` is a **host** that owns the hardware and runs
**modules** — small programs, writable by anyone, that draw Push's screen and
handle its pads, encoders and buttons. No DAW is involved at any layer; a MIDI
remapper is *a module*, not the product. Full design in
[plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md).

### Reaching other software

Modules can send MIDI out — to a synth, a DAW, anything. The app doesn't
create a virtual port; it **owns a named output port**:

| | How | Setup for the user |
|---|---|---|
| macOS, Linux | creates the port itself | none |
| Windows | attaches to an existing port | install [loopMIDI](https://www.tobias-erichsen.de/software/loopmidi.html) and create one |

Windows MM MIDI can't create virtual ports at all — a platform fact, not a
missing feature.

## Requirements

**Users:** none beyond the OS. Single binary.

**Development:**
- Go 1.25+
- libusb 1.0 (`brew install libusb` on macOS)
- A sibling checkout of `ableton-push-hack` for the `core/` module — see the
  `replace` directive in `go.mod`.

## Layout

```
cmd/pushapp/      the app: display + input + LEDs in one process
cmd/pushapp-ui/   Wails v3 module switcher (separate Go module)
cmd/probe/        USB descriptor dump — read-only, never opens the device
cmd/frametest/    display-only probe: one frame, or a timed hold
cmd/mapcheck/     cross-references captures against the button map
cmd/midiouttest/  MIDI-out probe: create/attach a port, send, and receive back
internal/module/  the module ABI: Module, Host, Frame/Op, Event
internal/host/    runtime: registry, control API, event fan-out, frame loop
internal/host/procmod/  process-loaded modules: JSON-over-stdio protocol
internal/display/ USB transport: claim interface 0, header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/midiout/ owns a named MIDI out port for modules (create or attach)
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
modules/          monitor (reference), thru, seq, remap
examples/modules/ process-loaded example modules (Python, Node.js)
tools/            macOS-only Swift probes (midimon, ledtest)
docs/             archive/feasibility.md (frozen writeup) + open-questions.md
```

## Stack

- **Go**, single binary — so the `core/` screen toolkit is reused, not ported.
- **`gousb`** (cgo → libusb) for the display interface.
- **`gomidi` + `rtmididrv`** for OS MIDI — the driver vendors RtMidi's C++
  sources, so there's no system package to install on any of the three OSes.
- cgo means **no cross-compilation**: build on each target OS.
  `.github/workflows/build.yml` builds on real macOS/Linux/Windows runners —
  see CLAUDE.md's "Cross-platform builds" for local per-OS setup.

## Related

- [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack) —
  standalone Push 3 hacks; source of the `core/` module reused here.
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) —
  official Push 2 display + MIDI specification.
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working
  pyusb reference implementation for Push 2.
