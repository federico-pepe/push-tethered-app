# LED output

**Status:** confirmed on tethered hardware  
**Last verified:** 2026-08-20  
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

**Which output cable carries pad LED writes depends on User Mode state —
confirmed 2026-08-20 on Push 3 hardware.** The original 2026-08-19 Push 2
measurement ("only Live Port carries pad LED writes, User Port lights
nothing") was taken outside User Mode and is a special case of a more
general rule: **Live Port renders pad LED writes only while User Mode is
off; User Port renders them only while User Mode is on** — the same
exclusive routing already confirmed for pad *input*
([midi-input.md](midi-input.md#user-modes-effect-on-routing)). Writing to
the wrong cable for the current mode silently lights nothing, no error.

`internal/midi.OpenOutCable` (used by `internal/identify.FlashLEDs`)
currently always targets the cable `internal/midi.PortRef.IsLive` reports
true — correct for every existing caller today since none of them are
User-Mode-aware yet, but this is the seam that would need to switch cables
if a module ever wants to paint pads while coexisting with Live via User
Mode. Not yet re-confirmed on Push 3's third (External) port.

## Button LEDs

| Property | Value |
|---|---|
| Message | CC, channel 1 |
| Value | Palette index 0–127, same mechanism as pad LEDs — **not** a brightness scale |
| Off | value `0` |

**Confirmed 2026-08-18** on real Push 3 hardware, both a screen-top button
(CC 102) and a round transport button (Play, CC 85): sweeping the CC value
from 0 to 127 in ascending steps produced a non-monotonic sequence of
distinct hues (white, orange, yellow, blue, green, gray, faint yellow, faint
blue, faint pink, bright red) rather than a smooth brightness ramp on a
fixed colour, on both button classes. That is inconsistent with "one white
LED whose brightness tracks the CC value" and consistent instead with the CC
value indexing the same 128-entry palette pad LEDs use — non-monotonic hue
jumps are expected from an unsorted palette, not from a brightness scale.
This overturns the previous (unverified) claim that button LEDs are
white-only and ignore colour — see `tools/ledbrightness.swift`, the probe
used for this measurement. Applies to every button with an LED: they all use
the palette, not a brightness scale.

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
still sends pad-LED MIDI, and both end up driving the same *physical* pad
LEDs at once (confirmed 2026-08-17: `pushapp` launched first keeps the
display, but the **physical pads** started reflecting Live's Session View
colouring — not a `pushapp`-rendered mirror; `modules/monitor`'s on-screen
`padsLit` carries no colour information and cannot have produced this).

The contention is **functional, not just cosmetic**: with `pushapp` running
second, Live also keeps the pad *MIDI* — both its colouring and pressing a
pad still drives Live, not `pushapp`. There is no arbitration between the two
hosts, and no direct signal for detecting the contention (see
[live-coexistence.md Part C](../../plans/2026-08-19-live-coexistence.md) —
process presence is the strongest available proxy).

**Confirmed 2026-08-20: no inbound echo of any kind.** Neither Live's
Session View colour changes nor our own `SetPad`/`SetButton` writes produce
any inbound NoteOn/CC on any port — `midimon` on all three input ports shows
nothing but SysEx and Active Sensing in both directions of that test. There
is a recurring SysEx pattern on Live Port whenever Live runs, independent of
any press or write, that looks handshake-shaped but is not yet decoded — see
[live-handshake.md](live-handshake.md). It is not currently a usable
contention signal.

**No re-assert step needed on a clean Live quit.** Confirmed 2026-08-20:
pads went off on their own when Live quit, not stuck holding Live's last
colours — unlike the User Mode case below, where Live's colours snap back
instantly because Live never stopped sending them. Launch order
(`pushapp`-first vs. Live-first) makes no difference to pad-LED stability
either way — both orders measured steady, not flickering, with both LED
policies. A user-facing toggle is enough; nothing here forces a default-on
back-off.

**Confirmed workaround, both halves: Push's own User Mode — updated
2026-08-20.** See
[midi-input.md](midi-input.md#user-modes-effect-on-routing) — engaging User
Mode on the device cuts Live off from pad MIDI entirely (device-level
routing, not just a suggestion Live could still listen past), while leaving
the display claim and button routing untouched. This was originally thought
to be pad-input-only, with Live's LED writes still flowing to the pads
underneath a local "User Mode override." That framing is now superseded:
**pad LED output is exclusively routed by the same mode toggle** — Live
Port stops rendering and User Port starts rendering the moment User Mode
engages, so a host that targets User Port can paint its own pad colours
while fully coexisting with Live, not just read pad presses. **`internal/midi`
already does this correctly, confirmed 2026-08-20, no code change needed** —
see [midi-input.md](midi-input.md#user-modes-effect-on-routing) for the
detail: `OpenRef` pairs each `PortRef` with its own same-role output cable,
so `pushapp -midi-in "Ableton Push 3 User Port"` while User Mode is engaged
already writes LEDs to the right cable, tested end-to-end (`monitor`'s own
white and a dedicated red palette-index-1 write, both rendered correctly,
positioned correctly, with Live running the whole time). **Guidance: don't
run `pushapp` while Live is open**, unless User Mode is engaged for the
duration and `pushapp` is pointed at the User Port cable (`-midi-in
"... User Port"`, or the equivalent `PortRef` in `pushapp-ui` once it
exposes per-unit port selection).

## Open questions

- `internal/host/procmod`'s wire JSON field (`{"brightness": ...}`) still says
  "brightness" — every Go-level `SetButton` (module, midi, host) is now named
  `value`. Renaming the wire field is a breaking protocol change for any
  existing process-loaded module and needs its own decision (alias old+new
  field? version bump?) before touching it.

## Related

- [midi-input.md](midi-input.md) — input decoding
- [live-handshake.md](live-handshake.md) — the recurring SysEx pattern seen
  whenever Live is running
- [hardware-reference.md](../hardware-reference.md) — upstream palette link
