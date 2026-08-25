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

**Where MPE-on actually comes from remains unknown** *(at this point in the
day — resolved later, see the Open items entry below: it's a device-side
settings-menu toggle, nothing to do with any of the protocol theories
chased below)*. The best lead is
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

**2026-08-25 status:** Phase 0/1 committed and pushed
(`0fd9c5a` on `push-hub-pointer`, off `live-screen-mirror`).

## GPL source discovery (2026-08-25, same day, after Phase 1 shipped)

Ableton published GPL source for Push 3 firmware v2.4.2 — Federico dropped
it at `ableton-push-hack/resources/push-assets/push3-242-gpl-sources.tgz`
(gitignored, ~725MB Yocto/OpenEmbedded dump: kernel + stock GPL package
sources, not the proprietary rootfs). First pass (kernel config only) found
`CONFIG_USB_GADGET`/`CONFIG_USB_CONFIGFS`/`CONFIG_USB_CONFIGFS_F_FS` all
present and built a theory around it — **then live SSH access to a real
Push 3 was available this same session, so the theory got checked
immediately instead of staying a guess.** Findings below are post-SSH,
corrected. Full writeup: `ableton-push-hack/docs/push3-internals.md`
("External-facing USB personality — gadget theory tested and killed",
"Live↔Push3 IPC sockets", "Mouse/keyboard, live test" — all dated
2026-08-25); cross-referenced into this repo's `live-handshake.md`,
`midi-input.md`, `usb-and-safety.md`.

**1. Gadget theory: dead.** `ls /sys/kernel/config/usb_gadget/` and
`ls /sys/class/udc/` both come back empty on the real device — no gadget
instance, no USB Device Controller registered at runtime. The kernel
config's options are just compiled-in capability, unused here.
`lsusb -t`/`lsusb` from the SoC's own side show it hosting a real hub
(`0424:2534`, SMSC/Microchip) with `2982:1969 "Ableton Push 3"` — the
XMOS-driven device — as a child, 7 interfaces, exact same VID:PID and
layout push-tethered-app sees over the external tether. **New leading
theory:** there's no SoC-composed gadget at all; XMOS's own USB device
presents directly to whichever side currently has it (external tethered
computer, or the SoC itself in standalone mode), mediated by the hub and
whatever the mode-switch button toggles at the hub/mux level. Simpler,
and explains "mutually exclusive, not concurrent" for free. Still
unconfirmed — would need catching the hub's port assignments change
between modes.

**2. The three `/data` IPC sockets are real, live, and currently
connected** (`live-to-push-midi-ipc-channel`, `push-to-live-midi-ipc-channel`,
`push-flip-api-ipc-channel`, all showing a connected peer in
`/proc/net/unix`) — but **standalone-mode only.** `ps aux` at check time
showed `/opt/push3/Live` actually running on-device (Push was in
standalone mode, own bundled Live active), so these are the local IPC
between Push3's *onboard* Live and its *onboard* hardware-control app.
A USB tether can't carry a Unix socket, so this channel is unreachable
from push-tethered-app's case regardless of what it turns out to do.
Doesn't solve the MPE trigger directly, but does confirm the onboard
`Push3` process is a real, stateful negotiation participant — reinforcing
(not proving) that an equivalent negotiation for the *tethered* case would
have to happen over the MIDI wire itself, putting `live-handshake.md`'s
recurring vendor SysEx back in play as a candidate.

**3. Mouse/keyboard: confirmed working end-to-end, same session.** Federico
plugged a real Keychron K2 keyboard into the USB-A port. `modprobe usbhid`
+ clean enumeration (4 HID input devices, hub port `1-1.2` — same internal
hub as XMOS, at port `1-1.4`), **nothing else grabs it** (`fuser` empty on
every `/dev/input/eventN`, Xorg included), and raw keystrokes captured
live off `/dev/input/event4` decode correctly (`EV_KEY` press/release for
the keys actually typed). **One real constraint:** the normal `ableton`
SSH account isn't in the `input` group and the device node is
`root:input 0660` — reading it needs `root` or a one-time provisioning
step (add `ableton` to `input`, or a udev rule). Conclusion: the
kernel/driver path is fully proven; a standalone hack can read a real
mouse/keyboard today.

**4. Bigger, same session, no code needed: keyboard shortcuts already
control the onboard Live, visibly reflected on Push's own screen.**
Federico typed real Live shortcuts (`Ctrl+N` new set, `Ctrl+T` new audio
track, `Ctrl+Shift+T` new MIDI track) on the plugged-in keyboard and
watched them execute, live, on Push 3's own physical screen. Traced end to
end: `Xorg` hotplug-claims the keyboard (with a startup delay — the first
`fuser` check ran too early and missed it) → `/opt/push3/Live` (running,
standalone mode) is a normal X11 client on `:0` and runs its own native
shortcuts on ordinary X key events, nothing Push-specific → the resulting
session-state change reaches Push3's own onboard app via IPC (almost
certainly `push-flip-api-ipc-channel`) → Push3 redraws its own DRM/KMS
screen to reflect it. Full writeup:
`ableton-push-hack/docs/push3-internals.md`'s "Keyboard shortcuts control
the onboard Live" section. **This changes what's worth building first** —
see below.

## Phase 2 — mouse/keyboard via Push's internal hub

### 2a. Host-side (push-tethered-app) — not started, unchanged

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

### 2b. Device-side (ableton-push-hack, SSH) — done, this session, plus a bonus

All three original checks run live against the real device, findings
above — no loose ends left on that list. **Unplanned fourth finding
(above, #4) is arguably the most actionable thing to come out of this
whole investigation:** keyboard shortcuts already control the onboard
Live with zero new code. Worth a short, real follow-up in
`ableton-push-hack` before anything else in this plan: catalog which Live
shortcuts are actually useful standalone (new set/track already proven;
worth checking rename-via-typing, save, undo/redo, search/browser
navigation), and whether mouse clicks work the same way (same X11
mechanism, untested). This is cheap (no code, just more SSH+keyboard
sessions) and could make a real standalone-mode UX improvement without
touching the display protocol at all.

Deliverable for 2a is still a findings + recommendation writeup, not a
merged feature — the actual mouse/keyboard-driven Shadow UI build gets its
own plan once an approach is chosen. 2b's bonus finding is a candidate for
its own small follow-up plan in `ableton-push-hack`, separate from 2a's
push-tethered-app scope entirely.

## Shipped alongside this, same day: push-manager USB Input checkbox

The keyboard-controls-Live finding above led straight to a small real
feature in `ableton-push-hack`, not just documentation: `usbhid` isn't
loaded by default, so `push-manager` (already running as `root` — no
privilege escalation needed) got a new `/api/input/usbhid` endpoint pair
(`handleUsbHid`/`handleUsbHidStatus` in the new `usbinput.go`) and a
checkbox in its System view, same toggle-with-revert-on-failure pattern as
the existing MIDI Intercept control. Per-session only (no boot
persistence), `modprobe -r` on uncheck. Also fixed an unrelated
pre-existing `go.sum` gap that was blocking any build of that module.
Built, deployed via the real installer, and **confirmed working on
hardware** — Federico verified the checkbox toggles real keyboard
availability. Committed: `ableton-push-hack@814950a` (docs),
`ableton-push-hack@87bb141` (feature).

## Phase 2a — confirmed with real hardware, same day, no new code needed

Push tethered to this Mac (controller mode) while all of the above was
happening made the first Phase 2a question directly testable, not just a
plan: plugged a Keychron K2 into the USB-A port and diffed
`system_profiler SPUSBDataType` before/after. **Confirmed:** the keyboard
shows up as a sibling of `Ableton Push 3` under the exact same hub
(`0424:2534` SMSC — same chip model `push3-internals.md` already found
internal to the SoC, likely the same part used twice in the board design,
external-facing and internal-facing) — same Location ID prefix
(`0x0113...`), macOS enumerates it as an entirely ordinary USB keyboard.
Answers Phase 2a's first question directly: `gousb`/libusb's port-topology
calls (`Port()`, parent-device lookup) should expose the same parent-hub
match cross-platform, no OS-specific device-tree code needed to identify
a hub-child relationship. Phase 2a's second question (reading input from
that scoped device without hijacking the user's regular mouse/keyboard)
remains open — this only confirmed *identification*, not *access*.

## xPort — the write-rule's origin, and a first read-only capture

Two separate things, same session:

1. **Why "never write to xPort" exists at all — traced, not just
   asserted.** Federico asked directly; the answer was in this project's
   very first commit (`7dfe0b8`, original `docs/feasibility.md` §8.5,
   written the same day `xPort` was first seen via `cmd/probe`): pure
   first-principles caution ("undocumented... purpose unknown. Do not
   send it speculative payloads"), never a reaction to any incident, and
   specifically about **writing** — reading was never tried or forbidden.
2. **First read-only test, same day.** Claimed interface 6 only (never
   interface 0 — display untouched throughout), listened on its IN
   endpoint, zero writes. Result: real, continuous, structured,
   unprompted 512-byte packets — not silence, not noise. Full capture and
   byte-level notes: `docs/protocol/xport.md` (new). Three regions
   visible: 16-bit value pairs in ADC-sensor range, a recurring
   `FF FF FF 3F <incrementing counter>` marker (looks like a real
   sequence number in a structured periodic packet), and long idle runs
   consistent with an unpressed capacitive sensor scan. Working
   hypothesis, unconfirmed: this is the same "periodic SysEx heartbeats
   including LED state and touch sensor data" `push3-internals.md`
   documents from the standalone side, seen here on the wire instead.

**Stopped here deliberately** — next steps (touch a pad while capturing
to falsify/confirm the sensor theory, correlate the counter's cadence
against the documented ~3-5/sec heartbeat rate, light an LED while
capturing to test the other half of the hypothesis, capture longer to
find real packet boundaries) are written up in `docs/protocol/xport.md`
itself, not repeated here.

## MPE resolved, then spent on padpointer: crosshair page now uses real sub-pad position

Once MPE's trigger was found (a Push-side settings-menu toggle, not a
protocol negotiation — see below), the natural next step was using it:
`modules/paddebug` was **deleted** (module, both registration points,
CLAUDE.md entry) — its job (finding the Channel Pressure bug, and this
MPE investigation) is done. `modules/padpointer`'s crosshair page was
upgraded to detect MPE per hold (from the pad's own channel: 1 = coarse,
2-16 = MPE) and use real `slide`/`bend` for sub-pad position instead of
snapping to the pad cell, with two live-hardware iterations to get it
feeling right:

1. **First pass** used a fixed symmetric offset (±1.5 cells for bend, ±1
   cell for slide) from the anchor cell's center. Federico caught two real
   bugs testing it live: vertical jumped visibly at every pad boundary,
   and horizontal barely moved at all.
2. **Root causes, both about wrongly assuming a fixed range:** `slide` is
   a *per-pad local* sensor reading (each pad's own 0-127 spans just that
   pad's height, not a grid-wide position) — the symmetric-offset math
   double-counted range across a cell, causing the boundary jump. `bend`
   doesn't swing anywhere near its theoretical 14-bit range during a real
   gesture — assuming it did made real movement register as a tiny
   fraction of intended.
3. **Fixes, confirmed working on hardware:** vertical now maps `slide`'s
   full 0-127 directly across the anchor cell's own height (no radius
   doubling) — Federico confirmed this "perfect." Horizontal now
   auto-calibrates against the actual min/max `bend` values ever observed
   (`Module.bendMin`/`bendMax`, module-lifetime, only ever widens) instead
   of a fixed constant, and splits available screen space asymmetrically
   around the anchor column (an edge pad has no room on one side) — per
   Federico's explicit spec: full bend in either direction should always
   reach that edge of the screen, from any column. Confirmed working.

Also caught and fixed along the way: two status strings used an em-dash,
which the project's own ASCII-only drawing rule turns into a broken
glyph — confirmed visually via `screensim` before and after the fix. Full
technical writeup, including the exact per-pad-local-slide and
small-bend-range facts as protocol facts (not just implementation notes):
`docs/protocol/midi-input.md`'s MPE section.

## Open items

- ~~**What triggers MPE**~~ **Resolved, 2026-08-25, same session.** Not a
  protocol-layer trigger at all — a persistent setting in Push 3's own
  onboard settings menu (Aftertouch mode: Polyphonic Aftertouch vs. MPE).
  Confirmed both states on real hardware: with Live 12 Suite running and
  `Push3.app` confirmed holding the display, pads stayed on channel 1
  (device was set to Polyphonic Aftertouch at the time — this is the
  "Live running as control surface ruled out" result recorded earlier the
  same day, now correctly attributed); switched the device to MPE and
  reproduced round-robin channels + real `slide`/`bend`/`pressure` data
  twice — once with Live still running, once with Live fully quit and
  `pushapp -module monitor` alone driving the display in plain controller
  mode. Every protocol-layer theory chased earlier the same day (RPN 6,
  the vendor SysEx heartbeat, the IPC sockets) was chasing the wrong
  layer — all real findings about other things, none of them the actual
  answer. Full writeup: `docs/protocol/midi-input.md`'s MPE section.
- **`xPort`'s real function** — theory unchanged ("is XMOS's own interface
  6 directly", documented as "Hardware control (LEDs, battery?)"), but no
  longer untested: a read-only capture found real structured traffic (see
  `docs/protocol/xport.md`). Not decoded yet — next steps are in that doc.
  Rule in `usb-and-safety.md` unchanged (still write-only caution).
- ~~**Mouse/keyboard end-to-end enumeration**~~ **Closed, 2026-08-25.**
  Confirmed working: real keyboard, clean enumeration, unclaimed by Xorg,
  live keystroke capture verified. Remaining detail for a future
  implementation, not a research question: the `ableton` account needs
  `root` or `input`-group membership to read `/dev/input/eventN`.
- ~~**`modules/paddebug`'s fate**~~ **Closed, 2026-08-25.** Deleted — its
  job (finding the Channel Pressure bug, then this whole MPE
  investigation) is done, and `modules/padpointer`'s crosshair page now
  covers live slide/bend visibility for anyone who needs it.
- **Phase 2a (host-side, push-tethered-app)** — first question (hub-child
  identification) confirmed with real hardware, no code written yet
  (`gousb`/libusb port-topology calls, not implemented). Second question
  (reading input from the scoped device without hijacking the user's
  regular mouse/keyboard) untouched — still the real open design question
  for this half of Phase 2.
