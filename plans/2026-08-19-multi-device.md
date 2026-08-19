# Driving two or more Push units at once

Status: in progress. Phase 0 (device identity + MIDI pairing) started
2026-08-19.

## Hardware verification log

**2026-08-19, one Push 3 (serial `37589789`) + one Push 2 (no serial, `usb:1.7`)
attached simultaneously.**

- `go run ./cmd/pushapp -devices`: `display.List()` correctly found both units,
  correctly reported Push 2 has no serial (this is not hypothetical — settles
  open question 1 halfway: Push 3 *does* report a real serial; Push 2 does
  not, so the `usb:` fallback is load-bearing, not speculative).
  `midi.ListUnits()` correctly grouped by unit from the two distinct device
  names, correctly assigned roles (Push 3: Live/User/External; Push 2:
  Live/User only, matching `docs/protocol/push2-vs-push3.md`), and paired
  every input to its output 1:1.
- With both attached and no selector, `bootstrap.Open` correctly **refused**
  (`"2 Push units attached — pass a PortRef..."`) instead of silently picking
  one — the D2 change working as intended.
- `-device serial:37589789 -midi-in "Ableton Push 3 Live Port"` and
  `-device usb:1.7 -midi-in "Ableton Push 2 Live Port"` each correctly claimed
  their own unit's display and MIDI (confirmed both by log and by eye — see
  below).
- **Two separate `pushapp` processes, one per unit, run concurrently**: both
  claimed their correct display, both sustained ~29.5fps, clean SIGINT
  shutdown on both, no `ErrBusy` once test-harness zombie processes (an
  artifact of `go run` not forwarding signals to its compiled child — a test
  methodology bug, not a product one) were cleared. This is a real answer to
  open question 3 (bus bandwidth) for the two-process case, though the
  in-process multi-session case in D5 still needs its own check once built.
- **Visually confirmed by the operator**: each screen showed the correct
  module, on the correct physical unit, with no cross-wiring, while both ran
  at once.

This is real hardware confirming D1, D2, and D3 end to end — not just the
synthetic tests. The `Ambiguous` fallback (D2) and same-process multi-session
isolation (D5) remain unverified against real hardware: this rig has two
*different* device names, so the identical-name collision path never
triggers. That gap stays open until a second Push 3 (or second Push 2) is
available.

**D4 (identify), same session.** `cmd/identifytest` (new, mirrors
`cmd/frametest`'s throwaway-probe shape) exercises `Flash` and `FlashLEDs`
directly against real hardware, ahead of the pairing UI that will eventually
call them.

- Push 3: `Flash` (screen blink + centred "PUSH 3" label) and `FlashLEDs`
  (all 64 pads, palette index 21) run concurrently for 10s, both **visually
  confirmed correct on the physical Push 3**, clean clear-out on both after.
- Push 2: `Flash` confirmed correct. `FlashLEDs` on the User Port's out cable
  (#4) **lit nothing**; retried on the Live Port's out cable (#3) and it lit
  correctly. This is a real, previously-undocumented protocol fact, not a
  code bug — recorded in `docs/protocol/led-output.md`: **only the Live Port
  cable carries pad LED writes**, at least on Push 2. `FlashLEDs` itself
  needed no change — its real callers will always pass a `PortRef.IsLive`
  cable, including in the `Ambiguous` fallback case, where `Role` is already
  known to be `"Live"` even when *which unit* it belongs to is not — but this
  is worth remembering before ever pointing it at an arbitrary candidate
  cable during manual disambiguation.
- Not yet checked: whether Push 3's third (External) port also fails to carry
  LEDs, or whether that is a Push-2-only, two-port-device fact.

**Three units, two of them an identical pair — 2026-08-19, same session.**
A second Push 3 (serial `37479006`) was added alongside the first Push 3
(`37589789`) and the Push 2. First attempt showed only two devices —
`system_profiler SPUSBDataType` (macOS's own enumeration, unrelated to any of
our code) confirmed the second unit had not enumerated on the bus at all, a
pure hardware/power issue; after replugging into a different port,
`system_profiler` and `-devices` both saw all three.

This settled open question 2 for the one platform available to test
(**macOS/CoreMIDI does not disambiguate two identical Push 3 units**) and
validated the whole design chain built for exactly this case:

- `display.List()` had **no ambiguity at all** — both Push 3s report distinct
  real serials, so D1's selector design needed nothing further. Ambiguity is
  purely a MIDI-naming problem on this platform, confirming the split in the
  design (dual selectors for USB, a real fallback needed only for MIDI) was
  targeted correctly.
- `midi.ListUnits()` correctly triggered the `Ambiguous` fallback on **every
  cable** of both Push 3s (all six, Live/User/External × 2) — CoreMIDI hands
  back six byte-identical port names, three duplicated from each unit. No
  false ambiguity was flagged on the Push 2's distinctly-named cables.
- `pmidi.Open()` and `pmidi.OpenNamed("Ableton Push 3 Live Port")` both
  **refused with a clear error** rather than silently opening one of the two
  — confirmed live: `"Ableton Push 3 Live Port" matches 2 units — ambiguous,
  use OpenRef with a specific PortRef"`.
- **`identify.Flash` correctly distinguished the two visually-identical Push
  3s by serial** — flashing `serial:37479006` and `serial:37589789` each lit
  only their own physical unit, confirmed by the operator both times. This is
  the exact real-world scenario D1's serial selector exists for: two boxes a
  human cannot tell apart by looking, addressed correctly by software.
- **`identify.FlashLEDs` correctly distinguished the two ambiguous "Live
  Port" out cables by raw driver number** (`ListOutPortNames`, added this
  session to `internal/midi` since `groupPorts` never assigns a paired
  `OutNum` to an ambiguous ref — that number has to come from somewhere for a
  human to try candidates by hand). Out port `#0` and out port `#5` each lit
  a different, correct physical unit, confirmed by the operator. Notably,
  **the two out-port numbers did not follow USB bus/enumeration order**
  (`#0` mapped to the unit that was plugged in *first*, `#5` to the one
  plugged in *second* — or rather, to whichever the driver's own port
  numbering happened to assign; there was no way to predict it from the USB
  side). This is a real, live demonstration of why per-OS USB-location
  correlation was rejected in favour of manual pairing during planning: even
  on the one platform tested, there is no reliable cross-reference between a
  CoreMIDI port number and a USB bus position.

Net: the `Ambiguous` fallback and the two-directional identify design are no
longer hypotheses on macOS — both are hardware-confirmed against genuinely
identical units. Still open: the same check on Linux/ALSA and Windows/WinMM,
and Push 2 (or a second Push 2) has not been tested for the identical-pair
case at all.

**D5/D6 (`hostManager` multi-session + the pairing UI) — 2026-08-19, same
session.** Built `cmd/pushapp-ui/hostmanager.go`'s session map, `Overview`,
`ConnectRequest`, and the two-column pairing view in `frontend/`, then ran it
against the live two-unit rig (Push 3 + Push 2) through the actual built app,
not a probe. Four real bugs surfaced this way — none of them caught by the
Go unit tests, all of them the kind that only show up when two sessions
genuinely run at once:

1. **Shared module instances across sessions.** `main.go` built the module
   list once and handed the same slice — the same `*seq.Module`, literally —
   to every session's `bootstrap.Options.Modules`. Starting `seq` on a second
   unit froze the first unit's pad grid mid-sequence, because both Runtimes
   were driving one shared ticker and step counter. Fixed by turning
   `hostManager.baseOpts.Modules` into a `newModules func() []module.Module`
   factory, called fresh inside `connect()` — `main.go` now passes
   `availableModules` itself, not `availableModules()`. Regression-tested
   with a stateful fake module compared by pointer identity (a zero-size fake
   struct made the first version of this test pass for the wrong reason —
   Go's runtime aliases all zero-size allocations to `zerobase`, so the
   struct needed a real field before pointer comparison meant anything).
2. **This app's own name collided with Push detection.** `internal/midi`
   filtered ports with a bare `strings.Contains(name, "Push")`, and this
   app's virtual MIDI-out port is named `"Push Tethered App"`
   (`internal/midiout.DefaultName`) — so once a MIDI-out module activated,
   the app's own output port showed up in the pairing view's MIDI list as if
   it were a third Push unit. Fixed by matching `"Ableton Push"` instead —
   every real capture this repo has ever recorded starts with exactly that —
   via a new `isPushHardwareName` used everywhere `internal/midi` used to
   check for a bare `"Push"` substring.
3. **Two sessions collided on the default MIDI-out name.** Both defaulted to
   `midiout.DefaultName` ("Push Tethered App"), so the log showed two
   *different* sessions both opening `MIDI out: "Push Tethered App"
   (virtual)` — a DAW subscribing by that name would get an arbitrary one of
   the two. Fixed in `hostManager.connect`: when the base options leave
   `MIDIOutName` empty, sessions after the first get a numbered suffix
   (`"Push Tethered App 2"`, `"Push Tethered App 3"`, …), using the same
   counter as session keys so numbers never collide even across
   connect/disconnect cycles. An explicit `MIDIOutName` in `baseOpts` is
   still respected unchanged for every session — the fallback only fires for
   the empty default.
4. **LEDs stayed lit on quit.** `main.go` called `mgr.shutdownAll()` after
   `app.Run()` returned — but with
   `Mac.ApplicationShouldTerminateAfterLastWindowClosed: true`, closing the
   last window tears the process down through macOS's own termination
   sequence, and `app.Run()` never returns in time for that line to execute.
   Confirmed live: no LEDs cleared on quit, and the log never reached "host:
   all sessions shut down". Fixed by wiring `mgr.shutdownAll` into Wails'
   `Options.OnShutdown`, which the framework documents and guarantees runs
   synchronously as part of the real shutdown sequence regardless of what
   triggered it. The post-`Run()` call was kept as an idempotent fallback for
   whatever path reaches it with sessions still open (the headless
   `wails3 dev`/SIGINT path, primarily), rather than removed.

Also fixed along the way, not bugs but friction reported live: the window
was too small for the two-column pairing view (480×420 → 900×700, plan D6
never specified a size).

All four fixes are covered by new Go tests
(`TestConnectGivesEachSessionFreshModuleInstances`,
`TestIsPushHardwareNameExcludesOwnProductName` +
`TestIsPushHardwareNameMatchesRealNames`,
`TestConnectAssignsDistinctMIDIOutNames` +
`TestConnectRespectsExplicitMIDIOutName`) and by the live retest above; the
OnShutdown fix has no meaningful Go-level unit test (it is a wiring fact
about the Wails framework's lifecycle, not app logic) and stays verified by
the live retest alone — worth remembering if `main.go`'s window/lifecycle
setup is ever refactored, since nothing will fail red if this regresses.

## Why

Ableton Live cannot drive two Push 3 Controller / Push 2 units simultaneously.
That is a limit of Live's control-surface architecture, not of USB or MIDI —
nothing at the protocol level stops a host from owning several units. Since
`pushapp-ui` already lets the user pick a MIDI port, driving several units from
one host looked close. It is, but the blockers are all in this repo, and they
are about *device identity* rather than concurrency:

- `display.Open()` takes no arguments and claims whichever unit libusb happens
  to list first.
- The MIDI layer addresses ports by name, and a name is not a unique key when
  two identical devices are attached.

What is already fine: `host.Runtime` is per-instance with no package-level
state, and `core/gfx` plus `core/display.ToBGR565` are pure functions over an
`*image.NRGBA`. N runtimes in one process is not the hard part.

## Ground truth

Verified in code and in the dependency sources while planning. Several items
contradict what the docs and the code comments currently assume.

- **`ctx.OpenDeviceWithVIDPID` does not error on ambiguity.** gousb v1.1.3
  `usb.go:211-219`: with multiple matching devices "it will return one of them,
  picked arbitrarily". So today two Push 3s means a silent wrong-device claim,
  not a catchable error.
- **Name-based MIDI open cannot address the second unit at all.** gomidi
  v2.3.24 `drivers/port.go:139-160` matches with `strings.Contains` and returns
  the first hit. Ports must be opened by driver *number*
  (`gm.InPort(n)` / `gm.OutPort(n)`).
- **`rtmididrv.Driver` has no locking.** `driver.go:22-27` — `opened` is a
  slice appended on every open with its mutex commented out. N sessions plus a
  polling UI is a real data race.
- `findMatchingOutPort`'s positional pairing is wrong even for **one** Push if
  the in-cable count differs from the out-cable count, and unfixable for two.
- On Windows two identical units get identical port names, so both
  `gm.FindInPort(name)` and the positional index silently take the first.

## Decisions

- **Manual pairing with visual identify**, not per-OS USB-location correlation.
  Windows exposes nothing usable, so auto-correlation would degrade to manual
  there anyway. Manual works identically on all three platforms.
- **In-process multi-session** (N `host.Runtime` in one `pushapp-ui`), not
  one process per unit. `cmd/pushapp` stays single-device but gains explicit
  selection.
- **Two selector forms, both required.** `serial:AB12CD34` survives replug and
  a UI restart but needs an open handle to read and may not be unique — Ableton
  may ship a constant string or none at all. `usb:3.7` (bus.address) needs no
  handle but changes on replug. gousb does not expose libusb's full port path,
  so there is no stable topology ID available. Design for "serial is not
  unique" from the start; that is why `usb:` exists rather than being a
  nicety.

## Phases

### D1 — device identity in `internal/display`

`internal/display/enumerate.go` (new) plus changes to `display.go`.

- `Info{Model, Serial, Bus, Address, Port, ID}`.
- `List() ([]Info, error)` — enumerate every Push, opening a handle per unit to
  read the serial string descriptor, then closing it. Never selects a
  configuration and never claims an interface, so it stays safe while Live owns
  the display and while another session drives the unit. Sorted by
  `(Bus, Address)` for stable UI ordering.
- `Open()` — behaviour unchanged (first unit, Push 3 preferred), now warning
  when more than one unit is present.
- `OpenID(sel)` — exact `serial:` match, then exact `usb:` match, then
  `ErrNotFound` wrapped with what *was* found. Never fuzzy: a wrong match here
  drives the wrong physical screen.
- `(*Device).Info()`.

**One shared refcounted `gousb.Context`.** `Context.Close()` errors while any
device is still open, and N contexts means N libusb event threads. Package-level
`acquireCtx()` / `releaseCtx()` with a refcount, acquired by `OpenID` and
`List`, released by `Device.Close()` after `dev.Close()`. This makes "close the
context with a device still open" structurally impossible.

**In-process claim registry.** `ErrAlreadyClaimed` plus a selector set that
`OpenID` inserts into and `Close` removes from. Without it, two sessions racing
for one unit surface a misleading `ErrBusy` ("Live?") for what is our own bug.

Preserve the `SetAutoDetach` comment verbatim through the refactor — refactors
are how such comments get lost.

### D2 — correct MIDI pairing in `internal/midi`

`internal/midi/ports.go` (new) plus changes to `midi.go`. Delete
`findMatchingOutPort`.

Group by physical unit, pair within the group, open by number:

- `PortRef{InName, OutName, InNum, OutNum, Unit, Cable, Role, Device, IsLive,
  Ambiguous}`.
- `ListUnits()` / `ListPortRefs()`; `ListInPorts()` kept unchanged for
  display-only use.
- `OpenRef(ref)` — opens by number, re-validating that the port at `InNum` still
  carries `InName` first, with a distinct error if not (the list changed under
  us, so opening anything would open the wrong device).
- `OpenNamed(name)` — resolves through `ListPortRefs`; now refuses when the name
  matches more than one unit instead of silently taking the first.
- `Open()` — keeps auto-detect, now refusing when two units are present, since
  there is no right answer.

The design is one pure function `groupPorts(ins, outs []portName) []PortRef`
over `unitKeyOf(name) (unit, role string, cable int)`, handling CoreMIDI/ALSA
role suffixes, the WinMM `MIDIIN2 (...)` / `MIDIOUT2 (...)` wrappers, and ALSA
client-number suffixes. Being pure, it is table-testable with zero hardware,
which is where most of this phase's confidence comes from.

**The `Ambiguous` fallback must exist before we measure anything.** When
`unitKeyOf` yields one unit key for what is clearly several boxes — repeated
cable indices, or more than one Live-role cable per key — stop grouping: emit
flat unpaired refs with `OutNum: -1` and the flag set, and let the UI demand an
explicit out-cable pick identified by flashing LEDs. Degrading to "ask the
user" is correct here; guessing is not. This is the difference between "works
on Windows" and "silently drives the wrong unit on Windows".

**Serialise driver access** via a shared mutex held across every entry into
`gm`/`drivers`, in both `internal/midi` and `internal/midiout`. Put it in a
small `internal/midilock` package rather than having `midiout` import `midi`
just for a lock.

### D3 — `bootstrap` takes a selector

`Options` gains `MIDIIn pmidi.PortRef` and `DisplaySel string`, keeping
`MIDIInName` as documented-legacy with precedence resolved in one commented
place. `Open` changes only its two open calls. The `ErrBusy` degrade is
unchanged; `display.ErrAlreadyClaimed` is added as a **hard** failure, since it
means the caller asked for a unit we already drive.

Two new multi-session collisions to guard: `CapturePath` (two sessions clobber
one file) and `MIDIOutName` (two sessions both defaulting create
identically-named CoreMIDI sources; on Windows both attach to the same loopMIDI
port and silently merge two modules' output).

### D4 — visual identify, in both directions

A display flash alone **cannot complete a pairing**: it maps a USB unit to a
physical box but says nothing about which MIDI port belongs to that box. Ship
both halves.

- `identify.Flash(ctx, sel, label, swatch, d, fps)` — claims via `OpenID`,
  refreshes continuously (a single frame vanishes; Push redraws its own idle
  screen when writes stop), then blanks and releases.
- `identify.FlashLEDs(ctx, ref, colour, d)` — lights a distinctive pad pattern
  on the candidate out cable. This is the **only** identify that works while
  Live holds the display, under `-no-display`, and in the `Ambiguous` Windows
  case.

Marker content: there is no scaled text in the stack (`core/gfx/text.Draw` is a
fixed 7px font; `plans/2026-08-18-frame-text-scale.md` is parked), so do not
attempt a giant digit. Fill the screen with the unit's assigned colour
alternating with black at ~2Hz and draw an ASCII label centred; the UI shows
the same colour as a swatch on that unit's row, so matching across the desk is
instant and needs no font work. Sanitize serials — strip `?`, control
characters, whitespace — and truncate before they reach `text.Draw`. 10-15fps
is plenty.

For an already-connected unit, do not claim the display twice: add
`(*Runtime).Identify(label, swatch, d)`, three mutex-guarded fields checked at
the top of `drawFrame`, painting into `r.img` instead of calling the module's
`Render`.

**Trap** if a `frameWriter` interface gets extracted for testing: `host.New` is
called with a possibly-nil `*display.Device` (nil on the `ErrBusy` degrade
path). A nil `*display.Device` inside a non-nil interface makes the
`r.dev == nil` check false and the next `WriteFrame` panic. `New` must
explicitly `if dev != nil { r.dev = dev }`.

### D5 — `hostManager` becomes multi-session

**Two key spaces.** A *session key* (opaque, monotonic `s1`, `s2` — immune to
port renumbering) is what `PushService` methods take. A *unit key* (display
selector, else the MIDI unit key) is what `lastErrs` and the pairing UI are
keyed by, because an error must outlive its session and stay attributable to a
physical box.

Extract a `session` struct; `hostManager` holds `sessions map[string]*session`,
an `order []string` for stable listing, `lastErrs` by unit key, identify
cancels by unit key, and an `open func(bootstrap.Options)` seam over
`bootstrap.Open` so the whole lifecycle is testable without hardware.

Semantics to preserve per session:

- The `if m.rt != nil` gate becomes **resource** dedup with two distinct errors
  ("that screen is already in use by session s1" vs "that MIDI port is…").
  "Already connected" disappears as a concept — it was never the real
  constraint.
- `rt.Activate(rt.List()[0].ID)`'s unchecked index becomes a checked lookup of
  `req.ModuleID` with a real error.
- `watch`'s pointer compare becomes `m.sessions[sess.key] != sess`, which is
  stricter, since keys cannot alias.
- `shutdownAll` must cancel every session context **and every in-flight
  identify** (each holds a display claim and would otherwise leave a marker
  frozen on screen), then wait on every `stopped` channel in parallel with a
  bounded timeout — because `rt.Shutdown()`, which clears the LEDs, runs inside
  `watch`, not inside `shutdown`. This is the one way this phase can break the
  clear-LEDs-on-every-exit-path rule.
- `connect` assigns a distinct ASCII `MIDIOutName` per session, and warns when
  two sessions resolve to the same attached out port.

`PushService` gains `APIVersion()`, `ListUnits`, `ListMIDIUnits`,
`IdentifyUnit`, `IdentifyMIDIPort`, `Connect(req)`, `Disconnect(key)`, and one
`Overview()` replacing `IsConnected` + `LastError` + `ListModules` — with N
sessions the current shape would be 1+N round trips per poll tick. Every module
method takes a session key.

Versioning: do not dual-maintain v1 shims — adding a parameter breaks the
generated TS bindings either way, and shims mean two paths where one is
untested. Make the break loud instead: `main.ts` checks `APIVersion()` against
a constant once and renders a "reload required" banner on mismatch. Regenerate
`frontend/bindings/` in the same commit or it goes silently stale.

Newly shared cross-session state: `module.Store` keys config by module ID only,
so two sessions running `seq` share one JSON file with last-writer-wins — for
BPM that may even be desirable. Leave it shared and documented rather than
inventing per-unit paths that would break existing users' files.
Install/uninstall mutate a shared directory; `Uninstall` must now refuse when
the module is active in **any** session. Cleanest is to move install/uninstall
off the session-scoped API entirely, since they are process-global filesystem
operations.

### D6 — frontend

`#connect-view` becomes a two-column pairing view (USB units with model,
serial tail, colour swatch, Identify; MIDI units with unit, Live cable,
Identify) and a "Pair and connect" button. `#module-list` becomes
`#session-list`, one card per session with its own module list, Identify,
Disconnect and status line. The module-level `busy` flag splits into a global
flag for connect/disconnect/install plus a set of busy session keys —
otherwise one session's activate freezes every other session's buttons.
`refresh()` collapses to one `Overview()` call. Per-unit errors render on that
unit's row, not in a global status line, which is unattributable with two
units.

### D7 — `cmd/pushapp` stays single-device, gains selection

`-midi-in` (closing a real gap — the Windows manual-pick escape hatch has no
CLI equivalent at all today), `-device serial:XXXX|usb:BUS.ADDR`, and
`-devices`, which runs before `bootstrap.Open` like `-install`/`-uninstall`,
prints `display.List()` and `pmidi.ListUnits()` including the `Ambiguous`
flag, claims nothing, and exits. That last one is what a user pastes into a bug
report.

Optionally in the same phase, a safety improvement disguised as a refactor:
migrate `cmd/frametest` onto `display.OpenID`. It currently carries its own
gousb code with a hardcoded display interface number — exactly the assumption
`findDisplayInterface`'s doc comment exists to prevent.

## Safety, per unit

Unchanged rules, now applied N times: claim only interface 0; never
`SetAutoDetach(true)`; never write to `xPort`; ASCII only when drawing. The one
that needs active attention is **clear LEDs on every exit path, including
SIGINT — now for every unit**, which is `shutdownAll`'s bounded parallel wait
above. Write it as an explicit checklist item, not an implied consequence.

Worth adding while here: a test for `findDisplayInterface`'s untested "two
vendor-specific candidates, neither named Display → refuse" branch. That branch
is the guard against ever claiming `xPort`.

## Verification

Zero hardware — where most of the confidence comes from:
`groupPorts`/`unitKeyOf` table tests over recorded port-name lists (CoreMIDI
single, ALSA single, WinMM single from `docs/platform/windows.md`, plus
synthesised double-unit sets per OS, named as *hypotheses* so a mismatch reads
as "our guess was wrong" rather than "the code broke"); selector round-trips
and `List` sort order behind a `var enumerate = func()…` seam (gousb's
`fakelibusb` is unexported); the `hostManager` lifecycle through the `open`
seam with `internal/module/moduletest` fakes; `paintMarker` pixel assertions.

`cmd/pushapp-ui` is a separate Go module and CI only *builds* it — add a
`go test ./...` step with `working-directory: cmd/pushapp-ui` or the D5 tests
rot.

One unit: `-devices` (does Push report a serial, in what format), `-device` and
`-midi-in` end to end, both identify paths eyeballed, screen-reclaim latency,
SIGINT leaves no LED lit.

Two units: serial **uniqueness**; two-unit port names on all three OSes; both
sessions driving distinct modules; unplugging one leaves the other running;
SIGINT clears LEDs on both; and sustained bandwidth — two units at 30fps is
~18.4 MB/s of bulk OUT against ~40 MB/s practical per USB 2.0 bus, tight if
both sit on one hub while Push 3 also streams audio. Measure achieved fps per
session and be ready to add a per-session FPS default or an aggregate cap.

## Open questions

1. **Does Push report a unique serial?** If not, pairings do not survive a
   replug and the UI must say so. Everything in D1 is shaped to survive a "no"
   — do not drop the `usb:` form on the assumption of a "yes".
2. **How does each OS name two identical units' MIDI ports?** Decides whether
   D2's `Ambiguous` path is the common case on Windows or a rare one. Cannot be
   inferred, only measured.
3. **Does one USB bus sustain two 30fps display streams?**
4. **How long after the last write does Push reclaim the screen?** Sets
   identify's minimum fps and finally quantifies the "refresh continuously"
   rule.
5. **Should `module.Store` be per-unit or shared?** Shared and documented for
   now. Changing it later is a config migration; changing it now is a decision
   no user asked for.
