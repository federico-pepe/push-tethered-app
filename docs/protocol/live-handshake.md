# Live↔Push handshake (raw observations, mechanism unconfirmed)

**Status:** partial — raw traffic recorded, meaning not yet decoded
**Last verified:** 2026-08-20 (Push 3, macOS, Live 12 Suite)
**Authoritative code:** none yet — this is capture evidence only

> **Possible explanation, 2026-08-25** (still not byte-decoded, and now
> genuinely contested by a second finding — see update below): one theory
> is this isn't a Live↔Push handshake at all, but Push 3's own onboard
> heartbeat. `ableton-push-hack/docs/push3-internals.md` (SSH access to
> the device itself) documents, independently of this capture, that
> "Push3 sends periodic SysEx heartbeats (~3-5/sec) including LED state
> and touch sensor data" — same manufacturer ID (`00 21 1D`), same rough
> cadence, same "independent of pad/button activity" character recorded
> below.
>
> **Update, same day:** an initial theory that Push 3's external USB
> personality is a Linux-gadget-composed device (which would have made
> the onboard `Push3` process an obvious author for *any* SysEx on that
> link, heartbeat or handshake) was tested live on the device and killed —
> no gadget/UDC exists at runtime; the external personality is more likely
> the internal XMOS co-processor's own USB device presenting directly, not
> anything the SoC composes. That doesn't kill the heartbeat theory (the
> onboard `Push3` app still plausibly authors SysEx over the ALSA MIDI
> path either way), but it does mean the "obviously it's this process"
> reasoning was weaker than it looked.
>
> **Update, later the same day — the MPE angle is dead.** This traffic is
> **not** what gates MPE: MPE on/off turned out to be a persistent setting
> in Push 3's own onboard settings menu (Aftertouch mode), confirmed
> independent of Live's presence entirely — see
> [midi-input.md](midi-input.md)'s MPE section, now resolved. This
> traffic's own mystery is unaffected by that (still genuinely open
> between "unconditional heartbeat, routed only while Live holds the
> connection" and "some other real negotiation") — it's just no longer a
> candidate for the *MPE* question specifically. The `0x3A`/`0x38`
> command bytes below still aren't decoded against a real payload.

This page exists to not lose the raw evidence. Nothing here is a confirmed
protocol fact the way the rest of `docs/protocol/` is — treat every command
below as "observed, uninterpreted" until decoded.

## What's captured

With Live running (any state — idle, Session View open, no deliberate user
action needed) and `tools/midimon.swift` listening on all three Push 3 input
ports, a small set of SysEx messages recurs continuously on **Live Port
only**, at roughly one burst every 0.2–0.5s:

```
F0 00 21 1D 01 01 3A 22 64 F7
F0 00 21 1D 01 01 38 0D 00 00 3A 00 00 3B 00 00 3C 00 00 00 00 00 F7
F0 00 21 1D 01 01 38 18 00 00 19 00 00 00 00 00 00 00 00 00 00 00 F7
```

Observed facts, nothing more:

- **Present only when Live is running.** Confirmed absent in every capture
  where Live was closed and only `pushapp`/`ledtest` were active (2026-08-20,
  cells 1–5 of [live-coexistence.md's A2](../../plans/2026-08-19-live-coexistence.md)).
- **Independent of pad presses, button presses, and our own LED writes.**
  Recurs at its own cadence whether or not anything is touched — see A5's
  clean-negative captures in the same plan (no NoteOn/CC/PolyAT ever
  accompanies it).
- **Independent of Session View activity.** Recurs at roughly the same rate
  whether Session View colours are actively being changed or the session is
  idle — not obviously tied to a specific Live-side event.
- **Appears on `midimon`'s *source* (device→host) capture of Live Port
  only** — never seen on User Port or External Port in any capture so far.
- **Command byte after the `F0 00 21 1D 01 01` vendor prefix**: `0x3A` and
  `0x38` seen so far, matching the vendor-command-prefix pattern documented
  for other known commands (device→host palette lookup `0x04`,
  User-Mode-toggle `0x0A` — see
  [midi-input.md](midi-input.md#user-modes-effect-on-routing)). Not
  cross-referenced against the upstream command table yet.

## What this is not (yet) confirmed to be

Working guess, **unconfirmed**: this could be part of a periodic handshake
or keepalive between Live's `Push3.app` helper (see
[usb-and-safety.md](usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20))
and the device — independent of pad/LED activity, the way `0xFE` Active
Sensing is independent of note traffic. Given it only shows up on a *source*
(input) endpoint while being correlated with Live's presence, it is
**not** simply "the bytes Live sends out being visible to another client" —
CoreMIDI source and destination endpoints are distinct objects even when
they share a display name, so if this reasoning is right the device itself
must be the one emitting these bytes back toward the host. That would make
it a genuine device-authored signal, not an artifact of the capture tool —
but this hasn't been isolated from the alternative explanation (some
CoreMIDI-level visibility quirk on this specific USB-MIDI composite device)
by controlled experiment yet.

## Why it matters

If this traffic turns out to be part of a real handshake or keepalive
protocol between Live and Push, it could become a much stronger contention
signal than process-presence (see
[live-coexistence.md Part C](../../plans/2026-08-19-live-coexistence.md)) —
something observed directly on the wire rather than inferred from `ps`.
Not relied on for anything yet; C1's ranking (process presence primary,
`ErrBusy` corroborating) stands until this is decoded.

## Next steps (not started)

- Test the "unconditional heartbeat, routed only when Live holds the
  connection" theory above directly: capture with `pushapp` alone (no
  Live) in both regular and User Mode, since User Mode is the one state
  already confirmed to change Live-Port/User-Port routing — if the
  heartbeat appears there too, it's unconditional and the "only when Live
  is running" observation was about routing, not emission.
- Correlate the `0x3A`/`0x38` commands against the upstream command table at
  [ableton-push-hack](https://github.com/federico-pepe/ableton-push-hack) if
  one exists there.
- Try to force a change in cadence or content (switch Live's active track,
  change a clip colour deliberately, open/close Session View) with tighter
  timestamp correlation than the "idle vs. active, no obvious difference"
  read done so far.
- Determine mechanism: real device-authored signal vs. CoreMIDI capture
  artifact, by testing on a second machine/CoreMIDI version if one becomes
  available.

## Related

- [midi-input.md](midi-input.md) — User Mode's `0x0A` toggle, decoded
- [led-output.md](led-output.md) — LED contention with Live
- [usb-and-safety.md](usb-and-safety.md) — the `Push3.app` helper this may
  be talking to
