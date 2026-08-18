# Standing open items

**Status: open, tracking.** This is the living successor to
`docs/open-questions.md`, archived 2026-08-18 to
[docs/archive/open-questions.md](../docs/archive/open-questions.md) — that
file's content was a punch list of intent (what to measure, fix, or decide
next), which is what `plans/` is for, not `docs/`'s durable-reference role.
Read the archived file for the full reasoning trail behind each item below;
this file tracks only what's still actually open, as of 2026-08-18.

When an item here resolves, fold the finding into CLAUDE.md/README/the
relevant `docs/` page (the living reference) and delete the entry here —
same discipline the old file had. Do not fold anything back into
`docs/archive/open-questions.md` or `docs/archive/feasibility.md` — both are
frozen.

---

## Blocking / next up

- **No disconnect detection.** Unplugging Push mid-session leaves
  `cmd/pushapp-ui` reporting the last-active module ("Active: ...") against a
  dead port. `hostManager` has no watchdog on port health, and neither
  `internal/midi` nor `internal/host` currently notice a failed send or a
  dead input stream. Needs its own investigation (what actually fails first
  on unplug, on which OS, and how to detect it without polling every frame)
  before the UI can be trusted to reflect real connection state.
- **Whether Wails v3 survives the headless requirement.** A Pi running one
  module in kiosk mode should not need `webkit2gtk`. The plan keeps a
  `-module <id>` flag that runs with no window at all, but if the UI and the
  headless path drift apart, Fyne/Gio (already the documented fallback in
  `docs/archive/open-questions.md`) needs revisiting.

## Needs discovery/exploration

- **Live's host→device configuration is invisible on macOS, and capturing it
  needs Live running somewhere — there's no way around that.** CoreMIDI can't
  see host→device SysEx, and those commands only exist while Live is actually
  sending them. `usbmon` is Linux-only and Live has no native Linux build, so
  "capture with usbmon" and "run Live" can't happen on the same box. The real
  next step is a USB-level capture on whichever OS Live actually runs on —
  Wireshark + USBPcap on Windows is the more realistic path than anything
  Linux-side. Still not done.
- **LED contention when Live and `pushapp` both hold the device at once.**
  Two scenarios tested (2026-08-17):
  - Live launched first, then `pushapp` run: claim fails cleanly with
    `ErrBusy`, Live keeps the display. Confirmed working.
  - `pushapp` run first, then Live launched: `pushapp` keeps the display
    (first claimant wins, screen exclusivity doesn't depend on launch order)
    — but `pushapp`'s on-screen pad-mirror grid started reflecting Live's
    Session View pad colouring. Live is evidently still sending pad-LED MIDI
    even though it doesn't own the display, and since co-existence mode
    leaves Push's MIDI interface bound to the OS driver, both processes end
    up driving the same physical pad LEDs at once. **Unresolved:** does
    `pushapp`'s own LED writes fight visibly with Live's (flicker,
    last-writer-wins), and should `pushapp` detect this and back off its own
    LED output when it notices another client is driving them?
- **What triggers the MPE on/off split and the User Port mirroring?** Both
  flip between sessions with nothing deliberately changed between them, and
  both may share one root cause. Needs a controlled A/B — e.g. power-cycle
  vs. reconnect-only vs. port-open-order — to isolate the variable.

## Genuinely unknown / partially theorized

- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, present on
  Push 3 only, undocumented. "x" plausibly XMOS. Do not probe it
  speculatively (USB safety rule in CLAUDE.md) — this is a "wait for a lead"
  unknown, not a "go measure it" one.
- **Endpoint `0x81` IN on the display interface** — never read from. Possibly
  a status/ack channel.
- **`User Port` / `External Port` roles — plausible theory, not yet
  confirmed.** Working theory: `External Port` corresponds to Push 3
  standalone's physical MIDI DIN connector on the back; `User Port` is active
  when Push's own "User Mode" is engaged on the device. Consistent with the
  "sometimes mirrors Live Port, sometimes near-silent" observation (User Mode
  is presumably off most of the time), but not measured — worth confirming by
  toggling User Mode deliberately and watching which port lights up.
- **Whether MPE can be disabled via SysEx** — unmeasured on either device.
- **Button-LED brightness fidelity and exact palette-index-to-colour
  accuracy** — sent without errors, never visually confirmed. Note: this is a
  *separate* mechanism from pad LEDs — buttons use a 0-127 brightness scale
  (`Host.SetButton`, `internal/module/module.go`), not the pad note-colour
  palette (`core/push3/colors.go`'s `NamedColors`) that the 2026-08-17 pad-LED
  colour fix (commit `e0fc684`) corrected. That fix does not cover this item.
- **Multi-hour endurance** — longest continuous run is 7 minutes. Nothing is
  known about drift, leaks, or thermal behavior over hours.
- **Pi 4 specifically untested.** Pi 5 confirmed 2026-08-18 (see
  `plans/2026-08-17-raspberry-pi-support.md`); Pi 4 expected identical but not
  measured.

## Refactors / improvements — this repo and `ableton-push-hack`

- **`core/display` gaining a `Writer` seam is still just a proposal, not
  done.** Something that accepts `ToBGR565` output and puts it on a panel —
  existing `Shm` becomes one implementation, a tethered USB writer becomes
  the second — is what would let a Shadow-UI panel render identically on
  standalone Push 3 and tethered Push 2 with no panel code changed. Not worth
  doing until a second consumer actually needs it.
