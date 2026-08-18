# LED output

**Status:** confirmed on tethered hardware  
**Last verified:** 2026-08-09  
**Authoritative code:** [internal/midi/midi.go](../../internal/midi/midi.go), `core/push3/colors.go`

LEDs are driven over **OS MIDI** (CoreMIDI on macOS). No handshake needed.
Works without claiming USB interface 5.

## Pad LEDs

| Property | Value |
|---|---|
| Message | Note On, channel 1 |
| Notes | 36–99 (8×8 grid) |
| Velocity | Palette index from `core/push3/colors.go` |
| Off | velocity `0` |

Pad geometry confirmed from output: note 36 = bottom-left, ascending to 99
top-right.

## Button LEDs

| Property | Value |
|---|---|
| Message | CC, channel 1 |
| Value | Brightness 0–127 |
| Colour | White LEDs ignore colour index |

## Palette

128 palette entries, indices 0–127. Full hardware table (SysEx-verified on
Push 3 firmware):

[ableton-push-hack/docs/push3-led-colors.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-led-colors.md)

`core/push3/colors.go` `NamedColors` map matches that table. Use palette
**indices** for pad LEDs; screen draw colours are separate RGBA values in module
draw ops.

Common values:

| Name | Index | Notes |
|---|---|---|
| off | 0 | black |
| green | 11 | `#34C216` |
| white (pad) | 120 | `#FFFFFF` |

CC button "white" uses index 122 — different from pad white.

## Exit behaviour

**Always clear LEDs on every exit path, including SIGINT.** Leaving the device
lit makes the next run ambiguous. The host clears LEDs on shutdown; modules
should release any notes they hold in `Close`.

## Probes

```bash
tools/ledtest.swift    # macOS LED sweep (Swift, not part of build)
```

## Open questions

- Button-LED brightness fidelity vs sent values
- LED contention when Live and `pushapp` both drive pads (see
  [open-questions.md](../open-questions.md))

## Related

- [midi-input.md](midi-input.md) — input decoding
- [hardware-reference.md](../hardware-reference.md) — upstream palette link
