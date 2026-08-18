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
| Value | Palette index 0–127, same mechanism as pad LEDs — **not** a brightness scale |
| Off | value `0` |

**Confirmed 2026-08-18** (screen-top button, CC 102, on real Push 3
hardware): sweeping the CC value from 0 to 127 in ascending steps produced a
non-monotonic sequence of distinct hues (white, orange, yellow, blue, green,
gray, faint yellow, faint blue, faint pink, bright red) rather than a smooth
brightness ramp on a fixed colour. That is inconsistent with "one white LED
whose brightness tracks the CC value" and consistent instead with the CC
value indexing the same 128-entry palette pad LEDs use — non-monotonic hue
jumps are expected from an unsorted palette, not from a brightness scale.
This overturns the previous (unverified) claim that button LEDs are
white-only and ignore colour — see `tools/ledbrightness.swift`, the probe
used for this measurement.

Only the screen-adjacent buttons (CC 20–27, 102–109 — the ones Live colours
to match track/clip colour) were tested. Other button classes (e.g. round
transport buttons) are unconfirmed and may behave differently.

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
tools/ledtest.swift        # macOS LED sweep (Swift, not part of build)
tools/ledbrightness.swift  # macOS single-button CC-value sweep (Swift, not part of build)
```

## LED contention with Live

`pushapp` and Live cannot both drive Push's pad LEDs safely. Screen
exclusivity means only one of them holds the display interface at a time, but
co-existence mode leaves Push's MIDI interface bound to the OS driver
regardless of which process owns the display — so whichever launched second
still sends pad-LED MIDI, and both end up driving the same physical LEDs at
once (confirmed 2026-08-17: `pushapp` launched first keeps the display, but
its pad-mirror grid started reflecting Live's Session View colouring). There
is no arbitration between the two. **Guidance: don't run `pushapp` while Live
is open.**

## Open questions

- Whether other button classes (round transport buttons, etc.) also use a
  palette index or are genuinely brightness-only/monochrome — only the
  screen-adjacent buttons have been measured.
- `internal/host`/`internal/midi`'s `SetButton(cc, brightness byte)` naming
  and doc comment describe a brightness scale; given the finding above, that
  API should be revisited (naming and/or behaviour) so callers don't reach
  for a brightness ramp expecting linear output. Not fixed in this pass — the
  measurement came first.

## Related

- [midi-input.md](midi-input.md) — input decoding
- [hardware-reference.md](../hardware-reference.md) — upstream palette link
