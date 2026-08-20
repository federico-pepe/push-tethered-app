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

## Genuinely unknown / partially theorized

- **`xPort` (interface 6)** — vendor-specific, 2 bulk endpoints, present on
  Push 3 only, undocumented. "x" plausibly XMOS. Do not probe it
  speculatively (USB safety rule in CLAUDE.md) — this is a "wait for a lead"
  unknown, not a "go measure it" one.
- **Endpoint `0x81` IN on the display interface** — never read from. Possibly
  a status/ack channel.
- **`External Port` role** — plausible theory, not yet confirmed: Push 3
  standalone's physical MIDI DIN connector on the back. (`User Port`'s role
  is now confirmed — see
  [docs/protocol/midi-input.md](../docs/protocol/midi-input.md#user-modes-effect-on-routing).)
- **Whether MPE can be disabled via SysEx** — unmeasured on either device.
- **`internal/host/procmod`'s wire JSON field still says `{"brightness": ...}`**
  even though every Go-level `SetButton` (module, midi, host) is now named
  `value`, per the 2026-08-18 palette-index finding (see
  [docs/protocol/led-output.md](../docs/protocol/led-output.md#button-leds)).
  Renaming the wire field is a breaking protocol change for any existing
  process-loaded module and needs its own decision (alias old+new field?
  version bump?) before touching it.

## Refactors / improvements — this repo and `ableton-push-hack`

- **`core/display` gaining a `Writer` seam is still just a proposal, not
  done.** Something that accepts `ToBGR565` output and puts it on a panel —
  existing `Shm` becomes one implementation, a tethered USB writer becomes
  the second — is what would let a Shadow-UI panel render identically on
  standalone Push 3 and tethered Push 2 with no panel code changed. Not worth
  doing until a second consumer actually needs it.
