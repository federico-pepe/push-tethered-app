# USB and hardware safety

**Status:** living policy  
**Last verified:** 2026-08-18  
**Authoritative code:** [internal/display/display.go](../../internal/display/display.go)

Read this before USB experiments or button sweeps on real hardware.

## USB rules

- **Claim only interface 0 (display).** Claiming MIDI or audio interfaces
  takes them away from the OS and any DAW.
- **Never write to `xPort` (interface 6) speculatively** — vendor-specific,
  undocumented, Push 3 only.
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
survives (enumeration, MIDI ports, audio). The claim releases when Live quits;
no replug needed.

Report "Live owns the display" and degrade; do not crash.

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

**ASCII only on screen.** `core/gfx/text` uses `basicfont.Face7x13`; non-ASCII
renders as a missing-glyph box. The host also sanitises text as defence in
depth.

**Look at the screen, not just the logs** when debugging.

## Related

- [display.md](display.md) — interface 0 details
- [push2-vs-push3.md](push2-vs-push3.md) — xPort absent on Push 2
