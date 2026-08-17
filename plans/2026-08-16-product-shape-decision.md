# Product shape decision

**Status: CLOSED 2026-08-17. Superseded by
[2026-08-17-module-host.md](2026-08-17-module-host.md) — read that instead.**

The answer was **none of A, B or C as written**: the product is a **module
host**, and the three candidates resolve into it as follows.

- **B (remapper) became a module**, not a product. The stated goal survives
  without the app being shaped around it.
- **A (Live companion) is dead.** No DAW coupling means the §4.1 screen
  exclusivity tension — the "fatal tension" flagged below — never arises.
- **C (creative surface) is what the host generalises**, and the overlap worry
  below is answered: the difference from `ableton-push-hack` is that anyone can
  write a module here, on Push 2 as well as Push 3, with no device hacking.

Two of the three "what I'd want answered" items were also resolved by the
decision rather than by measurement: Windows *is* in scope (question 2), because
"no DAW" removed the virtual-MIDI blocker — see the module-host plan for the
measured create-or-attach behaviour. Live's Extensions SDK (question 3) is moot.
Push 2 (question 5) is confirmed day-one and already works.

**Everything below is the original 2026-08-16 text, kept for the reasoning
trail. Do not plan against it.**

---

**Status: open.** Nothing should be built on top of `cmd/pushapp` until this is
settled — the three candidate products diverge sharply above the layer that
already exists.

## Why this can't be deferred

The stated goal is *"a fully configurable MIDI controller, independent of any
DAW."* Measurement on 2026-08-09 established that **co-existence mode cannot
remap MIDI** (§6.1a, §8.6): Push's MIDI lives on interface 5, bound to the OS
class driver, and that binding is exactly why the DAW can see Push's ports.
Claiming it to intercept and rewrite messages *is* full-ownership mode.

So co-existence is not a smaller version of the goal. It is a different product
that happens to share the display layer.

Everything built so far — `internal/display`, `internal/midi`, `internal/pushmap`,
the render loop — is **common to all three options below**. That is why the
decision can be made now without wasting work, and why it should be made before
the next feature.

## What is already true (don't re-litigate)

- Display, MIDI in and LED out all work on macOS in co-existence mode, zero
  extra software (§8, §9).
- Full ownership needs a **virtual MIDI port** so the DAW has something to talk
  to. Already solved on macOS (CoreMIDI virtual source, or the built-in IAC
  Driver) and Linux (ALSA seq — `core/alsaseq` already creates ports in the
  sibling project). **Unsolved on Windows.**
- Windows is doubly hard: no built-in virtual MIDI *and* libusb needs WinUSB,
  which Zadig installs by displacing Ableton's driver (§4.3, §6.2).
- Push 2 is a stated day-one goal and has **never been measured** on hardware.

## The axes

**1. Which mode is primary?**
Co-existence (screen + feedback, DAW keeps MIDI) vs. full ownership (we claim
MIDI, remap it, re-emit through a virtual port).

**2. What platforms does v1 commit to?**
macOS only / macOS + Linux / all three. Interacts with axis 1: full ownership on
Windows is the single hardest thing in the project.

**3. Is the app DAW-coupled?**
A screen showing *track names and device parameters* needs Live integration
(Extensions SDK, or a Remote Script over TCP as `hacks/browser-bridge` already
proves). A screen showing *its own state* needs nothing. This is independent of
axes 1 and 2 and is easy to conflate with them.

## The three candidate products

### A. Live companion
Co-existence + Live integration. Push keeps working normally with Live; the app
adds screens Live doesn't provide.

- **Needs:** Live-side bridge (Extensions SDK or Remote Script), a UI language
  for the screens.
- **Ships on:** macOS + Linux fully. Windows display-only, and only when Live
  isn't holding the screen.
- **Fatal tension:** §4.1 — if Live is running with Push as a control surface,
  **Live owns the display and we cannot have it**. The most useful version of
  this product is precisely the configuration where it cannot run. Would require
  users to deselect Push in Live's preferences, which also removes the Live
  integration that makes it worth having. **This tension needs resolving before
  A is viable at all.**

### B. Standalone configurable controller  ← the original stated goal
Full ownership. We claim Push's MIDI, apply a user-defined mapping, and emit to
the DAW through a virtual port.

- **Needs:** libusb MIDI backend (a second implementation in `internal/midi`),
  virtual port per OS, a mapping model, a config UI.
- **Ships on:** macOS + Linux. **Windows needs a decision of its own** —
  Windows MIDI Services (recent Win11 only), teVirtualMIDI (commercial driver),
  or "co-existence only on Windows".
- **Note:** the mapping engine in `hacks/push-manager/src/remap.go` already
  models src→out CC/Note with scaling and relative-encoder accumulation. Most of
  the engine exists.

### C. Creative surface
Neither DAW-coupled nor a remapper. Push becomes a screen + grid for your own
software: visualisers, sequencers, instruments.

- **Needs:** nothing beyond what exists today, plus whatever the app *does*.
- **Ships on:** all three, no virtual MIDI, no driver conflict.
- **Question it raises:** `ableton-push-hack` already does this on standalone
  Push 3. What tethered adds is **Push 2 support** and **no device hacking**.
  Is that enough to justify a second project?

## What I'd want answered before choosing

1. **Which do you actually want to use?** All three are buildable; only one is
   worth the months.
2. **Is Windows in scope for v1?** If yes, B needs its Windows answer up front.
   If Windows can be "co-existence only", B becomes much easier.
3. **Does Live's Extensions SDK expose track/device state usefully?** Determines
   whether A is a real option. There is an `ableton-extension-builder` skill
   available to check.
4. **Does A's screen-exclusivity tension have a way out?** If not, A is dead and
   the choice is B vs C.
5. **Push 2:** still a day-one goal, or quietly dropped? It changes the device
   abstraction and it has never been measured.

## Recommendation

**B, scoped to macOS + Linux, with Windows explicitly deferred to co-existence
or display-only.**

It is the stated goal, it is the only option that makes Push genuinely
DAW-independent, the virtual-port problem is already solved on both target
platforms, and the mapping engine largely exists upstream. A is blocked by a
constraint we measured and have no answer to. C is real but overlaps heavily
with a project that already exists.

The honest cost: B means committing to Windows being a second-class citizen for
a while, and saying so publicly rather than discovering it late.

## Next step once decided

If B: spike a **macOS CoreMIDI virtual source** and prove a mapped message
reaches Live through it. Small, and it retires the last unknown in the chain
before any mapping or UI work starts.
