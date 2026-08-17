# Open questions — what's unresolved, unmeasured, and worth refactoring

**Purpose:** `docs/archive/feasibility.md` is now frozen (§ numbers below refer
to it, but see CLAUDE.md's doc-sync rule — never edit that file). This doc
tracks **what's still open** since that snapshot, so it doesn't get buried in a
document that's mostly settled history.

Update this doc, don't let it drift — when an item here gets resolved, fold the
finding into CLAUDE.md/README (the living reference) and delete the entry here.
Never fold anything back into `docs/archive/feasibility.md` itself.

---

## 1. Blocking the next phase of work

- **Windows has never touched real hardware.** CI proves the binary compiles on
  `windows-latest`; nothing is known about the actual WinUSB/Zadig driver
  conflict (§4.3), or whether Push advertises WCID/MS OS descriptors (which
  would sidestep it). This is now the **only** remaining Windows unknown — the
  virtual-MIDI half is settled (below).
- **Whether Wails v3 survives the headless requirement.** A Pi 4/5 running one
  module in kiosk mode should not need `webkit2gtk`. The plan keeps a
  `-module <id>` flag that runs with no window at all, but if the UI and the
  headless path drift apart, Fyne/Gio (already the documented fallback) needs
  revisiting.
- **CI does not build or test `cmd/pushapp-ui` at all.** Confirmed safe to
  merge as-is (it's a separate Go module, invisible to root's `./...`), but
  nothing currently catches a regression there before it reaches `main`. See
  `plans/2026-08-17-ci-for-pushapp-ui.md`.

## 2. Needs discovery/exploration

- **Live's host→device configuration is invisible on macOS, and capturing it
  needs Live running somewhere — there's no way around that.** §11.2 captured
  Push's *replies* over CoreMIDI, but CoreMIDI can't see host→device SysEx, and
  those commands only exist at all while Live is actually sending them. `usbmon`
  itself is Linux-only tooling and **Live has no native Linux build**, so
  "capture with usbmon" and "run Live" can't happen on the same box. The real
  next step is a USB-level capture on whichever OS Live actually runs on —
  Wireshark + USBPcap on Windows is the more realistic path than anything
  Linux-side. Still not done.
- **Raspberry Pi 4/5 — zero hardware testing.** `plans/2026-08-17-raspberry-pi-support.md`
  lists five concrete unknowns (Go toolchain version on Pi OS, 64-bit vs
  32-bit, sustained frame rate on a weaker CPU, USB controller behavior, power
  margin) and a build recipe, but nothing has been run on an actual Pi yet.
- **LED contention when Live and `pushapp` both hold the device at once —
  newly found 2026-08-17, needs follow-up.** Two scenarios tested:
  - Live launched first, then `pushapp` run: claim fails cleanly with
    `ErrBusy` exactly as §4.1 predicts, Live keeps the display. This part of
    the degrade path is now confirmed working (previously untested per §9.3).
  - `pushapp` run first, then Live launched: `pushapp` keeps the display (first
    claimant wins, screen exclusivity doesn't depend on launch order) — but
    `pushapp`'s on-screen pad-mirror grid started reflecting Live's Session
    View pad colouring. Live is evidently still sending pad-LED MIDI even
    though it doesn't own the display, and since co-existence mode leaves
    Push's MIDI interface bound to the OS driver, both processes end up
    driving the same physical pad LEDs at once. **Unresolved:** does
    `pushapp`'s own LED writes fight visibly with Live's (flicker,
    last-writer-wins), and should `pushapp` detect this and back off its own
    LED output when it notices another client is driving them?
- **What triggers the MPE on/off split (§9.5) and the User Port mirroring
  (§11.1)?** Both flip between sessions with nothing deliberately changed
  between them, and both may share one root cause per §11.1's own suspicion.
  Needs a controlled A/B — e.g. power-cycle vs. reconnect-only vs. port-open-order
  — to isolate the variable.

## 3. Genuinely unknown / partially theorized

- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, present on
  Push 3 only, undocumented. "x" plausibly XMOS. Do not probe it speculatively
  (USB safety rule in CLAUDE.md) — this is a "wait for a lead" unknown, not a
  "go measure it" one.
- **Endpoint `0x81` IN on the display interface** — never read from. Possibly a
  status/ack channel.
- **`User Port` / `External Port` roles — plausible theory, not yet confirmed.**
  Working theory: `External Port` corresponds to Push 3 standalone's physical
  MIDI DIN connector on the back; `User Port` is active when Push's own "User
  Mode" is engaged on the device. Consistent with §11.1's "sometimes mirrors
  Live Port, sometimes near-silent" observation (User Mode is presumably off
  most of the time), but not measured — worth confirming by toggling User Mode
  deliberately and watching which port lights up.
- **Whether MPE can be disabled via SysEx** — unmeasured on either device.
- **Button-LED brightness fidelity and exact palette-index-to-colour accuracy**
  (§8.9) — sent without errors, never visually confirmed beyond the pad sweep.
- **Multi-hour endurance** — longest continuous run is 7 minutes (§12.4, up from
  24 seconds in §9.3). Nothing is known about drift, leaks, or thermal behavior
  over hours.
- **`ToBGR565`'s per-pixel `image.At()` cost on weaker hardware** (§9.3) — only
  relevant for holding a clean 30fps (60fps is not a goal, so that part of the
  original question is dropped). Directly relevant to the Raspberry Pi question
  above — measure there rather than assume.

## 4. Refactors / improvements — this repo and `ableton-push-hack`

- **`core/` geometry refactor — done 2026-08-17.** Push-2-identical geometry
  constants moved from `core/push3/geometry.go` to the new
  `core/display/geometry.go`; `core/push3` now re-exports them, so every
  existing `push3.VisW`/`VisH`/`Stride`/`FrameBytes`/`TotalBytes` caller in
  both repos kept working unchanged. `core/display` no longer imports
  `core/push3` — the dependency now runs `push3` → `display`. Verified: `core`
  module tests pass; `hacks/push-manager`, `hacks/keyboard-visualizer`,
  `hacks/automation`, and this app all build clean against it.
- **`core/display` gaining a `Writer` seam is still just a proposal, not done.**
  Something that accepts `ToBGR565` output and puts it on a panel — existing
  `Shm` becomes one implementation, a tethered USB writer becomes the second —
  is what would let a Shadow-UI panel render identically on standalone Push 3
  and tethered Push 2 with no panel code changed. Not worth doing until a
  second consumer actually needs it.
- **Touch-note numbers, the jog-wheel `IsEncoderCC` gap, and the encoder-CW/CCW
  prose are all already fixed upstream — confirmed 2026-08-17, nothing left to
  do.** `core/push3` in `ableton-push-hack` already carries the corrected touch
  notes and `IsEncoderCC(70) == true`; `internal/pushmap` here no longer
  overrides touch notes, only Push 2's deltas remain. The one remaining loose
  end — two stale inline comments in `core/push3/buttons.go` still saying
  "127=CW, 1=CCW" next to already-correct code — has been fixed too.
- **No `core/push2` package needed, and none is planned.** `ableton-push-hack`
  targets Push 3 standalone only; Push 2 support belongs to this repo, and
  `internal/pushmap/push2.go` is exactly where it should live.
- **CI arm64 Linux runner** — not added. Worth adding only after a Raspberry Pi
  build is manually confirmed once (per the Pi plan's own sequencing).
