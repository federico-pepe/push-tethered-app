# Stack and repository layout

**Status:** living reference  
**Last verified:** 2026-08-18  
**Authoritative code:** [go.mod](../../go.mod), [CLAUDE.md](../../CLAUDE.md) (agent index)

Rationale trail: [archive/feasibility.md](../archive/feasibility.md) §6.

## Stack decisions

| Choice | Why |
|---|---|
| **Go**, single binary | Reuse `core/` screen toolkit from ableton-push-hack |
| **`gousb`** (cgo → libusb) | USB display transport. Cost: no cross-compilation, LGPL-2.1 |
| **`gomidi` + `rtmididrv`** | OS MIDI on all three OSes; vendors RtMidi C++ — no system MIDI packages |
| **OS MIDI for input** | Never libusb for Push MIDI; co-existence with OS drivers |
| **`internal/midiout`** | Named output port — create (macOS/Linux) or attach (Windows) |
| **Wails v3** | Desktop UI (`cmd/pushapp-ui`). Linux needs webkit2gtk |

Rust + `nusb` was considered and rejected — forfeits `core/` reuse.

**No cross-compiling.** Build natively on each target OS. CI uses real
macOS/Linux/Windows runners ([.github/workflows/build.yml](../../.github/workflows/build.yml)).

## `core/` sibling dependency

[`ableton-push-hack/core`](https://github.com/federico-pepe/ableton-push-hack/tree/main/core)
is linked via `replace` in `go.mod`. Reused packages:

- `core/gfx`, `core/gfx/text`, `core/gfx/widgets` — drawing
- `core/display` — `ToBGR565` / `FromBGR565`
- `core/push3` — geometry, palette, encoder decode

**Never fork or vendor `core/`** — fix upstream so both projects benefit.

Fresh clone: place sibling repo at the path in `go.mod`'s `replace`, or edit
the path. CI checks out `ableton-push-hack@main` and runs `go mod edit -replace`.

## Repository layout

```
cmd/pushapp/           Host CLI — owns hardware, runs one module
cmd/pushapp-ui/        Wails v3 module switcher (separate Go module)
cmd/probe/             USB descriptor dump (read-only)
cmd/frametest/         Display-only probe
cmd/mapcheck/          Cross-reference captures against button map
cmd/midiouttest/       MIDI-out create/attach probe
internal/bootstrap/    Shared hardware-open sequence
internal/module/       Module ABI: Module, Host, Frame, Event, Store
internal/module/moduletest/  Fake Host for unit tests
internal/host/         Runtime: registry, frame loop, render, Store
internal/host/procmod/ Process-loaded modules (JSON stdio)
internal/display/      USB display transport
internal/midi/         OS MIDI in, event decode, LED helpers
internal/midiout/      Named MIDI out port for modules
internal/mirror/       Live HTTP/MJPEG screen mirror (taps the render output,
                        same as internal/capture, but streams to browsers)
internal/pushmap/      Push 2 map deltas + shared name tables
modules/               Built-in Go modules (monitor, thru, seq, remap)
examples/modules/      Process module examples (Python, Node.js)
tools/                 macOS Swift probes (midimon, ledtest)
docs/                  Reference documentation
plans/                 Decision history and intent
```

## `cmd/pushapp-ui` is a separate Go module

Do not add it to root `go build ./...`. It has its own `go.mod` with two
`replace` directives (root module + `core/`). Building needs `wails3` CLI and
Node/npm. See [cmd/pushapp-ui/README.md](../../cmd/pushapp-ui/README.md).

## Channel convention

APIs use channels **1–16**; wire format uses 0–15 inside `midiout`.
`gomidi`'s `Message.String()` prints 0-based channels — that is correct, not
a bug.

## Operating model

- **Full ownership** = we are the only host, not claiming USB interface 5
- **Co-existence with Live** is not a shipping mode — `ErrBusy` if Live holds
  the display
- A remapper is a module, not the product

## Related

- [module-host.md](module-host.md)
- [guides/development-setup.md](../guides/development-setup.md)
- [protocol/display.md](../protocol/display.md)
