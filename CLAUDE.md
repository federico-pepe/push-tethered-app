# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository. This is
a slim agent manual — durable reference lives in [docs/](docs/), read it
first for anything beyond the safety rules and pointers below.

> **Doc sync rule:** update this file, `README.md`, and `docs/` when a change
> is *meaningful* to a future reader — new behaviour, a changed protocol fact,
> a new API, a resolved or newly-found issue. Not every commit needs a doc
> update; skip internal refactors and trivial edits. When you do update, keep
> it in the same commit as the change. New protocol fact → `docs/protocol/`;
> resolved open question → fold into docs and delete from
> [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md).
>
> **Plans live in `plans/`**, one file per plan, named
> `YYYY-MM-DD-name-of-the-plan.md`. `docs/` holds durable reference (protocol
> facts, measurements, rationale); `plans/` holds intent (what we're about to
> do and why, including open decisions). When a plan is done, distill its
> settled contract into `docs/architecture/`; leave the plan itself as
> historical reasoning.
>
> **`docs/archive/` is frozen** — never edit, move, or delete anything inside
> it, never add to it unless explicitly asked. It holds superseded docs kept
> for history.

## Project

`push-tethered-app` — cross-platform desktop app that owns an **Ableton Push 2 /
Push 3 in tethered (controller) mode**: display, pads, buttons, encoders, LEDs.
**It is a module host** — `pushapp` owns the hardware and runs **modules**,
small programs anyone can write in Go or any other language, that draw the
screen and handle the controls. No DAW is involved at any layer.

**Status: pre-alpha, running, confirmed on Push 2 and Push 3 hardware.** Full
picture (built-in modules, process-loaded modules, `pushapp-ui`): root
[README.md](README.md) and [docs/README.md](docs/README.md).

Decision history: [plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md)
(module-host shape), [plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md)
(any-language modules). [plans/2026-08-16-product-shape-decision.md](plans/2026-08-16-product-shape-decision.md)
is **closed** — reasoning trail only, don't plan against it.

## Read this first

- [docs/README.md](docs/README.md) — reading paths by task (write a module,
  build/contribute, protocol facts)
- [docs/protocol/usb-and-safety.md](docs/protocol/usb-and-safety.md) — before
  any hardware interaction
- [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md) — what's still unresolved
- [docs/archive/feasibility.md](docs/archive/feasibility.md) — frozen, the
  original protocol evidence and stack rationale (don't edit)

## Relationship to `ableton-push-hack`

Sibling of `~/Documents/GitHub/ableton-push-hack` (Push 3 *standalone*,
deployed over SSH). `core/` is reused, not copied, via a `replace` in
`go.mod` — **never fork or vendor it**, fix upstream so both projects
benefit. That repo's hard safety rules (no `/boot`, `/opt`, `/etc`) don't
apply here — but see USB safety below. Full detail:
[docs/hardware-reference.md](docs/hardware-reference.md).

## Non-negotiable safety rules

These stay here even though they're also in `docs/` — an agent needs them
in-context before touching hardware, not one click away.

- **Claim only interface 0 (display).** Claiming MIDI/audio interfaces takes
  them away from the OS and the DAW.
- **Never write to `xPort` (interface 6) speculatively** — vendor-specific,
  undocumented.
- **No firmware operations. Ever.** No DFU, no control transfers with
  unknown vendor requests.
- **Never do a blind "press every button" sweep.** Run `cmd/pushapp` first —
  once a host drives the screen, the top-row buttons become plain MIDI and
  are safe to press. The leftmost button above the screen switches Push 3
  into standalone mode if the display isn't already held.
- **Always clear LEDs on every exit path, including SIGINT** — leaving the
  device lit makes the next run ambiguous.
- **Never call `dev.SetAutoDetach(true)`** — it's config-wide, not
  interface-wide, and tears audio/MIDI away from the OS class drivers.
- **ASCII only** when drawing — `core/gfx/text` renders any non-ASCII
  character as a missing-glyph box.

Full protocol detail (display format, MIDI decode order, MPE, LED messages,
button-sweep recovery): [docs/protocol/](docs/protocol/).

## Layout

```
cmd/pushapp/      the host: owns the hardware, runs one module. Wiring only.
cmd/pushapp-ui/   Wails v3 module switcher — SEPARATE Go module, see below.
cmd/probe/        USB descriptor dump (read-only, never opens the device)
cmd/frametest/    display-only probe, one frame or a timed hold
cmd/mapcheck/     cross-references captures against the button map
cmd/midiouttest/  MIDI-out probe: create/attach a port, send, and receive back
internal/bootstrap/  hardware-opening sequence shared by cmd/pushapp and -ui
internal/module/  the ABI: Module, Host, Frame/Op, Event, Meta, Store
internal/host/    runtime: registry, control API, event fan-out, frame loop
internal/host/procmod/       process-loaded module: JSON-over-stdio protocol
internal/display/ USB transport: claim interface 0, frame header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/midiout/ owns a named MIDI out port for modules (create or attach)
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
modules/monitor/  control-surface monitor; the reference module
modules/thru/     forwards pads/encoders/buttons out as MIDI
modules/seq/      8-step pad-grid sequencer; wall-clock-driven MIDI + Store
modules/remap/    user-editable overrides on top of thru's passthrough default
examples/modules/ process-loaded example modules (Python, Node.js)
tools/            macOS-only Swift probes (midimon, ledtest)
```

Full package-by-package rationale: [docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

### `cmd/pushapp-ui` is a separate Go module — do not add it to root's `./...`

It has its own `go.mod` with two `replace` directives (root repo,
`ableton-push-hack/core`), needs `wails3` (the CLI) + Node/npm to build, and
its config lives in `build/config.yml` (v3, not v2's `wails.json`). CI builds
it on all three OSes. Full detail:
[docs/guides/development-setup.md](docs/guides/development-setup.md).

## Writing a module

The contract is `internal/module.Module` — `Meta`, `Init`, `Handle`, `Draw`,
`Close`. `modules/monitor` is the reference implementation. Modules can also
be **any executable, any language** — `internal/host/procmod` runs one as a
child process over JSON-over-stdio; `examples/modules/` has working Python
and Node.js modules.

Guides: [writing-a-go-module.md](docs/guides/writing-a-go-module.md),
[writing-a-process-module.md](docs/guides/writing-a-process-module.md),
[writing-a-python-module.md](docs/guides/writing-a-python-module.md),
[writing-a-javascript-module.md](docs/guides/writing-a-javascript-module.md).
Architecture: [architecture/module-host.md](docs/architecture/module-host.md),
[architecture/process-modules.md](docs/architecture/process-modules.md).

```bash
go run ./cmd/pushapp -install path/to/your-module   # copies it in, registers it
go run ./cmd/pushapp -uninstall your-module-id
go run ./cmd/pushapp -list                          # shows installed too, [installed]
go run ./cmd/pushapp -module your-module-id
```

## Commands

```bash
go run ./cmd/pushapp            # host + first module
go run ./cmd/pushapp -list      # list compiled-in modules
go run ./cmd/pushapp -module monitor
go run ./cmd/probe        # dump USB descriptors
go run ./cmd/frametest    # claim interface 0, push one frame
go run ./cmd/midiouttest  # prove MIDI reaches other software
go build ./... && go vet ./... && go test ./...
```

`pushapp` flags: `-fps`, `-module <id>`, `-list`, `-no-display` (MIDI only),
`-no-leds`, `-midi-out <name>`, `-no-midi-out`, `-capture`, `-capture-raw`,
`-install <dir>`, `-uninstall <id>` (filesystem-only, no Push needed).

```bash
cd cmd/pushapp-ui
wails3 dev              # hot-reload window; needs wails3 + Node/npm
wails3 build            # produces bin/pushapp-ui
```

Full flag reference and probe tools: [docs/guides/debugging.md](docs/guides/debugging.md).

## Cross-platform builds

**No cross-compiling this app.** `gousb` (libusb) and `rtmididrv` (vendored
RtMidi C++) are both cgo — **build natively on each target OS**.
`.github/workflows/build.yml` does this on real macOS/Linux/Windows runners.
Per-OS setup: [docs/platform/macos.md](docs/platform/macos.md),
[docs/platform/linux.md](docs/platform/linux.md),
[docs/platform/windows.md](docs/platform/windows.md).

## Architecture decisions already made

Rationale in [docs/archive/feasibility.md](docs/archive/feasibility.md) §6
and [docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

- **Go**, single static binary, for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Cost: no cross-compilation, LGPL-2.1.
- **`gitlab.com/gomidi/midi/v2` + `drivers/rtmididrv`** for OS MIDI — no
  brew/apt dependency across all three OSes. Do not add rtmidi/portmidi as
  system packages.
- **Push's MIDI is read through the OS, never libusb.**
- **Wails v3** for the UI. Depends on `webkit2gtk` on Linux.

## Known constraints (high-churn — check docs for current status)

- **Screen exclusivity.** Live running as Push's control surface makes
  claiming interface 0 fail with `LIBUSB_ERROR_ACCESS`, cleanly. Handle
  explicitly: report "Live owns the display" and degrade, don't crash.
- **Windows MIDI input port naming** doesn't expose jack strings the way
  CoreMIDI/ALSA do — name-based auto-detect can't work there. Fixed with a
  manual port picker, confirmed 2026-08-18 on real Push 3 hardware (Windows
  11 VM + USB passthrough). Detail:
  [docs/platform/windows.md](docs/platform/windows.md).
- **Disconnect detection.** `cmd/pushapp-ui` now notices when Push is
  unplugged mid-session (`display.ErrDisconnected` bubbles up through
  `host.Runtime.Run` to `hostManager`, which flips `IsConnected` and exposes
  `LastError`) and falls back to the port-picker view instead of showing a
  stale module list against a dead port.
- **Don't run `pushapp` with Live open.** Co-existence mode leaves Push's
  MIDI interface bound to the OS driver even while Live doesn't own the
  display, so both processes end up driving the same pad LEDs — visible
  fighting, not just a display conflict. There's no arbitration between the
  two; the two are simply incompatible while both hold a MIDI connection to
  the device. See [docs/protocol/led-output.md](docs/protocol/led-output.md).
- **Channel convention: 1-16 at every API in this repo**, converted to the
  wire's 0-15 inside `midiout`.

Full list, including Push 2/3 deltas and unmeasured items:
[plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md),
[docs/protocol/push2-vs-push3.md](docs/protocol/push2-vs-push3.md).
