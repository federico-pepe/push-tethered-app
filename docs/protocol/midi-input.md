# MIDI input

**Status:** confirmed on tethered hardware
**Last verified:** 2026-08-20 (Push 2 + Push 3 map complete, User Mode routing confirmed)
**Authoritative code:** [internal/midi/midi.go](../../internal/midi/midi.go), `core/push3`, `internal/pushmap`

The host reads Push's control-surface MIDI through the **OS MIDI API**,
never through libusb. Rationale:
[architecture/stack-and-layout.md](../architecture/stack-and-layout.md).

## Ports

Push 3 exposes three MIDI input ports through the OS (interface 5 =
MIDIStreaming):

| Port | Role (confirmed 2026-08-20) |
|---|---|
| **Live Port** | Buttons/encoders/touch always. **Pads only while User Mode is off.** |
| User Port | Buttons/encoders/touch duplicate here always. **Pads only while User Mode is on.** |
| External Port | Keepalive only in normal use — carries no control-surface traffic. **Confirmed 2026-08-27: this is Push 3's physical MIDI DIN connector**, reachable as an ordinary MIDI in/out cable — see below. |

Push 2 has Live Port and User Port only.

**No host handshake is necessary.** Push emits MIDI as soon as the host
connects to it.

### User Mode's effect on routing

Confirmed on Push 3 hardware, 2026-08-20 (`tools/midimon.swift` on all
three ports at the same time, User button = CC 59,
`core/push3/buttons.go:104`):

- **Buttons, encoders, and touch (CC messages) always duplicate to both
  Live Port and User Port**, with identical bytes, regardless of the
  User Mode state. This does not change with the mode toggle.
- **Pad messages (Note On/Off, PolyAT) are exclusively routed:** Live
  Port only while User Mode is off, User Port only while User Mode is on.
  Never both, never neither. This is a real, device-level cutoff. With
  Push not selected in Live's generic MIDI-input preferences, Live
  receives zero pad bytes while User Mode is engaged. This is confirmed
  both by the port trace and by watching Live's UI directly.
- **The display claim is unaffected either way.** `pushapp` keeps
  interface 0 through Live launching and through User Mode toggling. The
  screen briefly blanks when Live launches, then returns to `pushapp`'s
  own UI.
- **Live's outbound LED SysEx (`38 0D…`/`38 18…`) never stops**, in User
  Mode or not. Live keeps sending it regardless of the device's mode.
- **User Mode enter and exit are announced unsolicited**, on both Live
  Port and User Port at the same time, right at the toggle: `F0 00 21 1D
  01 01 0A 01 F7` (enter), `F0 00 21 1D 01 01 0A 00 F7` (exit). This is
  usable as a detection signal — see
  [live-coexistence.md's C2](../../plans/2026-08-19-live-coexistence.md).
- **Pad LED *output* is exclusively routed too, and mirrors input.**
  Confirmed 2026-08-20. `tools/ledtest.swift`'s palette sweep, sent to
  the **Live Port** output cable, renders nothing while User Mode is on.
  The identical sweep, sent to the **User Port** output cable, renders
  correctly while User Mode is on. Earlier framing on this page called
  the User Mode grid a "local firmware override on top of Live's still-
  live color stream." That framing is superseded: it is not an override
  that masks the grid. Live Port stops being the live output cable the
  moment User Mode engages, the same as it stops being the live input
  cable. This means that colors snapping back to Live's the instant User
  Mode exits (still true) happens because Live Port becomes live again,
  not because Live's writes were ever blocked.

This makes User Mode a genuine, working **full** contention workaround,
not only a pad-input one. Engaging it gives a host exclusive pad input
*and* exclusive pad LED output, both routed away from Live at the device
level, not a routing suggestion Live can still listen past. **The
catch: a host must target the matching output cable for its mode.**
Writing pad LEDs to Live Port while in User Mode silently goes nowhere.

**Correction, confirmed 2026-08-20: `internal/midi` already handles this,
and no code change is necessary.** `OpenRef` pairs each `PortRef`'s input
cable with its own same-role output cable (`ports.go`'s pairing logic,
exact-name match on macOS/Linux, positional on Windows). It does not
hardcode Live. Running `pushapp -midi-in "Ableton Push 3 User Port"`
while User Mode is engaged and Live is running was tested end-to-end: pad
presses arrive through User Port, and `SetPad` writes land on User Port's
paired output cable automatically, rendering `pushapp`'s own colors live,
with Live running. What is still Live-hardcoded: `Open()`'s zero-arg
auto-detect (it guesses Live Port by design, with no selection given) and
`internal/identify.FlashLEDs`'s use of the unpaired `OpenOutCable` — see
[led-output.md](led-output.md). Neither is the general path a caller with
an explicit `PortRef` or name goes through. This does not affect the
display claim or button routing either way.

Windows names ports differently from CoreMIDI/ALSA — see
[platform/windows.md](../platform/windows.md).

### External Port role, confirmed 2026-08-27

Push 3's External Port is its physical MIDI DIN connector on the back of
the unit — the same jacks a hardware synth or drum machine would plug
into. Confirmed live: a module declaring `NeedsMIDIIn` received bytes sent
into the DIN input jack, and a module declaring `NeedsMIDIOut` (`seq`) had
its output arrive at the DIN output jack, both routed through
`internal/bootstrap.Options.ExtMIDIInFromPushExternal`/
`ExtMIDIOutToPushExternal` opening the cable directly (`midiin`/
`midiout.OpenExisting`) rather than through the app's own virtual loopback
port. See [architecture/module-host.md](../architecture/module-host.md#routing-through-push-3s-external-port-instead).

This is a real OS-visible MIDI cable, distinct from the undocumented
`xPort` USB vendor interface (interface 6) documented in
[xport.md](xport.md) — same name coincidence, unrelated hardware paths.
What remains unconfirmed: the keepalive traffic mentioned above (still
unidentified), and whether External Port carries anything else in normal
use beyond what a host explicitly sends/receives on it.

## MPE

**Resolved, 2026-08-25: MPE on/off is a persistent setting in Push 3's
own onboard settings menu** (an Aftertouch mode: Polyphonic Aftertouch
versus MPE). This is a device-level configuration choice, not something
Live negotiates over the wire. It is not gated by any handshake, and it
is not tied to Live's presence at all.

This is confirmed both ways on real hardware, in the same session. With
the device set to Polyphonic Aftertouch, pads stayed on channel 1 through
every condition tried (Live closed, and Live open and actively holding
the display as a real control surface — see below). Switched to MPE on the
device itself, pads immediately round-robined across member channels
2-16 with real, continuous `slide`/`bend`/`pressure` data. This was
reproduced again with Live fully quit, `pushapp -module monitor` alone
driving the display in plain controller mode, with no Live involved at
any point.

So "assume MPE is always on" (the original claim this section carried)
and "MPE is sometimes on, trigger unknown" (this section's own earlier
correction, the same day) were both wrong in the same way: both chased a
protocol-layer explanation for what turned out to be a simple hardware
setting.

**What this retires:** every protocol-layer hypothesis chased earlier the
same day. This includes the MPE Configuration Message (RPN 6) not
working, the recurring vendor SysEx on Live Port
(`docs/protocol/live-handshake.md`), the standalone-mode Unix-socket IPC
channels, and "Live running as control surface" as a trigger. None of
them was the answer. All were real findings about other things — the
SysEx traffic and the IPC sockets remain genuinely unexplained, just no
longer suspected of gating MPE.

**Practical consequence, unchanged: a module must not assume either
state.** The decoder handles pads on channel 1 and MPE member channels
2-16 alike, so a module must not assume one or the other. A user's own
Push can be configured either way, and nothing in the wire protocol
announces which. There is no way to query the device's current
Aftertouch-mode setting over MIDI, so a module has to infer it live, per
hold, from the pad's own channel (1 = Polyphonic Aftertouch/Push 2, 2-16
= MPE). `modules/padpointer`'s crosshair page does exactly this: MPE gets
real sub-pad `slide`/`bend` positioning, and Polyphonic Aftertouch (or
Push 2) falls back to the coarse per-cell behavior every page always had.
A module that wants richer per-note data must still handle the
Polyphonic-Aftertouch case gracefully, because it is a real, user-chosen
device state, not a fallback for an edge case.

### `slide`/`bend` behavior, confirmed live 2026-08-25 (once MPE is on)

Building `modules/padpointer`'s crosshair page against real MPE data
surfaced two non-obvious facts. Both are load-bearing for anything that
maps these values to screen position.

- **`slide` (CC 74) is a *per-pad local* reading, not a grid-wide
  position.** Each pad's own sensor spans roughly its own 0-127 range
  across just that pad's height. Sliding a finger down within one pad
  drives the value to that pad's own minimum right at the boundary with
  the pad below, which then starts back at its own maximum. A module
  must map it against the currently-held pad's own cell, not treat it as
  an absolute position across the whole strip.
- **`bend` (pitch bend) does not swing anywhere near its full 14-bit
  range (0-16383) during a normal within-pad gesture.** A real
  edge-to-edge slide only reaches a small slice of the theoretical range,
  confirmed live. Code that assumes the full range (for example,
  splitting 8192 either side of center) makes real gestures register as
  a tiny fraction of the intended movement. `padpointer` handles this by
  auto-calibrating against the actual min/max bend values observed live
  (`Module.bendMin`/`bendMax`, module-lifetime, monotonically widening),
  rather than a fixed constant. "Slide fully right" is defined as "the
  most-right value this specific Push has actually reported." This
  converges to true edge-to-edge with use, instead of needing a magic
  number guessed from one capture.

Not yet measured:

- The exact menu path on Push's own screen. A future doc update, or
  MANUAL.md, must name it precisely. "Push's settings" is what confirmed
  this, not yet the exact label or location.
- Whether this setting is readable or settable over MIDI/SysEx from the
  host side. If it becomes readable, a module or `pushapp` can detect or
  adapt to the device's current mode instead of silently guessing.
- The actual numeric bounds of `bend`'s practical range. Auto-calibration
  sidesteps the need for this, but a captured number is still a useful
  protocol fact.

## Decode order

1. **Filter Active Sensing** (`0xFE`, ~37/sec, over half of all traffic).
   Test for system realtime (`0xF8`–`0xFF`) **before** masking with
   `0xF0`. Otherwise `0xFE` decodes as SysEx.
2. **Decode channel first, then CC.** CC 71–79 are the nine encoders, but
   CC 71/74 are also MPE timbre controllers. The channel disambiguates
   them.

## Pads

- 8×8 grid, notes **36** (bottom-left) to **99** (top-right)
- Push 2: pads on channel 1 (no MPE)
- Push 3: MPE is sometimes on, sometimes off — see above. Do not assume
  either state.

**Channel Pressure (`0xD0`) works on channel 1, MPE or not.** Confirmed
2026-08-25 on real Push 3 hardware: a held pad sends continuous Channel
Pressure, ramping smoothly with the force applied to it. This is not
positional — a controlled test pressed harder without moving, and the
value tracked force, not location. This was previously decoded only on
MPE member channels (2-16, `Expression{Kind:"pressure"}`). Channel 1
messages were silently dropped. This is fixed in `internal/midi/midi.go`'s
`DecodeFor`, which now decodes channel 1 too. With MPE off, there is no
per-note channel to attribute a reading to when multiple pads are held at
once, so a module must pick its own attribution rule (for example,
`modules/padpointer` and `modules/paddebug` both attribute it to whichever
pad was pressed most recently).

Per-note `slide` (CC 74) and `bend` (pitch bend) remain MPE-only. CC 74 on
channel 1 is claimed by Encoder 4 (see Decode order above), so there is
no unambiguous channel-1 equivalent for `slide` or `bend` the way there
is for pressure.

## Encoders

Nine relative encoders plus a jog wheel:

| Control | CC | Encoding |
|---|---|---|
| Encoder 1–8 | 71–78 | Relative two's-complement |
| Jog wheel | 70 | Relative (`push3.IsEncoderCC` covers it) |
| Volume | 79 | Relative |
| Tempo | 14 | Relative |

Direction: **`1` = CW, `127` = CCW**. Decode with `push3.DecodeRel`.

**Encoders accelerate.** Fast turns produce deltas up to ±11. Never
assume that one message equals one click. Always use the decoded signed
value.

## Touch sensors

| Sensor | Note |
|---|---|
| Encoders 1–8 | 0–7 |
| Volume wheel | 8 |
| *(note 9)* | unused on Push 3, Swing on Push 2 |
| Tempo wheel | 10 |
| Jog wheel | 11 |
| Touch strip | 12 |
| D-Pad center | 13 |

Note On velocity 127 means contact. Velocity 0 means release.
Authoritative: `core/push3`.

## Buttons

CC messages: 127 means press, 0 means release. CC 104–107 are above the
screen. CC 20–22 are below (full map in upstream
[push3-button-map.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-button-map.md)).

**The button map is complete:** Push 3 has 87/87 CC and 13/13 touch. Push
2 has 75/80 CC and 12/14 touch. There are zero unknowns on either device.

Two CCs differ per device. Always resolve them with
`pushmap.ButtonNameFor` / `IsRelativeEncoderCCFor` and a known device:

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

- Push 2 arrow down/right CCs (expected 46/47/44/45)

See [plans/2026-08-18-open-items.md](../../plans/2026-08-18-open-items.md).
</content>
