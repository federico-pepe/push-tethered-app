# USB and hardware safety

**Status:** living policy
**Last verified:** 2026-08-20
**Authoritative code:** [internal/display/display.go](../../internal/display/display.go)

Read this before any USB experiment or button sweep on real hardware.

## USB rules

- **Claim only interface 0 (display).** Claiming MIDI or audio interfaces
  takes them away from the OS and from any DAW.
- **Never write to `xPort` (interface 6) speculatively.** It is
  vendor-specific, undocumented, and present on Push 3 only. This is a
  working hypothesis, unconfirmed, updated 2026-08-25 after a live check
  on the device (`ableton-push-hack`'s `push3-internals.md`, built from
  Ableton's GPL kernel source release plus direct SSH inspection). An
  initial theory that the SoC composes an external gadget, based on the
  kernel config alone, was tested and killed: no gadget instance or UDC
  exists at runtime. The simplest theory now is this: Push 3's internal
  XMOS co-processor (the actual USB device, `2982:1969`, 7 interfaces)
  presents directly to whichever side currently has it. That side is
  either the external tethered computer, or the SoC itself in standalone
  mode. So `xPort` (host-facing interface 6) is most likely XMOS's own
  interface 6, documented there as "Hardware control (LEDs, battery?),"
  not a relay of anything. This does not change the rule: still do not
  touch it without a specific, understood reason.
  **The rule has always been specifically about writing.** This was
  traced on 2026-08-25 to this project's very first commit, pure
  precaution from day one, never a reaction to any incident. A passive,
  read-only listen on `xPort`'s own IN endpoint (never interface 0, never
  a write) was tried the same day. That listen found real, structured,
  unprompted traffic — see [xport.md](xport.md) for the capture and what
  is known so far. This traffic is still not decoded. Still do not write
  to it.
- **No firmware operations. Ever.** No DFU, and no control transfers with
  unknown vendor requests.
- **Never run against a Push that is mid-OS-update.**
- A wedged display recovers with a replug or a power cycle. This is the
  expected worst case.

## gousb: do not enable AutoDetach

**Never call `dev.SetAutoDetach(true)`.** It is config-wide, not
interface-wide. `Device.Config()` detaches every interface, tearing audio
and MIDI away from the OS class drivers. It fails outright on macOS with
`LIBUSB_ERROR_ACCESS`.

If Linux reports `LIBUSB_ERROR_BUSY` when you claim the interface, detach
interface 0 alone.

## Live exclusivity

With Live running and Push as its control surface, claiming interface 0
fails with `LIBUSB_ERROR_ACCESS`, cleanly, before any write. Everything
else survives: enumeration, MIDI ports, and audio.

Report "Live owns the display" and degrade. Do not crash.

**The degrade is one-shot, not retried.** `display.go:51` claims once at
startup and does not poll. Confirmed 2026-08-20: a `pushapp` process that
degrades to MIDI-only because Live had the screen stays MIDI-only for its
entire run, even after Live quits and the interface frees up. Only a
fresh launch reclaims the display. There is currently no in-process
retry or reclaim path.

## Ableton background processes (confirmed 2026-08-20)

The actual claimant of interface 0 is not Live itself but a background
helper that Live spawns: **`Push3.app`** (bundle id `com.ableton.Push3`,
`LSBackgroundOnly = true`), living at
`<Live.app>/Contents/Helpers/Push3.app/Contents/MacOS/Push3`, present
identically across Live 12 Suite and 12 Beta. Push 2 has a sibling,
`Push2DisplayProcess.app` (`com.ableton.Push2DisplayProcess`), under
`<Live.app>/Contents/Push2/`. It is not launchd-managed — there is no
LaunchAgent or Daemon plist anywhere. It is a plain child process of Live
(`ps` shows `--parent-process-id=<Live's pid>`), spawned only when Live
launches.

Ownership matrix, all measured on real hardware:

- **Push plugged in, Live not running:** the helper does not start on its
  own. `frametest` succeeds. `pushapp` is safe to use with Push connected
  and Live never having run.
- **Live running:** the helper holds interface 0. `frametest` fails with
  `ErrBusy` as documented above, regardless of whether `pushapp` or Live
  claimed first. Launch order only decides which process keeps the
  screen, not whether contention exists.
- **Killing just the helper, Live still running:** the helper is not a
  persistent affordance to build a "stop" button around. It has no
  configuration to disable and no `launchd` `KeepAlive`, but Live itself
  watches it and respawns it in **~2.3s**. The display briefly frees
  during that window, but the pads snap back to Live-driven the moment
  the helper returns. This is a race, not a state.
- **Clean Live quit:** both Live and the helper exit immediately,
  confirmed with no lingering claim at +0s/+10s/+60s. **The previous claim
  on this page, that "the claim releases when Live quits and no replug is
  necessary," is correct only for a clean quit** — see below for the
  crash case.
- **`kill -9` on Live (a crash, not a clean quit):** the helper survives,
  orphaned (reparented to pid 1), and **still holds interface 0**.
  `frametest` fails immediately after the crash. This is not indefinite:
  the helper polls its parent's liveness (matching the
  `--parent-process-id` flag) and self-exits, measured at **~5.2s** after
  the crash. After that, `frametest` succeeds normally. As a result, a
  Live crash leaves a real multi-second window where the display stays
  claimed, with no Live process around to explain why. This is a genuine
  gap against "no replug is necessary," not a documentation error to
  dismiss.

None of this changes the guidance: do not run `pushapp` with Live open,
unless Push's own User Mode is engaged (see
[midi-input.md](midi-input.md#user-modes-effect-on-routing)).

## Button sweep safety

- **The leftmost button above the screen switches Push 3 into standalone
  mode**, dropping it out of controller mode mid-session.
- **Never do a blind "press every button" sweep.** Ask which controls
  have device-level functions first.
- **Hold the display first.** Run `pushapp` before you sweep. Once a host
  drives the screen, the top-row buttons become plain MIDI and are safe
  to press.
- **Identify ambiguous controls by touch sensor, not by press order.** A
  press bracketed by a touch note on/off proves which physical control it
  belongs to.
- Recovery: switch back to controller mode on the device. The capture in
  progress becomes void. Nothing else is lost.

## Drawing constraint

**ASCII only on screen.** `core/gfx/text`'s basic face (Tamzen7x13r,
embedded as an outline font) and its styled faces (Helvetica Neue, through
`NewFace`) both sanitize text to ASCII themselves before drawing. An
antialiased outline font's glyph coverage is not a free ASCII guarantee
the way the old fixed `basicfont.Face7x13` bitmap was, so the package
cannot rely on the font alone to reject non-ASCII characters. The host
also sanitizes text, as a second layer of defense.

**Look at the screen, not just the logs**, when you debug.

## Related

- [display.md](display.md) — interface 0 details
- [push2-vs-push3.md](push2-vs-push3.md) — xPort absent on Push 2
- [xport.md](xport.md) — the read-only xPort capture and what is known so far
</content>
