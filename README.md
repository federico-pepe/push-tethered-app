# Push Tethered App

Cross-platform desktop app that owns an **Ableton Push 2 / Push 3 in tethered
(controller) mode** — display, pads, buttons, encoders, LEDs — and turns it into
a platform you can write your own tools for, independent of any DAW.

> **Status: pre-alpha, but running.** `cmd/pushapp` is a working vertical slice:
> one Go binary that holds the screen at 30fps, reads the control surface, and
> lights the pads you press. **Confirmed on both Push 2 and Push 3 hardware from
> the same unmodified binary.**
>
> The product shape was decided 2026-08-17 — a **module host** — and the module
> contract is being built now against that slice. See
> [plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md),
> [docs/archive/feasibility.md](docs/archive/feasibility.md) (§8 = protocol
> measurements, §9 = the slice, §10 = Push 2) and
> [docs/open-questions.md](docs/open-questions.md) for what's still open.

```bash
go run ./cmd/pushapp                          # host + first module
go run ./cmd/pushapp -list                    # what modules are available
go run ./cmd/pushapp -capture demo.mp4        # ...and record the screen
go run ./cmd/midiouttest                      # prove MIDI reaches other apps
```

### Writing a module

```go
type Module interface {
    Meta() Meta            // id, name, whether it sends MIDI
    Init(h Host) error     // called on activation; h is the hardware
    Handle(ev Event)       // pads, buttons, encoders, touch, MPE
    Draw(f *Frame)         // append draw ops for one frame
    Close() error
}
```

A module never touches USB, never opens a MIDI port and never draws pixels — it
appends ops to a `Frame` and the host renders them with the shared `core/gfx`
widget toolkit. `Handle` and `Draw` are guaranteed never to run concurrently, so
module state needs no locks. `modules/monitor` is the reference, and
`internal/module/moduletest` provides a fake host so modules can be unit-tested
with no Push attached.

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

## What this is — a module host

Decided 2026-08-17: `pushapp` is a **host** that owns the hardware and runs
**modules**. A module is a small program — writable by anyone, with or without
the help of AI — that draws Push's screen and handles its pads, encoders and
buttons. The app ships a few modules to show what's possible; the point is that
you write your own.

No DAW is involved at any layer. A MIDI remapper is *a module*, not the product.

The full design, including the module contract and the phasing, is in
[plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md).

This replaces the earlier co-existence / full-ownership split. What survives of
it: Push's MIDI is always read through the OS API rather than libusb, and if
Ableton Live happens to be holding the display we degrade cleanly instead of
fighting for it.

### Reaching other software

Modules can send MIDI out — to a synth, a DAW, anything. The app does not create
a virtual port; it **owns a named output port**:

| | How | Setup for the user |
|---|---|---|
| macOS, Linux | creates the port itself | none |
| Windows | attaches to an existing port | install [loopMIDI](https://www.tobias-erichsen.de/software/loopmidi.html) (free) and create one |

Windows MM MIDI cannot create virtual ports at all, so this is a platform fact
rather than a missing feature. Verified working on macOS 2026-08-17.

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
cmd/midiouttest/  MIDI-out probe: create/attach a port, send, and receive back
internal/module/  the module ABI: Module, Host, Frame/Op, Event
internal/host/    runtime: registry, control API, event fan-out, frame loop
internal/display/ USB transport: claim interface 0, header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/midiout/ owns a named MIDI out port for modules (create or attach)
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
modules/monitor/  control-surface monitor; the reference module
modules/thru/     forwards pads/encoders/buttons out as MIDI
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
