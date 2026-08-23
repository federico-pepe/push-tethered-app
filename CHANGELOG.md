# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) (pre-1.0: expect breaking changes
between minor versions).

## [Unreleased]

## [0.1.2-alpha] - 2026-08-23

### Added

- `Frame.KnobArc` / op `"knobarc"`: a third knob composition alongside
  `Knob`/`KnobFull` — a 300° gauge arc from 7 o'clock (min) through 12 to
  5 o'clock (max), 60° gap at the bottom. Same `Knob` param shape as the
  other two. See [docs/architecture/design-system.md](docs/architecture/design-system.md).
- `Knob.Color`: assign any color (e.g. a `push3.Palette` entry) to an
  individual knob's fill/pointer instead of every knob sharing the host
  Theme's `Select` color. Unset falls back to `Theme.Select` as before —
  it does not default to white, since white is a valid `Color` choice in
  its own right. Applies to `Knob`, `KnobFull`, `KnobArc`, and `Fader`
  (all four share the `Knob` param type).
- `SoftButton.Color`: assign any color to an individual soft-button's
  label, overriding whatever `State` would otherwise pick
  (`White`/`OnColor`/`OffColor`). Unset falls back to `State`'s own
  default, not white, same reasoning as `Knob.Color`.
- Documented a standing invariant for `core/gfx/widgets` (in
  `ableton-push-hack`'s package doc, `theme.go`): every color-bearing
  widget, existing or future, must support the full Push palette as a
  real parameter, with a sensible fallback when unset — not hardcode a
  color or leave it a per-widget afterthought. `DrawFader` and
  `SoftButton` were the two gaps found auditing against this rule (both
  fixed to match — see the `Fader`/`SoftButton` entries above).
- `modules/uidemo`'s buttons page now also exercises `SoftButton.Color`
  (an 8th button, PINK) alongside the existing exclusive/independent
  groups, so every design-system color path is verified on real hardware,
  not just in `cmd/screensim`.
- `cmd/genpalette`: writes `core/push3.Palette` out as `palette.json` into
  every `examples/modules/*` directory that has a `manifest.json` —
  `byIndex[0..127]` (every raw hardware index, pre-resolved the same way
  `push3.ColorForIndex` does) and `byName` (the ~90 named entries). A
  process module (JS, Python, any language) can't import the Go `push3`
  package, so this is how it looks up a real palette color instead of
  hand-copying RGB. `go run ./cmd/genpalette` from the repo root
  regenerates all five example copies after any `push3.Palette` change
  (rare). See
  [writing-a-process-module.md](docs/guides/writing-a-process-module.md#colors).

### Fixed

- `examples/modules/{hello,beatcount}-{py,js}` and `knobs-js` had raw RGB
  literals with no traceable palette source — none imported anything from
  `core/push3` because a process module can't. All five now load
  `palette.json` (see `cmd/genpalette` above) and look up colors by name
  (`paletteColor`/`palette_color`) or by raw index
  (`paletteById`/`palette_by_id`) instead. `beatcount-{py,js}`'s
  status-text gray, which used to be `{120,120,120}` and matched no
  `push3.Palette` entry at all, is now `gray_green` — the same RGB
  `widgets.Default.Gray` resolves to on the Go side.

### Changed

- Design system visual polish: `widgets.Default`/`widgets.groupColors`
  (in `ableton-push-hack`'s `core/gfx/widgets`) now resolve every color
  through `push3.Palette`/`ColorForIndex` instead of raw RGB literals; a
  module `Frame` op's unset color field now renders white instead of
  invisible transparent black; `DrawArc`/`drawLine` are anti-aliased by
  default (so `DrawKnob`, `DrawKnobFull`, and `DrawEnvelope` all render
  smoothly), and `DrawKnob`/`DrawKnobFull` draw a 2px stroke instead of
  1px. See [docs/architecture/design-system.md](docs/architecture/design-system.md).
- Endless-encoder handling in `modules/uidemo` and `modules/ui-text-demo`
  now clamps at the accumulator (`push3.ClampInt`) instead of wrapping —
  turning past a bounded control's limit stops there and reverses
  immediately, rather than rolling back around to the minimum.

## [0.1.1-alpha] - 2026-08-20

### Added

- Confirmed Push's own **User Mode** as a working full co-existence mechanism
  with Ableton Live — both pad MIDI input and pad LED output are exclusively
  routed to Push's User Port while it's engaged, cutting Live off from the pads
  entirely while Live keeps running normally. Documented end to end in
  `MANUAL.md`, including the exact pairing order that makes it work.
- MIDI clock/transport output (`Host.SendClock`/`SendStart`/`SendContinue`/`SendStop`)
  alongside the existing `SendCC`/`SendNote`/`NoteOff`.
- External MIDI input (`internal/midiin`, `module.ExternalMIDI`, `Meta.NeedsMIDIIn`,
  `-ext-midi-in`/`-no-ext-midi-in`): a module can now receive raw MIDI from other
  software or hardware — an external clock to sync to, or any other message a
  module chooses to decode — mirroring the existing MIDI-out port. Unlike MIDI-out,
  a missing input port never blocks activation. Available to process-loaded
  (Python/JS) modules too, via `manifest.json`'s `needs_midi_in` and the
  `send_clock`/`send_start`/`send_continue`/`send_stop` RPC methods.
- `modules/seq` follows an external MIDI clock when one is connected (Start/Stop/Continue
  respected, falls back to its own wall-clock timing within 2s of the clock
  stopping), instead of only ever running on its own tempo.
- `modules/beatcount`: a minimal reference module for `NeedsMIDIIn` — counts an
  external MIDI clock and draws the current beat (1-4) across the pad grid as a
  digit, plus on screen. Not an instrument, a small example to read start to
  finish. Ported to `examples/modules/beatcount-py` and `beatcount-js`, the
  first process-loaded examples to use `needs_midi_in` and the `external_midi`
  event kind — including the base64-vs-array wire detail that trips up a
  `[]byte` field crossing into JSON.
- `pushapp-ui`'s pairing view now lists every MIDI cable (Live, User, External),
  not just Live — needed to select User Port for running alongside Ableton Live
  with Push's own User Mode engaged.
- `MANUAL.md`: end-user manual (pairing, MIDI port roles, running alongside Live,
  troubleshooting), split out from developer-facing `docs/`.

## [0.1.0-alpha] - 2026-08-20

Pre-alpha baseline: hardware confirmed on Push 2 and Push 3, module host
running, tag/release process introduced. macOS `.app` bundle now ships
libusb alongside the binary (no Homebrew required on the end-user machine);
CI's `build.yml`/`diagnostics.yml` cache apt packages, npm modules, and the
MSYS2 toolchain to cut Actions minutes on the free tier.

[Unreleased]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.2-alpha...HEAD
[0.1.2-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.1-alpha...v0.1.2-alpha
[0.1.1-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.0-alpha...v0.1.1-alpha
[0.1.0-alpha]: https://github.com/federico-pepe/push-tethered-app/releases/tag/v0.1.0-alpha
