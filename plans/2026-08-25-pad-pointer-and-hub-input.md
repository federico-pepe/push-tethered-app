# Pad-grid pointer + Push-hub mouse/keyboard

## Status (2026-08-25)

Started from a feasibility question: could a mouse/keyboard plugged into
Push 3's USB-A port control push-tethered-app's Shadow UI, and could the
8x8 pad grid itself work as a coarse touchpad? Both turned out feasible,
with the scope reshaped twice by real hardware findings along the way (see
below). **Phase 0 and Phase 1 are done and shipped. Phase 2
(mouse/keyboard via Push's hub) has not been started.**

### Corrections that reshaped the original plan

The first-pass research assumed the USB-A port was standalone-mode-only and
that pads gave one discrete note each. Federico corrected both against real
hardware/prior investigation:

- The USB-A port is a **powered USB hub, live in controller mode too** — a
  mouse/keyboard plugged in enumerates on the host computer like any other
  USB peripheral, a child of an internal hub, entirely separate from
  Push 3's own composite USB device. None of `docs/protocol/usb-and-safety.md`'s
  interface-claiming rules apply to it.
- Push 3's pad grid is one physical MPE surface (Federico can slide a
  finger continuously between pads), not 64 independent buttons —
  `internal/module/event.go`'s `Expression{Kind:"slide"|"bend"|"pressure"}`
  already existed to carry this, just unexercised.
- A prior standalone-mode HID investigation (kernel modules present but
  unloaded, Push 3's own `--faceless` app never reads X11/evdev input) means
  mouse/keyboard control of Push's **own onboard UI** doesn't work — but
  that's irrelevant to push-tethered-app, which runs on the host computer
  and only cares about the hub-passthrough path.

## Phase 0 — characterize pad expression data on real hardware (done)

Goal was to confirm whether `slide`/`bend`/`pressure` stay continuous
across pad boundaries on Push 3. What actually happened, in order:

1. Built `modules/paddebug` (throwaway diagnostic, still kept — see below)
   to show live slide/bend/pressure readouts.
2. **First surprise: all three read 0.** Turned out pads were arriving on
   **channel 1**, not MPE's per-note channels 2-16 — contradicting
   `docs/protocol/midi-input.md`'s old "assume MPE always on" claim (now
   corrected in that doc).
3. Tried the standard MIDI MPE Configuration Message (RPN 6, lower zone, 15
   member channels) on activation, on the theory that Live sends this on
   connect and we never had. **It did not turn MPE on.** Ruled out.
4. Added a temporary raw-MIDI logger (since removed — see Cleanup below)
   to see what Push actually sends on channel 1. Found real, continuous
   **Channel Pressure (`0xD0`)** messages — confirmed on hardware to track
   force (pressed harder without moving = value went up; moved without
   pressing harder = value didn't), not position.
5. **Real bug found and fixed:** `internal/midi/midi.go`'s `DecodeFor` only
   decoded Channel Pressure on MPE member channels — channel-1 messages
   were silently dropped. Fixed, with a regression test
   (`TestChannelPressureOnChannel1` in `internal/midi/decode_test.go`).
   `docs/protocol/midi-input.md` updated with the finding.

**Where MPE-on actually comes from remains unknown.** The best lead is
`docs/protocol/live-handshake.md`'s undocumented Ableton vendor SysEx
(`F0 00 21 1D 01 01 <cmd> ...`), observed only while Live itself is running
— likely a proprietary handshake, not anything in the standard MIDI spec.
**Not chased further, by explicit choice** (Federico picked "build Phase 1
now" over "chase the vendor handshake" when asked). Slide and bend
(finer per-note positioning) stay unavailable until this is cracked —
Channel Pressure was enough to build a real pointer without them.

## Phase 1 — `modules/padpointer` (done)

Two pages, D-Pad left/right to switch (same convention as `modules/uidemo`):

- **Menu page**: pad row moves a cursor onto an 8-item on-screen menu
  (`rowToItem` maps physical row to visual position), Channel Pressure
  crossing a threshold (60/127, picked from live capture — light touch
  stayed under 30, a deliberate press passed 60) toggles the highlighted
  item. One toggle per hold (`m.holding = false` after firing) so holding
  past the threshold doesn't flicker.
- **Crosshair page** (added after the menu page worked, then tuned once
  more): the full 8x8 grid positions a crosshair anywhere on screen
  (`crosshairXY` maps col/row to a pixel position). A light touch just
  moves it; only a firm press (same Channel Pressure threshold) triggers a
  short expanding, fading ring animation (`lerpColor` toward black, since
  the renderer's `Arc` doesn't honor a colour's own alpha channel) —
  confirmed working on hardware.

No device branch needed: Push 2 has no MPE at all and Push 3 currently
behaves the same way (channel 1, Channel Pressure only), so both pages
work identically on either device. Tests cover both pages'
Handle/Draw logic against `internal/module/moduletest`; screensim scenes
(`mod:padpointer`) render cleanly for fast iteration.

### Cleanup done this session

- The temporary raw-MIDI logger added to chase the Channel Pressure bug
  (`internal/midi/midi.go`'s `Listen`) has been **removed** — its job is
  done, and left in place it would have spammed stdout with
  `undecoded raw MIDI: ...` on every real run for any unhandled message
  (SysEx, unknown CC), not just during diagnosis.
- `modules/paddebug` is **kept**, deliberately, until there's a clearer
  understanding of what turns MPE on — see Phase 2/open items below.

## Phase 2 — mouse/keyboard via Push's internal hub (not started)

Still exactly as scoped in the original plan: a research spike, not a
known-shape build.

1. **Identify which HID devices are children of Push's hub, cross-platform.**
   Check whether gousb/libusb's port-topology calls (`Port()`, and
   libusb's `get_port_numbers`/`get_parent`, which gousb wraps) can
   enumerate Push's internal hub and a plugged-in mouse/keyboard's
   parent/child bus relationship on all three target OSes, reusing the
   existing USB stack (`internal/display/enumerate.go` already reads
   `Bus`/`Address`/`Port` for Push's own device) instead of adding three
   OS-specific device-tree APIs.
2. **Read input from that scoped device without hijacking the user's other
   mouse/keyboard.** Open question: a full OS-level global input hook
   (simple, but sees everything system-wide, needs filtering + OS
   permission prompts) vs. claiming just that device's HID interface
   directly (surgical, but takes it from the OS's own HID class driver —
   same "don't take a device away from the OS without a clear, scoped
   reason" principle as the `SetAutoDetach(true)` rule, applied to a
   different device).

Deliverable is a findings + recommendation writeup, not a merged feature —
the actual mouse/keyboard-driven Shadow UI build gets its own plan once the
approach is chosen.

## Open items

- **The Ableton vendor SysEx handshake** (`docs/protocol/live-handshake.md`)
  that likely gates MPE — undecoded, not chased. Would need capturing
  Live's real traffic (e.g. via `tools/midimon.swift`) while toggling
  something MPE-related and correlating against the `0x3A`/`0x38` command
  bytes already logged there.
- **`modules/paddebug`'s fate** — kept until the above is understood or
  deliberately abandoned; revisit then (delete, or keep as a standing
  diagnostic).
- **Phase 2 itself** — not started, see above.
- Everything in this session is on branch `push-hub-pointer`, off
  `live-screen-mirror`, uncommitted.
