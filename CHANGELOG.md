# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) (pre-1.0: expect breaking changes
between minor versions).

## [Unreleased]

### Changed

- CI now packages `pushapp-ui` into a normal, self-contained app per OS,
  instead of a bare executable: a `.dmg` on macOS, an NSIS installer on
  Windows, and an AppImage on Linux. Each one bundles libusb, so a
  release download runs with no separate install step. This also fixes
  the macOS release build, which failed to start on a machine with no
  Homebrew libusb installed (`Library not loaded:
  /opt/homebrew/opt/libusb/lib/libusb-1.0.0.dylib`). See
  [docs/platform/macos.md](docs/platform/macos.md) and
  [docs/platform/windows.md](docs/platform/windows.md).

## [0.2.0-alpha] - 2026-08-29

### Changed

- `examples/modules/knobs-js` is no longer bundled in-tree — it's the
  catalog's first real entry, published at
  [federico-pepe/pta-module-knobs](https://github.com/federico-pepe/pta-module-knobs)
  and installable via `-catalog-install knobs-js`. Proves the catalog
  flow end-to-end against a real external repo and release, not just
  local fixtures.

### Added

- **Module catalog and archive install.** `-install` now accepts a
  `.tar.gz`/`.tgz` archive in addition to a plain directory. New
  `-catalog-list`, `-catalog-install <id>`, `-catalog-check-updates`, and
  `-catalog-update <id>` flags (and matching `pushapp-ui` "Browse
  catalog…" button, plus an "update available" badge on installed
  modules) fetch a hosted `catalog/catalog.json` index, resolve a
  module's latest GitHub release, and download/install it — no manual
  clone-and-point-at-a-folder needed. Works the same for Python and
  Node.js process modules. See `internal/catalog`,
  `internal/archiveutil`, `internal/host/procmod/archive.go`, and
  [catalog/schema.md](catalog/schema.md) for the entry format and
  publishing steps. No checksum/signing verification — an installed
  module runs arbitrary code regardless of where it came from, same as
  a manual install.
- **Process modules can now be compiled binaries (Go, Rust, ...), not
  just scripts.** `manifest.json`'s new `exec_platforms` field maps
  `"GOOS/GOARCH"` to a per-platform exec command, so one catalog entry
  and one release archive can bundle a binary per target; a plain `exec`
  still works unchanged for Python/Node.js modules. See
  `Manifest.ResolvedExec` in `internal/host/procmod/manifest.go` and
  [docs/architecture/process-modules.md](docs/architecture/process-modules.md#compiled-non-script-modules-exec_platforms).

## [0.1.4-alpha] - 2026-08-28

### Changed

- CI: `build.yml`'s standalone `build-pi` job is folded into the main
  `build` matrix as an `ubuntu-24.04-arm` entry, and now also builds
  `cmd/pushapp-ui` for that target (uploaded as `pushapp-ui-ubuntu-24.04-arm`),
  which the old job skipped entirely. Also fixes a latent arch-collision bug
  in the wails3 CLI cache key surfaced by adding this entry: it was keyed on
  `runner.os` alone, and `ubuntu-24.04-arm` reports `Linux` the same as
  `ubuntu-latest` despite being aarch64 — an incompatible cached binary could
  have been restored into the wrong job depending on run order. Now keyed on
  `runner.arch` too.

### Fixed

- **Windows: MIDI output (and any `NeedsMIDIIn` module's external input)
  never actually worked**, even after correctly naming a loopback port to
  match `midiout.DefaultName`/`midiin.DefaultName`. Two independent bugs,
  found together while diagnosing a live Windows-on-ARM report:
  - RtMidi's Windows backend reports its "cannot create a virtual port"
    refusal as a C++ warning, not a thrown exception, so
    `rtmididrv.OpenVirtualOut`/`OpenVirtualIn` returned `err == nil` on
    Windows while creating nothing. `internal/midiout`/`internal/midiin`
    trusted that return value, so the fallback to attaching an existing
    named port never ran — the app logged `(virtual)` mode and reported
    success while every byte went nowhere. Fixed by skipping the
    virtual-port attempt outright on `runtime.GOOS == "windows"`.
  - `isPush()` in both packages matched any port name containing the bare
    substring `"push"` — which includes the app's own default port names,
    `Push Tethered App` and `Push Tethered App In`. This made the intended
    Windows workaround (name a loopback port to match the default)
    structurally impossible: `attach()`'s own anti-feedback-loop guard
    rejected the very port it was supposed to find. Fixed to match the
    real hardware marker, `"Ableton Push"`, mirroring `internal/midi`'s
    identical fix from 2026-08-19.

  Confirmed working end-to-end 2026-08-27 on real Push 3 hardware
  (Windows on ARM), including against a Windows MIDI Services (MIDI 2.0
  preview SDK) loopback endpoint pair — confirmed to bridge correctly
  into the legacy WinMM API this app uses, making it a loopMIDI substitute
  on ARM64, where loopMIDI has no build. See
  [docs/platform/windows.md](docs/platform/windows.md#midi-output).
- `pushapp-ui`'s log file (`<UserConfigDir>/push-tethered-app/logs/pushapp-ui.log`)
  stayed empty on every Windows double-click launch: `os.Stderr`, listed
  first in the `io.MultiWriter` given to `log.SetOutput`, has an invalid
  handle on the `-H windowsgui`-linked production build with no console,
  and `io.MultiWriter` aborts on a writer's first error without writing to
  the rest. Fixed by writing to the log file first. See
  [docs/guides/debugging.md](docs/guides/debugging.md#pushapp-ui-log-file).
- `modules/seq`: a single failed `SendNote`/`NoteOff` call (e.g. one made
  fractionally too early, before the OS finished registering a
  just-attached output port) latched the pad-grid's error banner
  ("send error: ...") permanently for the rest of the session, even once
  every subsequent send succeeded. `m.lastErr` now clears on the next
  successful send/release.
- CI: the `pushapp-ui` Windows build artifact never included
  `libusb-1.0.dll`, even though the build step copies it next to the exe —
  the upload step's file list only named the exe. Every downloaded Windows
  build failed to launch with no visible error (a `windowsgui` binary has
  no console to show one) until a user copied the DLL in by hand.

### Added

- Push 3 External Port MIDI routing: `NeedsMIDIIn`/`NeedsMIDIOut` modules
  can now be pointed at Push 3's own MIDI DIN jacks instead of the app's
  virtual loopback port, via `bootstrap.Options.ExtMIDIInFromPushExternal`/
  `ExtMIDIOutToPushExternal` (`internal/midiin`/`midiout.OpenExisting`).
  `cmd/pushapp` gets `-ext-port-in`/`-ext-port-out`; `pushapp-ui`'s pairing
  form gets matching checkboxes, greyed out for a Push 2. Confirmed live on
  Push 3 hardware with `beatcount` (in) and `seq` (out). External Port is
  no longer offered as a pairable cable in the pairing list — it never
  carried control-surface traffic — see
  [docs/architecture/module-host.md](docs/architecture/module-host.md#routing-through-push-3s-external-port-instead).

## [0.1.3-alpha] - 2026-08-26

### Added

- Live HTTP/MJPEG screen mirror (`internal/mirror`): taps the same render
  output `internal/capture` already produces (no extra USB traffic), streams
  to any number of browser clients instead of a file, at zero cost when
  nobody's watching. `pushapp` gets `-mirror-addr` (on by default at
  `localhost:3000`, pass `-mirror-addr=""` to disable); `pushapp-ui` serves
  every session's mirror unconditionally at `localhost:3000/screen/<key>`,
  with a "Live screen" toggle and "Open in browser" button
  (`PushService.OpenMirror`) per session card.
- `pushapp-ui`: a Settings panel — once at least one device is connected,
  the pairing UI moves out of the main window into a Settings… overlay
  instead of permanently eating space. Session cards gain a collapse
  triangle for their module list, useful with several units connected.
- `internal/applog`: shared timestamped log output (with level) and a
  startup banner for both `pushapp` and `pushapp-ui`, plus session-tagged
  connect/disconnect lines and `Errorf`/`Fatalf` helpers so a dropped
  session or fatal exit is tagged error, not info.
- `modules/padpointer`: pad-grid-driven pointer — pad row + Channel
  Pressure drive a menu page (press-to-click) and a crosshair page (full
  grid position via MPE slide/bend when available, coarse per-cell
  fallback otherwise; a firm press triggers a ring animation). Works the
  same on Push 2 and 3, no MPE dependency required.
- `knobs-js`: PAN 1/2, two bipolar `KnobArc` knobs on -50..+50, using the
  new `Knob.Bipolar` (`ableton-push-hack`) so the resting value renders as
  an empty ring instead of a half-full one.
- `cmd/xporttest`: a read-only, marker-aligned capture/analysis tool for
  Push 3's undocumented `xPort` interface (interface 6) — confirmed a
  per-pad touch correlation (two different pads lit up two different byte
  offsets in the 136-byte frame, 100% consistent across independent
  touch/release toggles). See `docs/protocol/xport.md`; the full pad map
  and pressure-scaling are parked as a stretch goal.

### Fixed

- `internal/midi`: Channel Pressure was silently dropped on MIDI channel 1
  (only channels 2-16 were decoded) — Push 3's pads land on channel 1 when
  MPE (Aftertouch mode) is off, not always on an MPE member channel as
  previously assumed.
- Module `Store` config files are now namespaced per-device (by
  `display.Info.ID` — serial, or `usb:BUS.ADDR` when a unit reports no
  serial), not just per-module. Running the same module against two Push
  units no longer has both sessions read/write one shared JSON file,
  last-writer-wins.
- MPE on/off resolved as a persistent Push 3 setting (Aftertouch mode, in
  Push's own settings menu), independent of Live's presence — not the
  protocol-layer handshake this was previously assumed to be.
  `docs/protocol/midi-input.md` corrected to no longer claim MPE is always
  on.

### Changed

- `pushapp`'s default mirror port moves to 3000 (7000/5000 collide with
  macOS's AirPlay Receiver); the mirror is now on by default rather than
  opt-in.
- `modules/paddebug` removed — its diagnostic job (finding the channel-1
  Channel Pressure bug and narrowing down the MPE trigger) is done.

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

[Unreleased]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.3-alpha...HEAD
[0.1.3-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.2-alpha...v0.1.3-alpha
[0.1.2-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.1-alpha...v0.1.2-alpha
[0.1.1-alpha]: https://github.com/federico-pepe/push-tethered-app/compare/v0.1.0-alpha...v0.1.1-alpha
[0.1.0-alpha]: https://github.com/federico-pepe/push-tethered-app/releases/tag/v0.1.0-alpha
