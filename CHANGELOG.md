# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) (pre-1.0: expect breaking changes
between minor versions).

## [Unreleased]

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

[Unreleased]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.1-alpha...HEAD
[0.1.1-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.0-alpha...v0.1.1-alpha
[0.1.0-alpha]: https://github.com/federico-pepe/push-tethered-app/releases/tag/v0.1.0-alpha
