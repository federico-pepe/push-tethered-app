# MIDI input

**Status:** confirmed on tethered hardware  
**Last verified:** 2026-08-20 (Push 2 + Push 3 map complete; User Mode routing confirmed)  
**Authoritative code:** [internal/midi/midi.go](../../internal/midi/midi.go), `core/push3`, `internal/pushmap`

Push's control-surface MIDI is read through the **OS MIDI API**, never libusb.
Rationale: [architecture/stack-and-layout.md](../architecture/stack-and-layout.md).

## Ports

Push 3 exposes three MIDI input ports via the OS (interface 5 = MIDIStreaming):

| Port | Role (confirmed 2026-08-20) |
|---|---|
| **Live Port** | Buttons/encoders/touch always; **pads only while User Mode is off** |
| User Port | Buttons/encoders/touch duplicate here always; **pads only while User Mode is on** |
| External Port | Keepalive only in normal use |

Push 2 has Live Port and User Port only.

**No host handshake** — Push emits MIDI as soon as it is connected.

### User Mode's effect on routing

Confirmed on Push 3 hardware, 2026-08-20 (`tools/midimon.swift` on all three
ports simultaneously, User button = CC 59, `core/push3/buttons.go:104`):

- **Buttons/encoders/touch (CC messages) always duplicate to both Live Port
  and User Port**, identical bytes, regardless of User Mode state. This does
  *not* change with the mode toggle.
- **Pad messages (Note On/Off, PolyAT) are exclusively routed**: Live Port
  only while User Mode is off, User Port only while User Mode is on. Never
  both, never neither. This is a real, device-level cutoff — with Push not
  selected in Live's generic MIDI-input prefs, Live receives zero pad bytes
  while User Mode is engaged, confirmed both by the port trace and by
  watching Live's UI directly.
- **The display claim is unaffected either way** — `pushapp` keeps interface
  0 through Live launching and through User Mode toggling; screen briefly
  blanks on Live's launch, then returns to `pushapp`'s own UI.
- **Live's outbound LED SysEx (`38 0D…`/`38 18…`) never stops**, User Mode or
  not — Live keeps sending it regardless of the device's mode.
- **User Mode enter/exit is announced unsolicited**, on both Live Port and
  User Port simultaneously, right at the toggle: `F0 00 21 1D 01 01 0A 01
  F7` (enter), `F0 00 21 1D 01 01 0A 00 F7` (exit). Usable as a detection
  signal — see [live-coexistence.md's C2](../../plans/2026-08-19-live-coexistence.md).
- **Pad LED *output* is exclusively routed too, mirroring input — confirmed
  2026-08-20.** `tools/ledtest.swift`'s palette sweep sent to the **Live
  Port** output cable renders nothing while User Mode is on; the identical
  sweep sent to the **User Port** output cable renders correctly while User
  Mode is on. (Earlier framing on this page called the User Mode grid a
  "local firmware override on top of Live's still-live colour stream" —
  that was based on Live Port writes only and is superseded by this: it
  isn't an override masking the grid, it's that Live Port stops being the
  live output cable the moment User Mode engages, same as it stops being
  the live input cable.) This means colours snapping back to Live's the
  instant User Mode exits (still true) is because Live Port becomes live
  again, not because Live's writes were ever being blocked.

This means User Mode is a genuine, working **full** contention workaround,
not just a pad-input one: engage it and a host gets exclusive pad input *and*
exclusive pad LED output, both routed away from Live at the device level —
not a routing suggestion Live could still be listening past. **The catch: a
host must target the matching output cable for its mode** — writing pad LEDs
to Live Port while in User Mode silently goes nowhere.

**Correction, confirmed 2026-08-20: `internal/midi` already handles this,
no code change needed.** `OpenRef` pairs each `PortRef`'s input cable with
its own same-role output cable (`ports.go`'s pairing logic, exact-name match
on macOS/Linux, positional on Windows) — it does not hardcode Live. Running
`pushapp -midi-in "Ableton Push 3 User Port"` while User Mode is engaged and
Live is running was tested end-to-end: pad presses arrive via User Port and
`SetPad` writes land on User Port's paired output cable automatically,
rendering `pushapp`'s own colours live, with Live running. What *is* still
Live-hardcoded: `Open()`'s zero-arg auto-detect (guesses Live Port by
design, no selection given) and `internal/identify.FlashLEDs`'s use of the
unpaired `OpenOutCable` — see [led-output.md](led-output.md). Neither is the
general path a caller with an explicit `PortRef`/name goes through. It does
not touch the display claim or button routing either way.

Windows names ports differently from CoreMIDI/ALSA — see
[platform/windows.md](../platform/windows.md).

## MPE

**Resolved, 2026-08-25: MPE on/off is a persistent setting in Push 3's own
onboard settings menu** (an Aftertouch mode: Polyphonic Aftertouch vs.
MPE) — a device-level configuration choice, not something Live negotiates
over the wire, not gated by any handshake, and not tied to Live's presence
at all. Confirmed both ways on real hardware, same session: with the
device set to Polyphonic Aftertouch, pads sat on channel 1 through every
condition tried (Live closed, Live open and actively holding the display
as a real control surface — see below); switched to MPE on the device
itself, pads immediately round-robined across member channels 2-16 with
real, continuous `slide`/`bend`/`pressure` data — reproduced again with
Live fully quit, `pushapp -module monitor` alone driving the display in
plain controller mode, no Live involved at any point. **So "assume MPE is
always on" (the original claim this section carried) and "MPE is
sometimes on, trigger unknown" (this section's own correction, earlier
the same day) were both wrong in the same way — chasing a protocol-layer
explanation for what turned out to be a simple hardware setting.**

**What this retires:** every protocol-layer hypothesis chased earlier the
same day — the MPE Configuration Message (RPN 6) not working, the
recurring vendor SysEx on Live Port (`docs/protocol/live-handshake.md`),
the standalone-mode Unix-socket IPC channels, "Live running as control
surface" as a trigger. None of them were the answer; all were real
findings about other things (the SysEx traffic and the IPC sockets are
still genuinely unexplained, just no longer suspected of gating MPE).

**Practical consequence, unchanged: do not assume either state.** The
decoder handles pads on channel 1 and MPE member channels 2-16 alike, so
a module must not assume one or the other — a user's own Push could be
configured either way, and nothing in the wire protocol announces which.
There is no way to query the device's current Aftertouch-mode setting
over MIDI, so a module has to infer it live, per hold, from the pad's own
channel (1 = Polyphonic Aftertouch/Push 2, 2-16 = MPE) —
`modules/padpointer`'s crosshair page does exactly this: MPE gets real
sub-pad `slide`/`bend` positioning, Polyphonic Aftertouch (or Push 2)
falls back to the coarse per-cell behavior every page always had. A
module that wants richer per-note data still has to handle the
Polyphonic-Aftertouch case gracefully, since it's a real, user-chosen
device state, not a fallback for an edge case.

### `slide`/`bend` behavior, confirmed live 2026-08-25 (once MPE is on)

Building `modules/padpointer`'s crosshair page against real MPE data
surfaced two non-obvious facts, both load-bearing for anything mapping
these to screen position:

- **`slide` (CC 74) is a *per-pad local* reading, not a grid-wide
  position.** Each pad's own sensor spans roughly its own 0-127 range
  across just that pad's height — sliding a finger down within one pad
  drives the value to *that pad's own* minimum right at the boundary with
  the pad below, which then starts back at *its own* maximum. A module
  must map it against the currently-held pad's own cell, not treat it as
  an absolute position across the whole strip.
- **`bend` (pitch bend) does not swing anywhere near its full 14-bit
  range (0-16383) during a normal within-pad gesture** — confirmed live
  that a real edge-to-edge slide only reaches a small slice of the
  theoretical range. Code that assumes the full range (e.g. splitting
  8192 either side of center) will make real gestures register as a tiny
  fraction of the intended movement. `padpointer` handles this by
  auto-calibrating against the actual min/max bend values observed live
  (`Module.bendMin`/`bendMax`, module-lifetime, monotonically widening)
  rather than a fixed constant — "slide fully right" is defined as "the
  most-right this specific Push has actually reported," which converges
  to true edge-to-edge with use instead of needing a magic number guessed
  from one capture.

Not measured: the exact menu path on Push's own screen (a future doc
update, or MANUAL.md, should name it precisely — "Push's settings" is
what confirmed this, not yet the exact label/location); whether this
setting is readable or settable over MIDI/SysEx from the host side, which
would let a module or `pushapp` flag/adapt to the device's current mode
instead of silently guessing; the actual numeric bounds of `bend`'s
practical range (auto-calibration sidesteps needing this, but a captured
number would still be a useful protocol fact).

## Decode order

1. **Filter Active Sensing** (`0xFE`, ~37/sec, over half of all traffic).
   Test for system realtime (`0xF8`–`0xFF`) **before** masking with `0xF0`,
   or `0xFE` decodes as SysEx.
2. **Decode channel first, then CC.** CC 71–79 are the nine encoders, but CC
   71/74 are also MPE timbre controllers — the channel disambiguates.

## Pads

- 8×8 grid, notes **36** (bottom-left) to **99** (top-right)
- Push 2: pads on channel 1 (no MPE)
- Push 3: MPE sometimes on, sometimes not — see above; do not assume either

**Channel Pressure (`0xD0`) works on channel 1, MPE or not.** Confirmed
2026-08-25 on real Push 3 hardware: a held pad sends continuous Channel
Pressure, ramping smoothly with how hard it's pressed (not positional — a
controlled test pressed harder without moving and the value tracked force,
not location). This was previously only decoded on MPE member channels
(2-16, `Expression{Kind:"pressure"}`); channel 1 messages were silently
dropped. Fixed in `internal/midi/midi.go`'s `DecodeFor` — now decoded on
channel 1 too. With MPE off there is no per-note channel to attribute a
reading to when multiple pads are held at once; a module has to pick its own
attribution rule (e.g. `modules/padpointer` and `modules/paddebug` both
attribute it to whichever pad was pressed most recently).

Per-note `slide` (CC 74) and `bend` (pitch bend) remain MPE-only — CC 74 on
channel 1 is claimed by Encoder 4 (see Decode order below), so there is no
unambiguous channel-1 equivalent for them the way there is for pressure.

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

- External Port role (User Port role is now confirmed, see above)
- Push 2 arrow down/right CCs (expected 46/47/44/45)

See [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md).
