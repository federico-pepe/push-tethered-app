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

Read [docs/feasibility.md](docs/feasibility.md) before doing anything
substantial. It carries the protocol evidence, the ranked blockers, and the
stack rationale, with section numbers referenced throughout this file.

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
- **Touch sensors — use `internal/pushmap`, NOT `core/push3`.** The shared map's
  touch notes are wrong (§8.8). Measured: encoders 1-8 = notes **0-7**, volume
  wheel = **8**, note 9 **unused**, tempo = 10, jog = 11, touch strip = **12**
  (absent upstream), D-Pad center = 13. Note On vel 127 = contact.
  `internal/pushmap` overrides only these; `core/push3` stays authoritative for
  pads, button CCs, encoder CCs, the LED palette and `DecodeRel`.
- **Jog wheel is CC 70 and IS a relative encoder** — but `push3.IsEncoderCC`
  omits it, so use **`pushmap.IsRelativeEncoderCC`** instead. Decoding CC 70 as
  a button turns every jog turn into a stream of phantom button presses (§9.4).
- **Encoders accelerate.** Deltas up to ±11 on fast turns — always use
  `push3.DecodeRel`'s signed value, never assume one message = one click.
- `core/push3/buttons.go:7` and that repo's map doc claim encoder "CW=127,
  CCW=1". **That prose is inverted** — CW sends `1`. `DecodeRel`'s code is
  correct. Deliberately not fixed upstream; see §8.8.
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

Still unmeasured: 77 of 85 button CCs, remaining encoders, button-LED
brightness fidelity, whether MPE can be disabled via SysEx, and what
`User Port` / `External Port` are for. `cmd/mapcheck`'s UNSEEN list tracks the
button-CC gap.

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
internal/display/ USB transport: claim interface 0, frame header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/pushmap/ map corrections + shared CC/touch name tables
tools/            macOS-only Swift probes (midimon, ledtest)
```

## Commands

```bash
go run ./cmd/pushapp      # the vertical slice: screen + input + LEDs
go run ./cmd/probe        # dump USB descriptors: interfaces, altsettings, endpoints
go run ./cmd/frametest    # claim interface 0, push one frame to the display
go build ./... && go vet ./... && go test ./...
```

`pushapp` flags: `-fps`, `-no-display` (MIDI only), `-no-leds`.

macOS needs libusb: `brew install libusb` (1.0.30 confirmed working) and
`export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH`.
`gousb` is cgo — **cross-compilation does not work**. Build on each target OS.

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

Recorded so they are not relitigated — rationale in `docs/feasibility.md` §6.

- **Go**, single static binary. Chosen for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Accepted costs: no cross-compilation
  (needs a per-OS CI matrix; mingw-w64 on Windows) and libusb's LGPL-2.1.
- **OS MIDI = `gitlab.com/gomidi/midi/v2` + `drivers/rtmididrv`** (chosen
  2026-08-16). The driver **vendors the RtMidi C++ sources**, so there is no
  brew/apt dependency — cgo compiles it in. One dependency covers macOS, Linux
  and Windows. Do not add rtmidi/portmidi as system packages.
- **Device MIDI path depends on the mode** (§6.1a — corrected 2026-08-09):
  full-ownership claims interface 5 over libusb (ep `0x03`/`0x83`);
  **co-existence must use an OS MIDI API**, because claiming interface 5 takes
  the CoreMIDI/ALSA ports away from the DAW. Multi-client MIDI is free on
  macOS/Linux; on Windows WinMM is exclusive-open, so co-existence there cannot
  read buttons while the DAW holds Push.
- **Wails v3** for the UI when a UI is needed. Note it depends on `webkit2gtk`
  on Linux — the one place the stack is not truly standalone. Fyne/Gio are the
  fallback if that becomes unacceptable.
- **Rust + `nusb`** was considered and is genuinely better engineering (pure
  Rust, no cgo, no LGPL). Rejected only because it forfeits `core/` reuse.
  Revisit if this becomes a distributed product.

## Two operating modes

- **Co-existence** — claim interface 0 only. DAW keeps Push's audio + MIDI.
  Zero extra software on all three OSes. No MIDI remapping. **Ships first.**
- **Full ownership** — also claim MIDI. Enables remapping, needs a virtual MIDI
  port to reach the DAW. macOS: CoreMIDI virtual source (or the built-in IAC
  Driver). Linux: ALSA seq, reuse `core/alsaseq`. **Windows: no built-in
  answer** — this is the project's hardest constraint (§6.2).

## Known constraints

- **Screen exclusivity (§4.1) — measured 2026-08-09.** With Live running and
  Push as its control surface, claiming interface 0 fails with
  `LIBUSB_ERROR_ACCESS` ("libusb: bad access [code -3]"), cleanly at claim time
  before any write. Everything else survives: USB enumeration, all 3 MIDI ports,
  and 16×16 audio. The claim releases as soon as Live quits — no replug. Handle
  this error explicitly in any display code: report "Live owns the display" and
  degrade, don't crash.
- **Windows virtual MIDI (§6.2):** the one requirement with no clean
  cross-platform answer. Options are Windows MIDI Services (recent Win11 only —
  verify the build floor, don't trust recalled version numbers), teVirtualMIDI
  (commercial driver), or shipping co-existence mode only.
- **Push 2 works from the same binary** (§10, measured 2026-08-16). Display,
  pads and LEDs are identical to Push 3; only the button CC table and MPE
  behaviour differ. Its button map is still largely unswept — `CC 111` is known
  to exist and be absent from the Push 3 map.
- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, undocumented,
  absent from Push 2's spec. Purpose unknown; enumerate freely, never send it
  invented payloads.
- **Endpoint `0x81` IN** on the display interface is unused by the write path.
  Possibly status/ack. Never read from so far.
- **Push 3 audio reports 16 in / 16 out** @ 44.1kHz — more than its analog I/O
  count, probably internal routing buses. Not investigated.
