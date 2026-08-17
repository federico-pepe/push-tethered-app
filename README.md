# Push Tethered App

Cross-platform desktop app to own an **Ableton Push 2 / Push 3 in tethered
(controller) mode** — display, pads, buttons, encoders, LEDs — as a fully
configurable MIDI controller, independent of any DAW.

> **Status: pre-alpha, but running.** `cmd/pushapp` is a working vertical slice:
> one Go binary that holds the screen at 30fps, reads the control surface, and
> lights the pads you press. **Confirmed on both Push 2 and Push 3 hardware from
> the same unmodified binary.** No configuration or remapping yet. See
> [docs/archive/feasibility.md](docs/archive/feasibility.md) (§8 = protocol
> measurements, §9 = the slice, §10 = Push 2) and
> [docs/open-questions.md](docs/open-questions.md) for what's still open.

```bash
go run ./cmd/pushapp                          # screen + input + LEDs
go run ./cmd/pushapp -capture demo.mp4        # ...and record the screen
```

## Why this can work

The hard part — driving Push's 960×160 screen — is already solved in the
sibling project [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack).
Its `core/` Go module contains a complete, device- and transport-agnostic Push
screen toolkit (`gfx`, `gfx/text`, `gfx/widgets`) plus the BGR565 codec, all
written against plain `image.NRGBA`. This project reuses it directly and swaps
the transport: shared-memory writer → USB bulk writer.

Push 3's display protocol appears **byte-identical to Push 2's public spec**
(`Ableton/push-interface`): bulk OUT ep `0x01`, 16-byte header
`FF CC AA 88` + 12 × `00`, 960×160 BGR565 LE, stride 1024 px, XOR `0xFFE7F3E7`.

## Verified so far

Measured on macOS (Apple Silicon), Push 3 in controller mode, Live not running —
**2026-08-09**:

- **VID `0x2982` / PID `0x1969`** (Push 2 is `0x1967`). Composite device
  (`bDeviceClass 239`, IAD), USB 2.0, 1 configuration, 7 interfaces.

  | # | Name | Class/Sub | Endpoints |
  |---|---|---|---|
  | 0 | **Display** | 255/255 vendor | **`0x01` OUT bulk 512**, `0x81` IN bulk 512 |
  | 1 | Audio | 1/1 control | `0x85` IN interrupt 6 |
  | 2 | Audio Out | 1/2 streaming | alt 1: `0x02` OUT isoc 624 |
  | 3 | Audio In | 1/2 streaming | alt 1: `0x82` IN isoc 624 |
  | 4 | MIDI | 1/1 control | — |
  | 5 | MIDI | 1/3 **MIDIStreaming** | `0x03` OUT bulk 512, `0x83` IN bulk 512 |
  | 6 | **xPort** | 255/255 vendor | `0x04` OUT bulk 512, `0x84` IN bulk 512 |

- **The display protocol works over USB in controller mode.** `cmd/frametest`
  claims interface 0, writes the 16-byte header + a single XOR-shaped
  327680-byte BGR565 frame to ep `0x01`, and the screen renders it correctly.
  - **A single frame is enough** — the standalone device's frame duplication is
    a quirk of Ableton's binary, not a hardware requirement.
  - **`core/display.ToBGR565` needs no tethered variant** — channel order is
    identical.
  - **The screen must be refreshed continuously.** One frame flashes and is then
    overwritten: with no host driving it, Push redraws its own "connect to a
    computer" idle screen. Holding the display means outrunning that renderer.
  - **30fps sustained cleanly:** 360 frames in 12.03s, 9.4 MB/s, zero write
    errors. 60fps (≈18.8 MB/s) is well within USB 2.0 high-speed budget.
- Interfaces 5 (MIDI) and 0 (display) use **the same endpoint addresses observed
  inside the standalone device** — so device MIDI can ride the same libusb handle.
- **macOS needs no driver, kext or elevated privileges** to claim interface 0.
- **Co-existence with Live is confirmed.** With Live running and Push as its
  control surface, claiming interface 0 fails cleanly with `LIBUSB_ERROR_ACCESS`
  — but USB enumeration, all 3 MIDI ports and 16×16 audio remain available. The
  claim releases the instant Live quits, no replug. The app loses the screen and
  nothing else.

- **The display is its own vendor-specific interface (0).** Claiming it with
  libusb leaves interfaces 1–5 bound to the OS class drivers, so a DAW keeps
  seeing Push's audio and MIDI normally. This makes *co-existence mode*
  (below) viable — the single most important architectural finding so far.
- **Audio is fully class-compliant.** macOS binds `AppleUSBAudioDevice` with
  `Ableton Push 3 Audio In`/`Out` streams, zero install.
- **3 MIDI ports**, named exactly as on the standalone device:
  `Ableton Push 3 Live Port`, `User Port`, `External Port`.
- **`xPort` (interface 6) is undocumented** — vendor-specific, 2 endpoints, not
  in Push 2's spec. Purpose unknown. ("x" plausibly = XMOS.)

### MIDI input

Captured via CoreMIDI with Live closed — Push emits **with no host handshake**,
all on `Live Port`:

- **MPE is on by default — but not always.** On 2026-08-09 pad note-ons rotated
  across channels 2–16; on 2026-08-16 the same setup put them on channel 1. The
  trigger is unidentified, so handle both (feasibility §9.5). Channel 1 is always
  the control surface; per-note pressure, CC 74 slide and pitch bend arrive on
  each note's member channel.
- **Decode channel first, then CC.** CC 71–79 are the encoders (Push 2's map) but
  CC 71/74 are also MPE timbre controllers — the numbers collide, the channel
  disambiguates.
- Pads: 8×8, notes **36 bottom-left → 99 top-right**.
- Encoders: relative, `1` = +1 click / `127` = −1; encoder 1 = CC 71. They
  **accelerate** — deltas up to ±11 on fast turns, so decode the signed value.
- Touch sensors: encoders 1–8 = notes **0–7**, volume wheel **8**, note 9 unused,
  tempo **10**, jog **11**, touch strip **12**, D-Pad center **13**. These
  correct `core/push3`, which is off by one for the encoders and volume wheel and
  omits the touch strip — see `internal/pushmap` and feasibility §8.8.
- Buttons: CC, 127 press / 0 release. CC 104–107 above the screen, CC 20–22 below.
- **Filter Active Sensing** — `0xFE` at ~37/sec, over half of all traffic.

### LED output

Driven over CoreMIDI (co-existence output path), 581 messages at ~8ms spacing,
no drops:

- **Pad LEDs:** Note On ch1, note 36–99, velocity = palette index from
  `core/push3/colors.go`, `0` = off. All 64 pads lit in one sweep.
- **Button LEDs:** CC ch1, value = brightness 0–127 (white LEDs ignore colour).
- A row-walk from note 36 travelled **bottom to top**, confirming note 36 =
  bottom-left from the output side — independently of the input-side measurement.
- No handshake required for output either.

**Display out, MIDI in and LED out all work simultaneously in co-existence mode
on macOS with zero additional software.** That is the v1 product surface, minus
remapping, demonstrated on hardware.

## Push 2 vs Push 3

Both confirmed working from one binary (§10). The display is **identical** —
interface 0, vendor-specific, bulk OUT `0x01`, same header, XOR and geometry —
as are the pad grid (notes 36–99) and the LED palette.

| | Push 2 | Push 3 |
|---|---|---|
| USB interfaces | 3 | 7 |
| MIDI endpoints | `0x02`/`0x82` | `0x03`/`0x83` |
| MIDI ports | 2 (Live, User) | 3 (+External) |
| Audio | none | class-compliant 16×16 |
| `xPort` | absent | present |
| MPE | no — pads on ch1 | usually |

Only the button CC table genuinely differs, so the device abstraction is small.

## Two operating modes

**Co-existence mode** — claim only interface 0. App draws the screen and
provides configuration; the DAW keeps Push's audio and MIDI natively. Works on
macOS, Windows and Linux with zero additional software. Cannot remap MIDI.

**Full-ownership mode** — claim the MIDI interface too. Remapping, custom
layouts, alternate modes. Requires a virtual MIDI port to re-emit to the DAW,
which has no built-in answer on Windows (see feasibility doc §6.2).

Co-existence mode ships first.

## Requirements

**Users:** none beyond the OS. Single binary.

**Development:**
- Go 1.25+
- libusb 1.0 (`brew install libusb` on macOS)
- A sibling checkout of `ableton-push-hack` for the `core/` module — see
  the `replace` directive in `go.mod`.

## Layout

```
cmd/pushapp/      the app: display + input + LEDs in one process
cmd/probe/        USB descriptor dump — read-only, never opens the device
cmd/frametest/    display-only probe: one frame, or a timed hold
cmd/mapcheck/     cross-references captures against the button map
internal/display/ USB transport: claim interface 0, header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
tools/            macOS-only Swift probes (midimon, ledtest)
docs/             archive/feasibility.md (frozen writeup) + open-questions.md
```

## Stack

- **Go**, single binary. Chosen so the `core/` screen toolkit is reused, not ported.
- **`gousb`** (cgo → libusb) for the display interface.
- **`gomidi` + `rtmididrv`** for OS MIDI. The driver vendors the RtMidi C++
  sources, so there is **no system package to install** — one dependency covers
  macOS, Linux and Windows.
- cgo means **no cross-compilation**: build on each target OS.
  `.github/workflows/build.yml` builds on real macOS/Linux/Windows runners —
  see CLAUDE.md's "Cross-platform builds" section for local per-OS setup
  (Linux needs `libusb-1.0-0-dev` + `libasound2-dev` + a udev rule; Windows
  needs mingw-w64 and is untested on real hardware so far).

## Related

- [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack) —
  standalone Push 3 hacks; source of the `core/` module reused here.
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) —
  official Push 2 display + MIDI specification.
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working pyusb
  reference implementation for Push 2.
