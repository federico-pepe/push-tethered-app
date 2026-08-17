# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository.

> **Doc sync rule:** keep this file, `README.md`, and `docs/` in sync with every
> code change. If a change affects behaviour, protocol facts, APIs or known
> issues — update the relevant docs in the same commit.
>
> **Plans live in `plans/`**, one file per plan, named
> `YYYY-MM-DD-name-of-the-plan.md` (date first so they sort chronologically).
> Write plans there rather than leaving them in chat or scattered across
> `docs/`. `docs/` holds durable reference — protocol facts, measurements,
> rationale; `plans/` holds intent — what we are about to do and why, including
> decisions that are still open.
>
> **`docs/archive/` is frozen — never edit, move, or delete anything inside
> it**, and never add anything to it unless explicitly asked to archive a
> specific file. It holds superseded docs (e.g. `feasibility.md`) kept for
> history; treat it as read-only. Current open questions live in
> [docs/open-questions.md](docs/open-questions.md) instead — update that file,
> not the archive, when something gets resolved or newly discovered.

## Project

`push-tethered-app` — cross-platform desktop app that owns an **Ableton Push 2 /
Push 3 in tethered (controller) mode**: display, pads, buttons, encoders, LEDs.
Goal is a fully configurable MIDI controller independent of any DAW.

**Status: pre-alpha, but running.** Protocol verification is done (§8).
`cmd/pushapp` is a working vertical slice: one binary holding the screen at
30fps, reading the control surface and driving the LEDs, confirmed on hardware
(§9). No configuration, mapping or UI yet.

**The open question is what v1 actually is** — co-existence mode cannot remap
MIDI, so the stated goal needs full ownership. See
[plans/2026-08-16-product-shape-decision.md](plans/2026-08-16-product-shape-decision.md).
Do not build mapping/config features until that is decided.

Read [docs/archive/feasibility.md](docs/archive/feasibility.md) before doing
anything substantial. It carries the protocol evidence, the ranked blockers,
and the stack rationale, with section numbers referenced throughout this file
(it's archived/frozen — don't edit it, see the doc-sync rule above). Check
[docs/open-questions.md](docs/open-questions.md) for what's still unresolved
since that snapshot.

## Relationship to `ableton-push-hack`

This project is a **sibling** of `~/Documents/GitHub/ableton-push-hack`, which
targets Push 3 *standalone* (hacks deployed to the device over SSH). Two things
matter here:

1. **`core/` is reused, not copied.** `go.mod` has a `replace` pointing at that
   checkout's `core/` module (`github.com/federico-pepe/ableton-push-hack/core`).
   Reused packages: `core/gfx`, `core/gfx/text`, `core/gfx/widgets`,
   `core/display` (the `ToBGR565`/`FromBGR565` codec), `core/push3`
   (geometry, LED palette, encoder helpers), and `core/httpx`/`core/sse`/
   `core/hackcfg` if a local web UI appears.
   - **Do not fork or vendor these.** If a change is needed, make it in
     `ableton-push-hack` so both projects benefit.
   - `core/alsaseq`, `core/display.Shm` and `core/pmclient` are **not** usable
     here — they are on-device Linux plumbing. Exception: `core/alsaseq` becomes
     relevant again if this app ever creates a virtual MIDI port *on Linux*.
2. **That repo's hard safety rules do not apply here** (no `/boot`, `/opt`, or
   `/etc` on the Push filesystem is involved) — but see USB safety below.

## Key protocol facts

Push 3, controller mode, measured on macOS 2026-08-09 — see README for the full
interface table.

- **VID `0x2982`, PID `0x1969`** (Push 2: `0x1967`). Composite device, USB 2.0,
  1 configuration, 7 interfaces.
- **Interface 0 = `Ableton Push 3 Display`**, vendor-specific
  (class/subclass/protocol all `255`), 2 endpoints. This is the one to claim.
- **Interface 5 = MIDIStreaming** (class 1, subclass 3), 2 endpoints → the three
  CoreMIDI ports `Live Port` / `User Port` / `External Port`.
- **Interface 6 = `xPort`**, vendor-specific, 2 endpoints, **undocumented**.
  Purpose unknown; do not poke it blind (see USB safety).
- Display format — **confirmed working tethered 2026-08-09**: bulk OUT ep `0x01`,
  16-byte header `FF CC AA 88` + 12 × `00`, then 960×160 BGR565 LE, stride
  1024 px, XOR `0xFFE7F3E7`. `core/display.ToBGR565` emits the payload unchanged
  (no tethered variant needed); the XOR is applied on top.
- **A single 327680-byte frame is sufficient.** The standalone device's frame
  duplication is a quirk of Ableton's binary, not a hardware requirement.
- **The screen must be refreshed continuously.** One frame flashes and is then
  overwritten — with no host driving it, Push redraws its own "connect to a
  computer" idle screen. Holding the display means outrunning that renderer.
  30fps costs 9.4 MB/s and runs clean; 60fps is well within USB 2.0 budget.

## MIDI facts (measured 2026-08-09, Live closed — §8.7)

Push emits MIDI **with no host handshake**, on `Ableton Push 3 Live Port` only
(`User Port` / `External Port` carry nothing but keepalive).

- **MPE is on by default — but NOT always (§9.5).** Measured 2026-08-09 pad
  note-ons rotated across **channels 2-16**; on 2026-08-16 the same setup put
  pads on **channel 1**. The trigger is unidentified. Handle both; never assume
  one layout. Channel 1 is always the control surface. Per-note pressure, CC 74 slide and pitch
  bend all arrive on the note's own member channel.
- **Decode channel FIRST, then CC.** Push 2 assigns CC 71-79 to the nine
  encoders, and CC 71/74 are *also* MPE timbre controllers. The numbers collide;
  the channel disambiguates. Treating CC 74 as "encoder 4" without checking the
  channel turns pad slide into phantom encoder movement.
- **Pads:** 8×8, notes **36 (bottom-left) to 99 (top-right)**, both corners
  verified.
- **Encoders:** relative two's-complement — `1` = +1 click, `127` = -1.
  Encoder 1 = CC 71. `core/push3.DecodeRel` handles this unchanged.
- **Touch sensors.** Encoders 1-8 = notes **0-7**, volume wheel = **8**, note 9
  **unused**, tempo = 10, jog = 11, touch strip = **12**, D-Pad center = 13.
  Note On vel 127 = contact. These were off by one upstream (§8.8) but the
  correction has since been applied to `core/push3` itself (confirmed on both
  Push 3 and Push 2) — `core/push3` is directly authoritative now,
  `internal/pushmap` no longer overrides touch notes.
- **Jog wheel is CC 70 and IS a relative encoder.** `push3.IsEncoderCC` now
  covers it directly — the omission from §9.4 was fixed upstream. Decoding
  CC 70 as a button turns every jog turn into a stream of phantom button
  presses, which is what the original bug looked like.
- **Encoders accelerate.** Deltas up to ±11 on fast turns — always use
  `push3.DecodeRel`'s signed value, never assume one message = one click.
- Encoder direction is `1` = CW, `127` = CCW — `core/push3/buttons.go` and its
  map doc used to state this backwards (§8.8); both were corrected upstream.
- **Buttons:** CC, 127 press / 0 release. CC 104-107 above the screen,
  CC 20-22 below.
- **Filter Active Sensing.** Push sends `0xFE` ~37×/second — over half of all
  traffic. Test for system realtime (`0xF8`-`0xFF`) *before* masking with
  `0xF0`, or `0xFE` decodes as SysEx.

### LED output (§8.9)

- **Pads:** Note On ch1, note 36-99, **velocity = palette index** from
  `core/push3/colors.go`, `0` = off. All 64 confirmed lit.
- **Buttons:** CC ch1, **value = brightness** 0-127 (white LEDs ignore colour).
- No handshake needed. Works in co-existence mode over CoreMIDI — do not claim
  interface 5 just to drive LEDs.
- A row-walk from note 36 ran bottom-to-top, confirming `push3.PadNote` /
  `PadCoord` from the output side as well as the input side.
- **Always clear LEDs on every exit path, including SIGINT.** A probe that
  leaves the device lit makes the next run ambiguous.

**The button map is now complete: Push 3 87/87 CC and 13/13 touch notes, Push 2
75/80 and 12/14, zero unknowns on either (§10.6, §12).** Two CCs mean different
things per device — CC 15 (Push 2 Swing turn / Push 3 tempo press) and CC 111
(Push 2 Browse / Push 3 volume press) — so **always resolve CCs per device**.

Still unmeasured: button-LED brightness fidelity, whether MPE can be disabled
via SysEx, what `User Port` / `External Port` are for, and Push 2's arrow
down/right (expected to match Push 3's 46/47/44/45).

Probe tools are macOS-only Swift, not part of the app build, kept so the
measurements stay reproducible: `tools/midimon.swift` (MIDI in),
`tools/ledtest.swift` (LED out). `cmd/mapcheck` (Go) cross-references captures
against the map.

## Drawing on the screen

- **ASCII only.** `core/gfx/text` uses `basicfont.Face7x13`; any non-ASCII
  character (em-dash, ellipsis, accents) renders as a missing-glyph box on the
  panel (§9.4).
- **Look at the screen, not just the logs.** Both bugs in §9.4 were invisible in
  terminal output that reported healthy frame rates throughout. Use
  `pushapp -capture out.mp4` and inspect a frame.

## Hardware-interaction safety (button sweeps)

- **Not every Push 3 button is an inert MIDI sender in controller mode.** The
  **leftmost button above the screen switches the device into standalone mode**,
  dropping it out of controller mode mid-session (found 2026-08-16 when the
  operator stopped a sweep before pressing it).
- **Never instruct a blind "press every button" sweep.** Ask which controls have
  device-level functions first. A sweep that reboots the device into another
  mode loses the session and can leave the USB state ambiguous.
- **Fix: hold the display first.** Run `cmd/pushapp` before sweeping — the
  top-row buttons are soft buttons owned by Push's own idle UI, so once a host
  drives the screen they become plain MIDI and are safe to press (§12.1).
- **Identify ambiguous controls by their touch sensor, not by press order.** A
  press bracketed by a touch note on/off proves which physical control it
  belongs to (§12.2). Press order has already misled once (§10.6).
- Recovery is simply switching back to controller mode from the device, but the
  capture in progress is void.

## USB safety

The device is expensive, and some of this is undocumented. Rules:

- **Claim only the interface you need.** Default to interface 0 (display).
  Claiming MIDI/audio interfaces takes them away from the OS and the DAW.
- **Never write to `xPort` (interface 6) speculatively.** It is vendor-specific
  and unidentified. Read/enumerate freely; do not send it invented payloads.
- **No firmware operations. Ever.** No DFU, no control transfers with vendor
  requests whose meaning is unknown.
- **Never run against a Push that is mid-OS-update.**
- If a display write wedges the screen, replug/power-cycle the device. That is
  the expected worst case and it is recoverable — keep it that way.

## Layout

```
cmd/pushapp/      the app: display + input + LEDs in one process
cmd/probe/        USB descriptor dump (read-only, never opens the device)
cmd/frametest/    display-only probe, one frame or a timed hold
cmd/mapcheck/     cross-references captures against the button map
cmd/midiouttest/  MIDI-out probe: create/attach a port, send, and receive back
internal/display/ USB transport: claim interface 0, frame header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/midiout/ owns a named MIDI out port for modules (create or attach)
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
tools/            macOS-only Swift probes (midimon, ledtest)
```

## Commands

```bash
go run ./cmd/pushapp      # the vertical slice: screen + input + LEDs
go run ./cmd/probe        # dump USB descriptors: interfaces, altsettings, endpoints
go run ./cmd/frametest    # claim interface 0, push one frame to the display
go run ./cmd/midiouttest  # prove MIDI reaches other software on this machine
go build ./... && go vet ./... && go test ./...
```

`pushapp` flags: `-fps`, `-no-display` (MIDI only), `-no-leds`, `-capture`.

`midiouttest` flags: `-list`, `-port <name>`, `-ch <1-16>`, `-bpm`, and
`-listen <name>` to become the receiver instead of the sender. The two halves
prove each other with no synth involved — run `-listen` in one terminal against
the port the sender creates.

## Cross-platform builds

**There is no cross-compiling this app from one machine.** `gousb` (libusb) and
`rtmididrv` (vendored RtMidi C++) are both cgo — cgo cross-compilation needs a
full C cross-toolchain and the target's native libraries, not just a Go
`GOOS`/`GOARCH` switch. **Build natively on each target OS.**
`.github/workflows/build.yml` does this on real `macos-latest` /
`ubuntu-latest` / `windows-latest` runners — that is the actual "build for all
three platforms" answer; a laptop cannot do it alone.

Per-OS setup for a local build:

- **macOS:** `brew install libusb`, then
  `export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH`
  (1.0.30 confirmed working).
- **Linux:** `sudo apt install libusb-1.0-0-dev libasound2-dev pkg-config
  build-essential` (Debian/Ubuntu; package names differ elsewhere). The ALSA
  package is easy to miss — `rtmididrv`'s Linux backend needs
  `alsa/asoundlib.h` and fails at the cgo compile step, not link, if it's
  absent. Also needs a **udev rule** to claim the display without root:
  `SUBSYSTEM=="usb", ATTR{idVendor}=="2982", MODE="0666"` in
  `/etc/udev/rules.d/`, then `udevadm control --reload-rules && udevadm
  trigger` and replug. Confirmed 2026-08-16: `cmd/probe` builds and runs
  unmodified.
- **Windows:** needs a mingw-w64 toolchain (MSYS2) for cgo, plus libusb via
  MSYS2/vcpkg. MIDI uses WinMM, built into Windows. **Not yet tried on real
  Windows hardware** — CI covers compilation only, not the driver-conflict risk
  already documented under Known constraints.

`go.mod`'s `replace` directive for `core/` is a relative path
(`../../Documents/GitHub/ableton-push-hack/core`) — a fresh clone on any OS
needs that sibling repo checked out at the matching relative location, or the
path edited to match wherever it actually lives.

**CI checks out `ableton-push-hack@main`** for the `core/` dependency, into a
fixed subdirectory of its own workspace, then runs `go mod edit -replace` to
point at that checkout — CI-only, never touching the committed `go.mod`. This
was not straightforward: a naive attempt to mirror the local sibling-repo
layout inside the CI workspace does not actually satisfy the relative
`../../` path, because GitHub's runner nests the workspace differently per OS.
`push-core-refactor` (the branch holding the entire `core/` extraction) merged
into `main` on 2026-08-17 — before that, this workflow was expected to fail on
every OS for that reason, documented in the workflow file's own history.

### gousb gotcha — do not enable autodetach

**Never call `dev.SetAutoDetach(true)`.** It is *config-wide*, not
interface-wide: `Device.Config()` loops over every interface in the
configuration and detaches each one. On Push that tears audio (1-3) and MIDI
(4-5) away from the OS class drivers, destroying co-existence mode — and on
macOS it fails outright with `LIBUSB_ERROR_ACCESS`. If a Linux run reports
`LIBUSB_ERROR_BUSY` when claiming, detach interface 0 alone.

`Device.Config(n)` only issues `set_configuration` when `n` differs from the
active config. Push is already on config 1, so no disruptive reconfiguration
happens.

## Architecture decisions already made

Recorded so they are not relitigated — rationale in `docs/archive/feasibility.md` §6.

- **Go**, single static binary. Chosen for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Accepted costs: no cross-compilation
  (needs a per-OS CI matrix; mingw-w64 on Windows) and libusb's LGPL-2.1.
- **OS MIDI = `gitlab.com/gomidi/midi/v2` + `drivers/rtmididrv`** (chosen
  2026-08-16). The driver **vendors the RtMidi C++ sources**, so there is no
  brew/apt dependency — cgo compiles it in. One dependency covers macOS, Linux
  and Windows. Do not add rtmidi/portmidi as system packages.
- **Push's MIDI is read through the OS, never libusb** (revised 2026-08-17).
  §6.1a made this conditional on the operating mode; the module-host decision
  makes it unconditional. `internal/midi` uses the OS API on all three OSes.
  Claiming interface 5 over libusb (ep `0x03`/`0x83`) is not planned.
- **MIDI *out* to other software goes through `internal/midiout`**, which owns a
  named port rather than assuming it can create one — see Known constraints for
  the measured per-OS behaviour.
- **Wails v3** for the UI when a UI is needed. Note it depends on `webkit2gtk`
  on Linux — the one place the stack is not truly standalone. Fyne/Gio are the
  fallback if that becomes unacceptable.
- **Rust + `nusb`** was considered and is genuinely better engineering (pure
  Rust, no cgo, no LGPL). Rejected only because it forfeits `core/` reuse.
  Revisit if this becomes a distributed product.

## Operating model — decided 2026-08-17

**The product is a module host.** `pushapp` owns the hardware and runs
**modules** — small programs, writable by anyone, that draw the screen and
handle pads/encoders/buttons. No DAW is involved at any layer. See
[plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md).

This retired the old co-existence / full-ownership split, so **do not plan
against it**:

- **"Full ownership" does not mean claiming interface 5.** It means *we are the
  only host*. OS MIDI via `rtmididrv` works on all three OSes with no driver
  install, and WinMM's exclusive-open only ever hurt when sharing Push with a
  DAW. **The libusb MIDI backend is out of scope** — a possible later latency
  optimisation, nothing more.
- **Co-existence is not a shipping mode**, just a fact about `ErrBusy`: if Live
  holds the display we degrade and say so.
- **A remapper is a module, not the product** (the old option B).

## Known constraints

- **Screen exclusivity (§4.1) — measured 2026-08-09.** With Live running and
  Push as its control surface, claiming interface 0 fails with
  `LIBUSB_ERROR_ACCESS` ("libusb: bad access [code -3]"), cleanly at claim time
  before any write. Everything else survives: USB enumeration, all 3 MIDI ports,
  and 16×16 audio. The claim releases as soon as Live quits — no replug. Handle
  this error explicitly in any display code: report "Live owns the display" and
  degrade, don't crash.
- **MIDI out to other software — solved 2026-08-17, was §6.2's "hardest
  constraint".** The app does **not** create a virtual port; it **owns a named
  output port**, obtained two ways, both in `internal/midiout`:
  - **create** — `rtmididrv.Driver.OpenVirtualOut(name)`
    (`drivers/rtmididrv/driver.go:105`). macOS calls `MIDISourceCreate`
    (`RtMidi.cpp:1637`), Linux creates an ALSA seq port (`:2553`).
    **Verified working end-to-end on macOS 2026-08-17** — notes and CC sent from
    Go were received back through the published port by a second process.
  - **attach** — Windows WinMM refuses to create one:
    `"MidiOutWinMM::openVirtualPort: cannot be implemented in Windows MM MIDI
    API!"` (`RtMidi.cpp:3128`, WinUWP the same at `:3947`). It is a *warning*, no
    port. So on Windows we open an **existing** port by name; the user provides
    it with loopMIDI (free) or Windows MIDI Services.

  `midiout.Open` tries create then falls back to attach, with no build tags — so
  Windows would pick up native virtual ports automatically if that ever lands.
  Cost is one documented install step on Windows, not a blocker. **Never attach
  to a port whose name mentions Push** — that loops our output back into the
  decoder; `midiout.isPush` guards it and a test pins it.
- **Channel convention: 1-16 at every API in this repo**, converted to the
  wire's 0-15 inside `midiout`. `gomidi`'s `Message.String()` prints channels
  **0-based**, so a message sent on channel 3 displays as `channel: 2`. That is
  correct, not an off-by-one.
- **Push 2 works from the same binary** (§10, measured 2026-08-16). Display,
  pads and LEDs are identical to Push 3. Its map is swept to 75/80 CC with zero
  unknowns; the five differences live in `internal/pushmap/push2.go` — CC 15
  Swing, 52 Master, 53 Stop Clip, 87 New (Push 3 uses 92), 111 Browse. **Use
  `pushmap.ButtonNameFor`/`TouchNameFor`/`IsRelativeEncoderCCFor`**, not the
  device-agnostic versions, wherever the device is known.
- **Push 2's note 9 is the Swing encoder touch** — the note left unused on
  Push 3, and the reason the upstream touch numbering was off by one (§10.6).
- **Push 2 arrow CCs down/right are unverified** (§10.6): observed 45/47 in an
  order that may not have matched the instruction.
- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, undocumented,
  absent from Push 2's spec. Purpose unknown; enumerate freely, never send it
  invented payloads.
- **Endpoint `0x81` IN** on the display interface is unused by the write path.
  Possibly status/ack. Never read from so far.
- **Push 3 audio reports 16 in / 16 out** @ 44.1kHz — more than its analog I/O
  count, probably internal routing buses. Not investigated.
