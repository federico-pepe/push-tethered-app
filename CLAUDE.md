# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository.

> **Doc sync rule:** update this file, `README.md`, and `docs/` when a change
> is *meaningful* to a future reader — new behaviour, a changed protocol fact,
> a new API, a resolved or newly-found issue. Not every commit needs a doc
> update; skip internal refactors and trivial edits. When you do update, keep
> it in the same commit as the change.
>
> **Plans live in `plans/`**, one file per plan, named
> `YYYY-MM-DD-name-of-the-plan.md`. `docs/` holds durable reference (protocol
> facts, measurements, rationale); `plans/` holds intent (what we're about to
> do and why, including open decisions).
>
> **`docs/archive/` is frozen** — never edit, move, or delete anything inside
> it, never add to it unless explicitly asked. It holds superseded docs kept
> for history. Open questions live in
> [docs/open-questions.md](docs/open-questions.md) instead.

## Project

`push-tethered-app` — cross-platform desktop app that owns an **Ableton Push 2 /
Push 3 in tethered (controller) mode**: display, pads, buttons, encoders, LEDs.

**It is a module host.** `pushapp` owns the hardware and runs **modules** —
small programs anyone can write — that draw the screen and handle the
controls. No DAW is involved at any layer; a MIDI remapper is *a module*, not
the product. Decided 2026-08-17, see
[plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md).

**Status: pre-alpha, running, confirmed on Push 2 and Push 3 hardware.** The
module contract, host, renderer and per-module persistence all work. Four
built-in modules: `monitor` (control-surface mirror), `thru` (forwards
controls out as MIDI), `seq` (8-step pad-grid sequencer), `remap`
(user-editable overrides on `thru`). `cmd/pushapp-ui` is a Wails v3 switcher
(list/switch/install/uninstall modules). **Modules can be any executable, not
just compiled-in Go** — `internal/host/procmod` runs one as a child process
over JSON-over-stdio; `examples/modules/` has working Python and Node.js
modules, confirmed end-to-end on hardware. See
[plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md).

[plans/2026-08-16-product-shape-decision.md](plans/2026-08-16-product-shape-decision.md)
is **closed** — it framed three candidate products and the answer was a
fourth. Read it for the reasoning trail only; do not plan against it.

Read [docs/archive/feasibility.md](docs/archive/feasibility.md) for the
protocol evidence and stack rationale behind the facts below (frozen, don't
edit — section numbers below refer to it). Check
[docs/open-questions.md](docs/open-questions.md) for what's still open.

## Relationship to `ableton-push-hack`

Sibling of `~/Documents/GitHub/ableton-push-hack` (Push 3 *standalone*, deployed
over SSH). Two things matter:

1. **`core/` is reused, not copied**, via a `replace` in `go.mod` pointing at
   that checkout's `core/` module. Reused: `core/gfx`, `core/gfx/text`,
   `core/gfx/widgets`, `core/display` (`ToBGR565`/`FromBGR565`), `core/push3`
   (geometry, LED palette, encoder decode). **Never fork or vendor these** —
   fix upstream so both projects benefit. `core/alsaseq`, `core/display.Shm`,
   `core/pmclient` are on-device Linux plumbing, not usable here.
2. **That repo's hard safety rules (no `/boot`, `/opt`, `/etc`) don't apply
   here** — but see USB safety below.

## Key protocol facts

Push 3, controller mode, measured on macOS. Push 2 PID `0x1967`, Push 3
`0x2982`/`0x1969`.

- **Interface 0** = display, vendor-specific, 2 endpoints — the one to claim.
  **Interface 5** = MIDIStreaming → CoreMIDI's `Live Port`/`User Port`/
  `External Port`. **Interface 6** = `xPort`, vendor-specific, undocumented —
  enumerate freely, never write to it speculatively.
- **Display:** bulk OUT ep `0x01`, 16-byte header `FF CC AA 88` + 12×`00`,
  960×160 BGR565 LE, stride 1024px, XOR `0xFFE7F3E7`. **One 327680-byte frame
  is sufficient** — the standalone device's frame duplication is a quirk of
  Ableton's binary, not a hardware requirement.
- **The screen must be refreshed continuously** — with no host driving it,
  Push redraws its own idle screen over whatever was last sent. 30fps
  (9.4 MB/s) runs clean; 60fps is well within USB 2.0 budget.

## MIDI facts

Push emits MIDI with **no host handshake**, on `Live Port` only (`User`/
`External` carry keepalive only).

- **MPE is on by default — but not always.** Pad note-ons have been observed
  rotating across channels 2-16, and separately all on channel 1, with no
  identified trigger. Handle both. Channel 1 is always the control surface;
  per-note pressure, CC 74 slide and pitch bend arrive on the note's own
  channel.
- **Decode channel FIRST, then CC.** CC 71-79 are the nine encoders, but CC
  71/74 are also MPE timbre controllers — the channel disambiguates.
- **Pads:** 8×8, notes 36 (bottom-left) to 99 (top-right).
- **Encoders:** relative two's-complement (`1`=+1 click, `127`=-1), encoder 1
  = CC 71, direction `1`=CW/`127`=CCW. **Accelerate** — deltas up to ±11 on
  fast turns, always decode the signed value, never assume one message = one
  click. **Jog wheel is CC 70 and is a relative encoder too** —
  `push3.IsEncoderCC` covers it.
- **Touch sensors:** encoders 1-8 = notes 0-7, volume wheel = 8, note 9
  unused, tempo = 10, jog = 11, touch strip = 12, D-Pad center = 13. Note On
  vel 127 = contact. `core/push3` is authoritative for these now (an earlier
  off-by-one was fixed upstream).
- **Buttons:** CC, 127 press / 0 release. CC 104-107 above the screen, CC
  20-22 below. **Filter Active Sensing** (`0xFE`, ~37/sec, over half of all
  traffic) — test for system realtime (`0xF8`-`0xFF`) before masking with
  `0xF0`, or it decodes as SysEx.
- **Button map is complete**: Push 3 87/87 CC + 13/13 touch, Push 2 75/80 +
  12/14, zero unknowns either device. Two CCs differ per device — CC 15
  (Push 2 Swing / Push 3 tempo) and CC 111 (Push 2 Browse / Push 3 volume) —
  always resolve CCs per device via `pushmap.ButtonNameFor`/`TouchNameFor`/
  `IsRelativeEncoderCCFor`, not the device-agnostic versions.

Still unmeasured: button-LED brightness fidelity, whether MPE can be disabled
via SysEx, what `User`/`External` ports are for, Push 2's arrow down/right
(expected to match Push 3's 46/47/44/45).

Probe tools (macOS-only Swift, not part of the app build):
`tools/midimon.swift` (MIDI in), `tools/ledtest.swift` (LED out).
`cmd/mapcheck` cross-references captures against the map.

### LED output

- **Pads:** Note On ch1, note 36-99, velocity = palette index from
  `core/push3/colors.go`, `0` = off.
- **Buttons:** CC ch1, value = brightness 0-127 (white LEDs ignore colour).
- No handshake needed. Works over CoreMIDI without claiming interface 5.
- **Always clear LEDs on every exit path, including SIGINT** — leaving the
  device lit makes the next run ambiguous.

## Drawing on the screen

- **ASCII only.** `core/gfx/text` uses `basicfont.Face7x13`; any non-ASCII
  character renders as a missing-glyph box. Write ASCII rather than relying on
  the host's substitution.
- **Look at the screen, not just the logs** when debugging — a healthy frame
  rate in logs doesn't mean the frame rendered correctly.
  `pushapp -capture out.mp4` records the screen (never the physical pad LEDs)
  for inspection.

## Hardware-interaction safety (button sweeps)

- **The leftmost button above the screen switches Push 3 into standalone
  mode**, dropping out of controller mode mid-session.
- **Never do a blind "press every button" sweep** — ask which controls have
  device-level functions first.
- **Fix: hold the display first.** Run `cmd/pushapp` before sweeping — once a
  host drives the screen, the top-row buttons become plain MIDI and are safe
  to press.
- **Identify ambiguous controls by their touch sensor, not by press order** —
  a press bracketed by a touch note on/off proves which physical control it
  belongs to.
- Recovery: switch back to controller mode from the device. The capture in
  progress is void, but nothing else is lost.

## USB safety

- **Claim only interface 0 (display).** Claiming MIDI/audio interfaces takes
  them away from the OS and the DAW.
- **Never write to `xPort` (interface 6) speculatively** — vendor-specific,
  undocumented.
- **No firmware operations. Ever.** No DFU, no control transfers with unknown
  vendor requests.
- **Never run against a Push mid-OS-update.**
- A wedged display recovers with a replug/power-cycle — expected worst case,
  keep it that way.

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
internal/module/moduletest/  fake Host so modules test with no hardware
internal/host/    runtime: registry, control API, event fan-out, frame loop
internal/host/render.go      op registry: display list -> image via core/gfx
internal/host/store.go       per-module JSON persistence, atomic writes
internal/host/procinstall.go Runtime.Install/Uninstall/LoadInstalled
internal/host/procmod/       process-loaded module: JSON-over-stdio protocol,
                              manifest.json, the supervisor (Proc)
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

### `cmd/pushapp-ui` is a separate Go module — do not add it to root's `./...`

It has its own `go.mod`, so the main `go build ./... && go vet ./... && go
test ./...` stays untouched by Wails and by webkit2gtk (Linux). Consequences:

- Its `go.mod` carries **two** `replace` directives of its own — back to this
  repo's root, and to `ableton-push-hack/core` — since a `replace` in a
  dependency's own `go.mod` is never honoured; only the main module's
  `replace`s apply.
- **Building it needs `wails3` (the CLI) and Node/npm**, on top of everything
  `cmd/pushapp` needs. Install: `go install
  github.com/wailsapp/wails/v3/cmd/wails3@latest`, then `wails3 doctor`.
- **Project config is `build/config.yml`, not `wails.json`** — v3 replaced v2's
  `wails.json`. Its `info:` block generates the assets under `build/darwin`,
  `build/windows`, `build/linux`; regenerate with `wails3 task
  common:update:build-assets` after editing, which overwrites hand-edits to
  those files.
- **Desktop only.** Wails' `ios/`/`android/` template targets were deleted
  2026-08-18 (libusb rules mobile out anyway). They used to break `go build
  ./...` here via their build-tagged `main_ios.go`/`main_android.go`; that
  now works. `build/ios`/`build/android` are gitignored since
  `update:build-assets` regenerates iOS assets regardless.
- **`wails3 build` produces a bare executable on every OS** — no `.app`, no
  installer. `wails3 package` makes those (.app/.dmg, AppImage/deb/rpm, NSIS);
  not wired into CI. A `pushapp-ui.dev.app` in `bin/` is `wails3 dev`'s doing,
  not a build output.
- It **can** import `internal/*` packages of the root module despite being a
  different module — Go's internal-visibility rule is based on import *path
  text*, and its module path shares the required prefix.
- **CI builds it too** (`.github/workflows/build.yml`, all three OSes) —
  `wails3 build`, reusing the root job's `core/` checkout and per-OS lib
  installs, plus `webkit2gtk-4.1-dev` on Linux and a `go mod edit -replace`
  for `cmd/pushapp-ui`'s own core/ replace.

## Writing a module

The contract is `internal/module.Module` — `Meta`, `Init`, `Handle`, `Draw`,
`Close`. `modules/monitor` is the reference implementation.

- **A module never draws pixels.** `Draw` appends ops to a `*module.Frame`;
  the host renders them via `core/gfx` + `core/gfx/widgets`. Use the typed
  methods (`f.Rect`, `f.Text`, `f.List`, …), never `AppendRaw` (that's for the
  process loader and tests).
- **`Handle` and `Draw` never run concurrently** — one goroutine, so module
  state is plain fields, no mutex.
- **Never block in `Handle`.**
- **The op set is open** — a new op is one `host.RegisterOp` + one `Frame`
  method, no ABI change. An unknown op is skipped, never fatal.
- **Declare `NeedsMIDIOut`** if the module sends MIDI (`modules/thru` is the
  reference). The host then refuses to activate it if no port can be opened.
  - **The port opens on activation, never earlier** — `host.Options` takes
    `OpenMIDIOut` as a **function**, not an open port, since opening one
    publishes it system-wide on macOS/Linux. A failed open is cached.
  - **Release your own notes in `Close`** — the host clears LEDs but knows
    nothing about notes in flight (see `thru`'s `held` set).
- **`Store()` gives the module its own persisted JSON** (one file per module
  ID, `internal/host/store.go`). Set defaults on your struct before `Get` —
  nothing stored yet just leaves defaults alone. The host logs the file path
  on activation for hand-editing (see `modules/remap`'s doc).
- **Test with `moduletest.Host`** (records every LED/MIDI write, no Push
  needed) and call `moduletest.NonASCIIStrings(f)` in Draw tests — op-kind
  tests alone miss non-ASCII content.

### Not writing Go? Modules can be any executable

`internal/host/procmod` runs a module as a child process, any language, one
JSON object per line over stdin/stdout — see
[plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md) for
the wire protocol, `examples/modules/hello-py`/`hello-js` for references. A
module is a directory with `manifest.json` (`id`, `name`, `exec`, optionally
`needs_midi_out`) plus its script/executable:

```bash
go run ./cmd/pushapp -install path/to/your-module   # copies it in, registers it
go run ./cmd/pushapp -uninstall your-module-id
go run ./cmd/pushapp -list                          # shows installed too, [installed]
go run ./cmd/pushapp -module your-module-id
```

Two things easy to get wrong in a new language:

- **Flush every line immediately** — the host reads one line at a time; a
  module that buffers stdout (Python's default off-terminal) looks hung.
- **The Image op is not available** — an `*image.NRGBA` doesn't cross a
  process boundary; raw pixel control needs an in-tree Go module.

`draw`'s response/ops are the same JSON shapes `internal/module`'s Go types
produce — a colour is `{"R":.,"G":.,"B":.,"A":.}` (Go's `image/color.NRGBA`
encoding, capitalised, no `json` tags of its own).

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

`midiouttest` flags: `-list`, `-port <name>`, `-ch <1-16>`, `-bpm`, `-listen
<name>` (become receiver instead of sender — proves both halves with no synth).

```bash
cd cmd/pushapp-ui
wails3 dev              # hot-reload window; needs wails3 + Node/npm
wails3 build            # produces bin/pushapp-ui
```

## Cross-platform builds

**No cross-compiling this app.** `gousb` (libusb) and `rtmididrv` (vendored
RtMidi C++) are both cgo, which needs a full C toolchain per target — **build
natively on each target OS**. `.github/workflows/build.yml` does this on real
macOS/Linux/Windows runners.

Per-OS local setup:

- **macOS:** `brew install libusb`, then
  `export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH`.
- **Linux:** `sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config
  build-essential` (ALSA is easy to miss — `rtmididrv` needs
  `alsa/asoundlib.h`). Also a udev rule to claim the display without root:
  `SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"` in
  `/etc/udev/rules.d/`, then `udevadm control --reload-rules && udevadm
  trigger` and replug.
- **Windows:** mingw-w64 toolchain (MSYS2) for cgo, libusb via MSYS2/vcpkg.
  MIDI uses WinMM, built in. **The display/USB path is still untried on real
  Windows hardware.** MIDI *has* touched real Windows hardware and failed
  once already — see the Windows MIDI port-naming entry under Known
  constraints.

`go.mod`'s `core/` `replace` is a relative path
(`../../Documents/GitHub/ableton-push-hack/core`) — a fresh clone needs that
sibling repo at the matching relative location, or the path edited to match.
CI checks out `ableton-push-hack@main` into a fixed subdirectory of its own
workspace and runs `go mod edit -replace` to point at it — CI-only, never
touching the committed `go.mod`.

### gousb gotcha — do not enable autodetach

**Never call `dev.SetAutoDetach(true)`.** It's *config-wide*, not
interface-wide — `Device.Config()` detaches every interface in the
configuration, tearing audio and MIDI away from the OS class drivers and
destroying co-existence mode (fails outright on macOS with
`LIBUSB_ERROR_ACCESS`). If Linux reports `LIBUSB_ERROR_BUSY` claiming, detach
interface 0 alone. `Device.Config(n)` only issues `set_configuration` when `n`
differs from the active config, so no disruptive reconfiguration happens on
Push (already config 1).

## Architecture decisions already made

Rationale in `docs/archive/feasibility.md` §6.

- **Go**, single static binary, for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Cost: no cross-compilation, LGPL-2.1.
- **`gitlab.com/gomidi/midi/v2` + `drivers/rtmididrv`** for OS MIDI — vendors
  RtMidi C++, so no brew/apt dependency across all three OSes. Do not add
  rtmidi/portmidi as system packages.
- **Push's MIDI is read through the OS, never libusb.** `internal/midi` uses
  the OS API on all three OSes; claiming interface 5 over libusb is not
  planned.
- **MIDI *out* to other software goes through `internal/midiout`**, which owns
  a named port rather than assuming it can create one.
- **Wails v3** for the UI. Depends on `webkit2gtk` on Linux — the one place
  the stack isn't standalone; Fyne/Gio are the fallback if that becomes
  unacceptable.
- **Rust + `nusb`** was considered (no cgo, no LGPL) but rejected — it
  forfeits `core/` reuse. Revisit if this becomes a distributed product.

## Operating model

**The product is a module host.** `pushapp` owns the hardware and runs
modules; no DAW involved at any layer. This retired the old co-existence /
full-ownership split:

- **"Full ownership" doesn't mean claiming interface 5** — it means *we are
  the only host*. OS MIDI via `rtmididrv` needs no driver install on any OS.
  The libusb MIDI backend is out of scope, a possible later latency
  optimisation only.
- **Co-existence is not a shipping mode**, just a fact about `ErrBusy`: if
  Live holds the display we degrade and say so.
- **A remapper is a module, not the product.**

## Known constraints

- **Screen exclusivity.** With Live running and Push as its control surface,
  claiming interface 0 fails with `LIBUSB_ERROR_ACCESS`, cleanly, before any
  write. Everything else survives (USB enumeration, all 3 MIDI ports, 16×16
  audio). The claim releases the instant Live quits, no replug. Handle this
  error explicitly: report "Live owns the display" and degrade, don't crash.
- **MIDI out to other software — solved.** The app does not create a virtual
  port; it **owns a named output port** two ways, both in `internal/midiout`:
  - **create** — `rtmididrv.Driver.OpenVirtualOut(name)`. Verified working
    end-to-end on macOS.
  - **attach** — Windows WinMM refuses to create a virtual port at all, so on
    Windows we open an **existing** one by name; the user provides it with
    loopMIDI (free) or Windows MIDI Services.

  `midiout.Open` tries create then falls back to attach, no build tags.
  **Never attach to a port whose name mentions Push** — that loops our output
  back into the decoder; `midiout.isPush` guards it.
- **Windows MIDI *input* port naming — real bug, found 2026-08-18.** WinMM
  doesn't expose the USB MIDI jack strings CoreMIDI/ALSA read `"Live Port"`/
  `"User Port"`/`"External Port"` from at all — it names the first cable after
  the bare device name and only prefixes the others (`Ableton Push 3 MIDI`,
  `MIDIIN2 (Ableton Push 3 MIDI)`, ...). `internal/midi`'s name-based
  auto-detect can never match there, for Push 2 or Push 3 alike. Fixed with a
  manual escape hatch, not a naming heuristic: `pmidi.ListInPorts`/
  `OpenNamed`, wired into `cmd/pushapp-ui` as a port-picker view that appears
  only when auto-detect fails. Not yet confirmed fixed on real Windows
  hardware.
- **No disconnect detection.** Unplugging Push mid-session leaves
  `cmd/pushapp-ui` reporting the last-active module against a dead port —
  `hostManager` has no watchdog on port health. See
  [docs/open-questions.md](docs/open-questions.md).
- **Channel convention: 1-16 at every API in this repo**, converted to the
  wire's 0-15 inside `midiout`. `gomidi`'s `Message.String()` prints channels
  0-based, so channel 3 displays as "channel: 2" — that's correct, not a bug.
- **Push 2 works from the same binary.** Display, pads, LEDs identical to
  Push 3. Its map is 75/80 CC with zero unknowns; the five differences live in
  `internal/pushmap/push2.go` — CC 15 Swing, 52 Master, 53 Stop Clip, 87 New
  (Push 3 uses 92), 111 Browse. Use `pushmap.ButtonNameFor`/`TouchNameFor`/
  `IsRelativeEncoderCCFor`, not the device-agnostic versions, wherever the
  device is known.
- **Push 2's note 9 is the Swing encoder touch** — unused on Push 3.
- **Push 2 arrow CCs down/right are unverified** — observed 45/47 in an order
  that may not have matched the instruction.
- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, undocumented,
  absent from Push 2. Enumerate freely, never send it invented payloads.
- **Endpoint `0x81` IN** on the display interface is unused by the write path.
  Possibly status/ack. Never read from so far.
- **Push 3 audio reports 16 in / 16 out** @ 44.1kHz — more than its analog I/O
  count, probably internal routing buses. Not investigated.
