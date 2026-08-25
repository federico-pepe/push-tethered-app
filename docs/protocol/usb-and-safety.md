# USB and hardware safety

**Status:** living policy  
**Last verified:** 2026-08-20  
**Authoritative code:** [internal/display/display.go](../../internal/display/display.go)

Read this before USB experiments or button sweeps on real hardware.

## USB rules

- **Claim only interface 0 (display).** Claiming MIDI or audio interfaces
  takes them away from the OS and any DAW.
- **Never write to `xPort` (interface 6) speculatively** — vendor-specific,
  undocumented, Push 3 only. Working hypothesis, unconfirmed, updated
  2026-08-25 after checking live on the device (`ableton-push-hack`'s
  `push3-internals.md`, from Ableton's GPL kernel source release plus
  direct SSH inspection — an initial "SoC composes an external gadget"
  theory from the kernel config alone was tested and killed: no gadget
  instance or UDC exists at runtime). Simplest theory now: Push 3's
  internal XMOS co-processor (the actual USB device, `2982:1969`, 7
  interfaces) presents directly to whichever side currently has it —
  external tethered computer, or the SoC itself in standalone mode — so
  `xPort` (host-facing interface 6) most likely *is* XMOS's own interface
  6, documented there as "Hardware control (LEDs, battery?)", not a relay
  of anything. Doesn't change the rule; still don't touch it without a
  specific, understood reason.
  **The rule has always been specifically about writing** — traced
  2026-08-25 to this project's very first commit, pure precaution from
  day one, never a reaction to any incident. A passive, read-only listen
  on `xPort`'s own IN endpoint (never interface 0, never a write) was
  tried the same day and found real, structured, unprompted traffic —
  see [xport.md](xport.md) for the capture and what's known so far. Still
  not decoded; still don't write to it.
- **No firmware operations. Ever.** No DFU, no control transfers with unknown
  vendor requests.
- **Never run against a Push mid-OS-update.**
- A wedged display recovers with replug or power-cycle — expected worst case.

## gousb: do not enable AutoDetach

**Never call `dev.SetAutoDetach(true)`.** It is config-wide, not interface-wide
— `Device.Config()` detaches every interface, tearing audio and MIDI from OS
class drivers. Fails outright on macOS with `LIBUSB_ERROR_ACCESS`.

If Linux reports `LIBUSB_ERROR_BUSY` when claiming, detach interface 0 alone.

## Live exclusivity

With Live running and Push as its control surface, claiming interface 0 fails
with `LIBUSB_ERROR_ACCESS` — cleanly, before any write. Everything else
survives (enumeration, MIDI ports, audio).

Report "Live owns the display" and degrade; do not crash.

**The degrade is one-shot, not retried.** `display.go:51` claims once at
startup and does not poll — confirmed 2026-08-20: a `pushapp` process that
degraded to MIDI-only because Live had the screen stays MIDI-only for its
entire run even after Live quits and the interface frees up. Only a fresh
launch reclaims the display; there is currently no in-process retry/reclaim
path.

## Ableton background processes (confirmed 2026-08-20)

The actual claimant of interface 0 is not Live itself but a background helper
Live spawns: **`Push3.app`** (bundle id `com.ableton.Push3`,
`LSBackgroundOnly = true`), living at
`<Live.app>/Contents/Helpers/Push3.app/Contents/MacOS/Push3`, present
identically across Live 12 Suite, 12 Beta. Push 2 has a sibling,
`Push2DisplayProcess.app` (`com.ableton.Push2DisplayProcess`), under
`<Live.app>/Contents/Push2/`. Not launchd-managed — no LaunchAgent/Daemon
plist anywhere; it is a plain child process of Live
(`ps` shows `--parent-process-id=<Live's pid>`), spawned only when Live
launches.

Ownership matrix, all measured on real hardware:

- **Push plugged in, Live not running:** the helper does not start on its
  own. `frametest` succeeds. `pushapp` is safe to use with Push connected and
  Live never having run.
- **Live running:** the helper holds interface 0; `frametest` fails with
  `ErrBusy` as documented above, regardless of whether `pushapp` or Live
  claimed first — order only decides which one keeps the screen, not whether
  contention exists.
- **Killing just the helper, Live still running:** the helper is not a
  persistent affordance to build a "stop" button around. It has no config to
  disable and no `launchd` `KeepAlive` — but Live itself watches and
  respawns it in **~2.3s**. The display briefly frees during that window,
  but the pads snap back to Live-driven the moment the helper returns; a
  race, not a state.
- **Clean Live quit:** both Live and the helper exit immediately, confirmed
  with no lingering claim at +0s/+10s/+60s. **The previous claim on this page
  that "the claim releases when Live quits; no replug needed" is correct only
  for a clean quit** — see below for the crash case.
- **`kill -9` on Live (crash, not clean quit):** the helper survives,
  orphaned (reparented to pid 1) and **still holding interface 0** —
  `frametest` fails immediately after the crash. It is not indefinite: the
  helper polls its parent's liveness (matches the `--parent-process-id` flag)
  and self-exits, measured at **~5.2s** after the crash, after which
  `frametest` succeeds normally. So a Live crash leaves a real multi-second
  window where the display stays claimed with no Live process around to
  explain why — a genuine gap versus "no replug needed," not a documentation
  error to wave away.

None of this changes the guidance: don't run `pushapp` with Live open, unless
Push's own User Mode is engaged (see
[midi-input.md](midi-input.md#user-modes-effect-on-routing)).

## Button sweep safety

- **The leftmost button above the screen switches Push 3 into standalone
  mode**, dropping out of controller mode mid-session.
- **Never do a blind "press every button" sweep** — ask which controls have
  device-level functions first.
- **Hold the display first.** Run `pushapp` before sweeping — once a host
  drives the screen, top-row buttons become plain MIDI and are safe to press.
- **Identify ambiguous controls by touch sensor**, not press order — a press
  bracketed by a touch note on/off proves which physical control it belongs to.
- Recovery: switch back to controller mode on the device. The capture in
  progress is void; nothing else is lost.

## Drawing constraint

**ASCII only on screen.** `core/gfx/text`'s basic face (Tamzen7x13r, embedded
as an outline font) and its styled faces (Helvetica Neue, via `NewFace`) both
sanitize to ASCII themselves before drawing — an antialiased outline font's
glyph coverage isn't a free ASCII guarantee the way the old fixed
`basicfont.Face7x13` bitmap was, so the package can't rely on the font alone
to reject non-ASCII. The host also sanitises text as defence in depth.

**Look at the screen, not just the logs** when debugging.

## Related

- [display.md](display.md) — interface 0 details
- [push2-vs-push3.md](push2-vs-push3.md) — xPort absent on Push 2
- [xport.md](xport.md) — the read-only xPort capture and what's known so far
