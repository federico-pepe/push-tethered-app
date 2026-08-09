# push-tethered-app

Cross-platform desktop app to own an **Ableton Push 2 / Push 3 in tethered
(controller) mode** — display, pads, buttons, encoders, LEDs — as a fully
configurable MIDI controller, independent of any DAW.

> **Status: pre-alpha.** No app yet — but **the display protocol is confirmed
> working on real hardware.** `cmd/frametest` renders to a tethered Push 3's
> screen at 30fps using the existing `core/` widget toolkit. See
> [docs/feasibility.md](docs/feasibility.md) for the full writeup (§8 = measured
> results).

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

- **MPE is on by default** — pad note-ons rotate across channels 2–16, channel 1
  is the control surface. Per-note pressure, CC 74 slide and pitch bend arrive on
  each note's member channel.
- **Decode channel first, then CC.** CC 71–79 are the encoders (Push 2's map) but
  CC 71/74 are also MPE timbre controllers — the numbers collide, the channel
  disambiguates.
- Pads: 8×8, notes **36 bottom-left → 99 top-right**.
- Encoders: relative, `1` = +1 click / `127` = −1; encoder 1 = CC 71. Touch =
  Note On ch1 note 0–10. Touch strip touch = ch1 note 12.
- Buttons: CC, 127 press / 0 release. CC 104–107 above the screen, CC 20–22 below.
- **Filter Active Sensing** — `0xFE` at ~37/sec, over half of all traffic.

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
cmd/probe/       USB descriptor dump — interfaces, endpoints, altsettings
cmd/frametest/   Minimal "light the screen" test: one frame to bulk OUT
internal/        Shared device/transport code (grows from the above)
docs/            feasibility.md — full writeup, blockers, stack rationale
```

## Related

- [`ableton-push-hack`](https://github.com/federico-pepe/ableton-push-hack) —
  standalone Push 3 hacks; source of the `core/` module reused here.
- [`Ableton/push-interface`](https://github.com/Ableton/push-interface) —
  official Push 2 display + MIDI specification.
- [`ffont/push2-python`](https://github.com/ffont/push2-python) — working pyusb
  reference implementation for Push 2.
