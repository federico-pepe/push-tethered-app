# LED output

**Status:** confirmed on tethered hardware
**Last verified:** 2026-08-20
**Authoritative code:** [internal/midi/midi.go](../../internal/midi/midi.go), `core/push3/colors.go`

The host drives LEDs over **OS MIDI** (CoreMIDI on macOS). No handshake is
necessary. This works without a claim on USB interface 5.

## Pad LEDs

| Property | Value |
|---|---|
| Message | Note On, channel 1 |
| Notes | 36–99 (8×8 grid) |
| Velocity | Palette index from `core/push3/colors.go` |
| Off | velocity `0` |

Pad geometry is confirmed from device output. Note 36 is the bottom-left
pad. Note 99 is the top-right pad. Notes ascend from bottom-left to
top-right.

**The output cable that carries pad LED writes depends on the state of
User Mode.** This is confirmed on 2026-08-20 on Push 3 hardware.

The original 2026-08-19 Push 2 measurement found that only Live Port
carries pad LED writes and User Port lights nothing. That measurement was
taken outside User Mode. It is a special case of a more general rule.

**Live Port renders pad LED writes only while User Mode is off. User Port
renders pad LED writes only while User Mode is on.** This matches the
exclusive routing already confirmed for pad *input*
([midi-input.md](midi-input.md#user-modes-effect-on-routing)).

Writing to the wrong cable for the current mode lights nothing. The device
gives no error.

`internal/midi.OpenOutCable` (used by `internal/identify.FlashLEDs`)
always targets the cable where `internal/midi.PortRef.IsLive` reports
true. This is correct for every existing caller today, because none of
them are User-Mode-aware yet. This is the seam that a module must change
if it wants to paint pads while it coexists with Live through User Mode.

This behavior is not yet re-confirmed on Push 3's third (External) port.

## Button LEDs

| Property | Value |
|---|---|
| Message | CC, channel 1 |
| Value | Palette index 0–127, same mechanism as pad LEDs — **not** a brightness scale |
| Off | value `0` |

**Confirmed on 2026-08-18** on real Push 3 hardware. The test used a
screen-top button (CC 102) and a round transport button (Play, CC 85).
Sweeping the CC value from 0 to 127 in ascending steps produced a
non-monotonic sequence of distinct hues, on both button classes: white,
orange, yellow, blue, green, gray, faint yellow, faint blue, faint pink,
and bright red. This was not a smooth brightness ramp on a fixed color.

This result does not match a theory of one white LED whose brightness
tracks the CC value. It matches instead a theory where the CC value
indexes the same 128-entry palette that pad LEDs use. Non-monotonic hue
jumps are expected from an unsorted palette, not from a brightness scale.

This finding overturns the previous, unverified claim that button LEDs
are white-only and ignore color (see `tools/ledbrightness.swift`, the
probe used for this measurement). Every button with an LED uses the
palette, not a brightness scale.

## Palette

128 palette entries, indices 0–127. Full hardware table (SysEx-verified on
Push 3 firmware):

[ableton-push-hack/docs/push3-led-colors.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-led-colors.md)

The `core/push3/colors.go` `NamedColors` map matches that table. Use
palette **indices** for pad LEDs. Screen draw colors are separate RGBA
values in module draw operations.

The same file's `Palette`/`ColorForIndex` function gives the RGBA value
that a palette index resolves to. A module can use this to preview an LED
color on screen. `ColorForIndex` rounds a raw 0-127 index down to the
nearest of the 90 named entries, because not all 128 raw indices have a
name. See `modules/ui-text-demo` for a live example that drives both a
swatch and its name from one encoder.

Common values:

| Name | Index | Notes |
|---|---|---|
| off | 0 | black |
| green | 11 | `#34C216` |
| white (pad) | 120 | `#FFFFFF` |

The CC button "white" uses index 122. This differs from pad white.

## Exit behavior

**Always clear LEDs on every exit path, including SIGINT.** Leaving the
device lit makes the status of the next run unclear. The host clears LEDs
at shutdown. Modules must release any notes they hold in `Close`.

## Probes

```bash
tools/ledtest.swift        # macOS LED sweep (Swift, not part of build)
tools/ledbrightness.swift  # macOS single-button CC-value sweep (Swift, not part of build)
```

## LED contention with Live

`pushapp` and Live cannot safely drive Push's pad LEDs at the same time.
Screen exclusivity means that only one process holds the display interface
at a time. But co-existence mode leaves Push's MIDI interface bound to the
OS driver, regardless of which process owns the display.

As a result, whichever process launches second still sends pad-LED MIDI.
Both processes then drive the same physical pad LEDs at the same time.
This is confirmed on 2026-08-17: `pushapp` launched first and kept the
display, but the physical pads began to reflect Live's Session View
coloring. This was not a mirror rendered by `pushapp`. `modules/monitor`'s
on-screen `padsLit` value carries no color information and cannot have
produced this effect.

The contention is functional, not only cosmetic. When `pushapp` runs
second, Live also keeps the pad MIDI. Both Live's coloring and a pad press
still drive Live, not `pushapp`. There is no arbitration between the two
hosts, and there is no direct signal to detect the contention (see
[live-coexistence.md Part C](../../plans/2026-08-19-live-coexistence.md)).
Process presence is the strongest proxy available.

**Confirmed on 2026-08-20: there is no inbound echo of any kind.** Neither
Live's Session View color changes nor our own `SetPad`/`SetButton` writes
produce an inbound NoteOn or CC message on any port. `midimon` on all
three input ports shows nothing but SysEx and Active Sensing. This holds
in both directions of that test.

There is a recurring SysEx pattern on Live Port whenever Live runs. It is
independent of any press or write, and it looks handshake-shaped, but it
is not yet decoded (see [live-handshake.md](live-handshake.md)). It is not
currently a usable contention signal.

**No re-assert step is necessary on a clean Live quit.** Confirmed on
2026-08-20: pads turned off on their own when Live quit. They did not stay
stuck holding Live's last colors. This differs from the User Mode case
below, where Live's colors snap back instantly because Live never stopped
sending them.

Launch order (`pushapp` first, or Live first) makes no difference to
pad-LED stability. Both orders measured steady, with no flickering, under
both LED policies. A user-facing toggle is enough. Nothing here forces a
default-on back-off.

**Confirmed workaround for both halves of the problem: Push's own User
Mode.** Updated 2026-08-20. See
[midi-input.md](midi-input.md#user-modes-effect-on-routing).

Engaging User Mode on the device cuts Live off from pad MIDI entirely.
This is device-level routing, not just a suggestion that Live can still
listen past. It leaves the display claim and button routing untouched.

This workaround was originally thought to cover pad input only. Under
that framing, Live's LED writes still flowed to the pads underneath a
local "User Mode override." That earlier framing is now superseded.
**Pad LED output is
exclusively routed by the same mode toggle.** Live Port stops rendering
and User Port starts rendering the moment User Mode engages. A host that
targets User Port can paint its own pad colors while it fully coexists
with Live, not only read pad presses.

**`internal/midi` already does this correctly.** Confirmed 2026-08-20, no
code change is necessary. See
[midi-input.md](midi-input.md#user-modes-effect-on-routing) for the
detail: `OpenRef` pairs each `PortRef` with its own same-role output
cable. So `pushapp -midi-in "Ableton Push 3 User Port"`, while User Mode
is engaged, already writes LEDs to the right cable. This was tested
end-to-end: `monitor`'s own white write and a dedicated red
palette-index-1 write both rendered correctly, in the correct position,
with Live running the whole time.

**Guidance: do not run `pushapp` while Live is open**, unless User Mode is
engaged for the duration and `pushapp` targets the User Port cable
(`-midi-in "... User Port"`, or the equivalent `PortRef` in `pushapp-ui`
once it exposes per-unit port selection).

## Open questions

- `internal/host/procmod`'s wire JSON field (`{"brightness": ...}`) still
  uses the name "brightness". Every Go-level `SetButton` function
  (module, midi, host) is now named `value`. Renaming the wire field is a
  breaking protocol change for any existing process-loaded module. It
  needs its own decision, for example an alias of the old and new field,
  or a version bump, before anyone changes it.

## Related

- [midi-input.md](midi-input.md) — input decoding
- [live-handshake.md](live-handshake.md) — the recurring SysEx pattern seen
  whenever Live is running
- [hardware-reference.md](../hardware-reference.md) — upstream palette link
</content>
