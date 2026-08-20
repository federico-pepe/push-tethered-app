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

**Assume MPE is always on by default on Push 3.** Pad note-ons arrive on
per-note channels 2–16, round-robining across them. Channel 1 all-pads
behavior has been observed in the past on this same hardware with nothing
deliberately changed, but a 2026-08-18 A/B (fresh session, reconnect-only,
full power-cycle) never reproduced it — MPE stayed on and round-robining
through all three. Given that, "always on" is the working assumption rather
than something the decoder needs to detect and branch on.

The decoder still handles channel 1 pads too (Push 2 has no MPE and always
uses it), so nothing breaks if a Push 3 session ever does show up on channel
1 — it's just not expected.

Not measured: whether MPE can be disabled via SysEx.

## Decode order

1. **Filter Active Sensing** (`0xFE`, ~37/sec, over half of all traffic).
   Test for system realtime (`0xF8`–`0xFF`) **before** masking with `0xF0`,
   or `0xFE` decodes as SysEx.
2. **Decode channel first, then CC.** CC 71–79 are the nine encoders, but CC
   71/74 are also MPE timbre controllers — the channel disambiguates.

## Pads

- 8×8 grid, notes **36** (bottom-left) to **99** (top-right)
- Push 2: pads on channel 1 (no MPE)
- Push 3: MPE, assumed always on (see above)

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
