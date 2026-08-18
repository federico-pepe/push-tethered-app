# MIDI input

**Status:** confirmed on tethered hardware  
**Last verified:** 2026-08-16 (Push 2 + Push 3 map complete)  
**Authoritative code:** [internal/midi/midi.go](../../internal/midi/midi.go), `core/push3`, `internal/pushmap`

Push's control-surface MIDI is read through the **OS MIDI API**, never libusb.
Rationale: [architecture/stack-and-layout.md](../architecture/stack-and-layout.md).

## Ports

Push 3 exposes three MIDI input ports via the OS (interface 5 = MIDIStreaming):

| Port | Role (observed) |
|---|---|
| **Live Port** | All control-surface traffic — pads, encoders, buttons, touch |
| User Port | Keepalive only in normal use |
| External Port | Keepalive only in normal use |

Push 2 has Live Port and User Port only.

**No host handshake** — Push emits MIDI as soon as it is connected.

Windows names ports differently from CoreMIDI/ALSA — see
[platform/windows.md](../platform/windows.md).

## MPE

MPE is on by default on Push 3 — **but not always consistently**:

- Pad note-ons have been observed on channels 2–16 (per-note channels)
- Separately, all pads on channel 1 have been observed
- No identified trigger for the switch

**Handle both.** Channel 1 is always the control surface. Per-note pressure,
CC 74 slide, and pitch bend arrive on the note's own channel.

Still unmeasured: whether MPE can be disabled via SysEx.

## Decode order

1. **Filter Active Sensing** (`0xFE`, ~37/sec, over half of all traffic).
   Test for system realtime (`0xF8`–`0xFF`) **before** masking with `0xF0`,
   or `0xFE` decodes as SysEx.
2. **Decode channel first, then CC.** CC 71–79 are the nine encoders, but CC
   71/74 are also MPE timbre controllers — the channel disambiguates.

## Pads

- 8×8 grid, notes **36** (bottom-left) to **99** (top-right)
- Push 2: pads on channel 1 (no MPE)
- Push 3: usually MPE (see above)

## Encoders

Nine relative encoders + jog wheel:

| Control | CC | Encoding |
|---|---|---|
| Encoder 1–8 | 71–78 | Relative two's-complement |
| Jog wheel | 70 | Relative (`push3.IsEncoderCC` covers it) |
| Volume | 79 | Relative |
| Tempo | 14 | Relative |

Direction: **`1` = CW, `127` = CCW**. Decode with `push3.DecodeRel`.

**Encoders accelerate** — fast turns produce deltas up to ±11. Never assume
one message = one click; always use the decoded signed value.

## Touch sensors

| Sensor | Note |
|---|---|
| Encoders 1–8 | 0–7 |
| Volume wheel | 8 |
| *(note 9)* | unused on Push 3; Swing on Push 2 |
| Tempo wheel | 10 |
| Jog wheel | 11 |
| Touch strip | 12 |
| D-Pad center | 13 |

Note On vel 127 = contact, 0 = release. Authoritative: `core/push3`.

## Buttons

CC messages, 127 = press, 0 = release. CC 104–107 above the screen; CC 20–22
below (full map in upstream [push3-button-map.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-button-map.md)).

**Button map is complete:** Push 3 — 87/87 CC + 13/13 touch; Push 2 — 75/80
CC + 12/14 touch; zero unknowns on either device.

Two CCs differ per device — always use `pushmap.ButtonNameFor` /
`IsRelativeEncoderCCFor` with a known device:

| CC | Push 2 | Push 3 |
|---|---|---|
| 15 | Swing | Tempo |
| 111 | Browse | Volume |

## Probes

```bash
go run ./cmd/mapcheck     # cross-reference captures against the map
tools/midimon.swift       # macOS MIDI monitor (Swift, not part of build)
```

Full map: [hardware-reference.md](../hardware-reference.md).

## Open questions

- What triggers MPE on/off between sessions
- User Port / External Port roles
- Push 2 arrow down/right CCs (expected 46/47/44/45)

See [open-questions.md](../open-questions.md).
