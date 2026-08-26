# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repository. This is a
short agent manual. Durable reference lives in [docs/](docs/) — read it
first for anything beyond the safety rules and pointers below.

> **Doc sync rule:** Update this file, `README.md`, `MANUAL.md`, and
> `docs/` when a change matters to a future reader: new behavior, a changed
> protocol fact, a new API, or a resolved or newly found issue. Not every
> commit needs a doc update. Skip internal refactors and trivial edits.
> When you update docs, put the update in the same commit as the change.
>
> Put a new protocol fact in `docs/protocol/`. When you resolve an open
> question, fold it into the docs and delete it from
> [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md). Put
> anything an **end user** needs to operate or configure the app (pairing,
> port roles, running alongside Live, troubleshooting a specific error
> message) in [MANUAL.md](MANUAL.md), not in the UI itself. Keep the app's
> own screens short, and put explanations in the manual instead of inline
> text.
>
> **Plans live in `plans/`**, one file per plan, named
> `YYYY-MM-DD-name-of-the-plan.md`.
>
> - `docs/` holds durable reference (protocol facts, measurements,
>   rationale) for **contributors**.
> - `MANUAL.md` holds the same kind of durable truth, but written for
>   **end users**: how to use the app, not how it is built.
> - `plans/` holds intent: what we are about to do, why, and any open
>   decisions.
>
> When a plan is done, distill its settled contract into
> `docs/architecture/` (and into `MANUAL.md` if it changes how a user
> operates the app). Leave the plan itself as historical reasoning.
>
> **`docs/archive/` is frozen.** Never edit, move, or delete anything
> inside it. Never add to it unless a user explicitly asks.

## Project

`push-tethered-app` is a cross-platform desktop app. It owns an **Ableton
Push 2 or Push 3 in tethered (controller) mode**: display, pads, buttons,
encoders, and LEDs. **It is a module host.** `pushapp` owns the hardware
and runs **modules** — small programs that draw the screen and handle the
controls. You can write a module in Go or in any other language. No DAW is
involved at any layer.

**Status: pre-alpha.** The app runs, and is confirmed on Push 2 and Push 3
hardware, including pairing and driving two units at once from
`pushapp-ui` (macOS, Windows, and Linux/Raspberry Pi — see
[plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md)). For
the full picture (built-in modules, process-loaded modules,
`pushapp-ui`), see the root [README.md](README.md) and
[docs/README.md](docs/README.md).

Decision history:
[plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md)
(module-host shape) and
[plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md)
(any-language modules).
[plans/2026-08-16-product-shape-decision.md](plans/2026-08-16-product-shape-decision.md)
is **closed** — it is a reasoning trail only. Do not plan against it.

## Read this first

- [MANUAL.md](MANUAL.md) — end-user manual: pairing, port roles, running
  alongside Live, troubleshooting
- [docs/README.md](docs/README.md) — reading paths by task (write a
  module, build or contribute, protocol facts)
- [docs/protocol/usb-and-safety.md](docs/protocol/usb-and-safety.md) —
  read this before any hardware interaction
- [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md) — what
  is still unresolved
- [docs/archive/feasibility.md](docs/archive/feasibility.md) — frozen; the
  original protocol evidence and stack rationale (do not edit)

## Relationship to `ableton-push-hack`

This repo is a sibling of `~/Documents/GitHub/ableton-push-hack` (Push 3
*standalone*, deployed over SSH). `core/` is reused, not copied, through a
`replace` in `go.mod`. **Never fork or vendor it.** Fix issues upstream so
both projects benefit. That repo's hard safety rules (no `/boot`, `/opt`,
`/etc`) do not apply here, but see the USB safety rules below. Full
detail: [docs/hardware-reference.md](docs/hardware-reference.md).

## Non-negotiable safety rules

These rules also live in `docs/`, but they stay here too. An agent needs
them in context before it touches hardware, not one click away.

- **Claim only interface 0 (display).** Claiming MIDI or audio interfaces
  takes them away from the OS and the DAW.
- **Never write to `xPort` (interface 6) speculatively.** It is
  vendor-specific and undocumented.
- **Do no firmware operations, ever.** No DFU. No control transfers with
  unknown vendor requests.
- **Never do a blind "press every button" sweep.** Run `cmd/pushapp`
  first. Once a host drives the screen, the top-row buttons become plain
  MIDI, and you can press them safely. The leftmost button above the
  screen switches Push 3 into standalone mode, if the display is not
  already held.
- **Always clear LEDs on every exit path, including SIGINT.** A device
  left lit makes the next run ambiguous.
- **Never call `dev.SetAutoDetach(true)`.** This setting is config-wide,
  not interface-wide, and it tears audio and MIDI away from the OS class
  drivers.
- **Use ASCII only when drawing.** `core/gfx/text` renders any non-ASCII
  character as a missing-glyph box.

Full protocol detail (display format, MIDI decode order, MPE, LED
messages, button-sweep recovery): [docs/protocol/](docs/protocol/).

## Layout

```
cmd/pushapp/      the host: owns the hardware, runs one module. Wiring only.
cmd/pushapp-ui/   Wails v3 module switcher — SEPARATE Go module, see below.
cmd/probe/        USB descriptor dump (read-only, never opens the device)
cmd/frametest/    display-only probe, one frame or a timed hold
cmd/mapcheck/     cross-references captures against the button map
cmd/midiouttest/  MIDI-out probe: create/attach a port, send, and receive back
cmd/screensim/    renders named Frame/widget test scenes to PNG — no hardware,
                   no display claim; the fast-iteration tool for the design system
cmd/genpalette/   writes core/push3.Palette out as palette.json into every
                   examples/modules/* directory, so a process module in any
                   language can look up a palette color by name or 0-127 index
                   instead of hand-copying RGB — see writing-a-process-module.md
internal/applog/  shared log.SetOutput wrapper: timestamps every log.Printf line
                   (RFC3339-ish, microseconds, no zone) and a startup banner —
                   used by both cmd/pushapp and cmd/pushapp-ui
internal/bootstrap/  hardware-opening sequence shared by cmd/pushapp and -ui
internal/module/  the ABI: Module, Host, Frame/Op, Event, Meta, Store
internal/host/    runtime: registry, control API, event fan-out, frame loop
internal/host/procmod/       process-loaded module: JSON-over-stdio protocol
internal/renderframe/  the Frame/Op renderer itself (RegisterOp, Render, SupportedOps),
                   split out of internal/host so gousb-free tools like cmd/screensim
                   can import it
internal/display/ USB transport: claim interface 0, frame header, XOR, refresh
internal/midi/    OS MIDI in/out, event decoding, LED helpers
internal/midiout/ owns a named MIDI out port for modules (create or attach)
internal/midiin/  owns a named MIDI in port for modules (create or attach) — raw bytes only, no decoding
internal/mirror/  live HTTP/MJPEG screen mirror — taps the same render output as
                   internal/capture (no extra USB traffic) but fans out to any
                   number of browser clients instead of writing a file; a Hub
                   costs nothing when no client is connected
internal/pushmap/ Push 2 map deltas + shared CC/touch name tables
modules/monitor/  control-surface monitor; the reference module
modules/thru/     forwards pads/encoders/buttons out as MIDI
modules/seq/      8-step pad-grid sequencer; wall-clock-driven MIDI + Store
modules/remap/    user-editable overrides on top of thru's passthrough default
modules/beatcount/ minimal NeedsMIDIIn reference: counts an external MIDI clock
modules/uidemo/   every design-system widget, one page per cluster, driven by
                   real hardware controls — run this to verify the widget set
modules/ui-text-demo/ live font-tuning bench: encoders drive face/weight/size/
                   palette-color/margin — dial in a text-rendering change on
                   real hardware instead of guessing constants
modules/padpointer/ pad-grid-driven pointer: pad row moves a cursor onto an
                   8-item menu, Channel Pressure clicks; crosshair page adds
                   sub-pad XY via MPE slide/bend when the device's Aftertouch
                   mode is set to MPE (Polyphonic Aftertouch coarse-cell
                   fallback otherwise — see docs/protocol/midi-input.md's MPE section)
examples/modules/ process-loaded example modules (Python, Node.js)
tools/            macOS-only Swift probes (midimon, ledtest)
```

Full package-by-package rationale:
[docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

### `cmd/pushapp-ui` is a separate Go module — do not add it to root's `./...`

It has its own `go.mod`, with two `replace` directives (root repo,
`ableton-push-hack/core`). It needs `wails3` (the CLI) and Node/npm to
build. Its configuration lives in `build/config.yml` (v3, not v2's
`wails.json`). CI builds it on all three operating systems. Full detail:
[docs/guides/development-setup.md](docs/guides/development-setup.md).

## Writing a module

The contract is `internal/module.Module`: `Meta`, `Init`, `Handle`,
`Draw`, `Close`. `modules/monitor` is the reference implementation. A
module can also be **any executable, in any language** —
`internal/host/procmod` runs one as a child process over JSON-over-stdio.
`examples/modules/` has working Python and Node.js modules.

Guides: [writing-a-go-module.md](docs/guides/writing-a-go-module.md),
[writing-a-process-module.md](docs/guides/writing-a-process-module.md),
[writing-a-python-module.md](docs/guides/writing-a-python-module.md),
[writing-a-javascript-module.md](docs/guides/writing-a-javascript-module.md).
Architecture:
[architecture/module-host.md](docs/architecture/module-host.md),
[architecture/process-modules.md](docs/architecture/process-modules.md).

The drawing widget set (`core/gfx/widgets`, shared with
`ableton-push-hack`) is catalogued in
[docs/architecture/design-system.md](docs/architecture/design-system.md)
(widget-to-op-to-Frame-method map, how to preview with `cmd/screensim`).
Its design decisions live in `ableton-push-hack`'s `DESIGN.md`. Read that
before you add a new widget or drawing pattern. Design-system work in
progress:
[plans/2026-08-21-design-system-screensim.md](plans/2026-08-21-design-system-screensim.md).

```bash
go run ./cmd/pushapp -install path/to/your-module   # copies it in, registers it
go run ./cmd/pushapp -uninstall your-module-id
go run ./cmd/pushapp -list                          # shows installed too, [installed]
go run ./cmd/pushapp -module your-module-id
```

## Commands

```bash
go run ./cmd/pushapp            # host + first module
go run ./cmd/pushapp -list      # list compiled-in modules
go run ./cmd/pushapp -module monitor
go run ./cmd/probe        # dump USB descriptors
go run ./cmd/frametest    # claim interface 0, push one frame
go run ./cmd/midiouttest  # prove MIDI reaches other software
go run ./cmd/screensim -list-scenes         # design-system scenes, no hardware
go run ./cmd/screensim -scene <name> -out out.png
go run ./cmd/genpalette   # regenerate examples/modules/*/palette.json after a push3.Palette change
go build ./... && go vet ./... && go test ./...
```

`pushapp` flags:

- `-fps`, `-module <id>`, `-list`, `-no-display` (MIDI only), `-no-leds`,
  `-midi-out <name>`, `-no-midi-out`.
- `-ext-midi-in <name>`, `-no-ext-midi-in` — external MIDI input for
  modules that declare `NeedsMIDIIn` (`internal/midiin`). The module
  receives raw bytes as `module.ExternalMIDI`. Unlike MIDI-out, missing
  input never blocks activation. See `internal/module/module.go`'s
  `Meta.NeedsMIDIIn` doc.
- `-capture`, `-capture-raw`.
- `-mirror-addr <addr>` — serves a live MJPEG mirror of the screen at
  `http://<addr>/screen`. This is on by default at `localhost:3000`. Pass
  `-mirror-addr=""` to disable it. Avoid `:7000` and `:5000`; macOS's
  AirPlay Receiver uses both by default. See `internal/mirror`.
  `pushapp-ui` always serves the same way, at
  `localhost:3000/screen/<session key>`, with no flag needed — one Hub
  per session, not shared, because more than one Push can connect at
  once. `PushService.OpenMirror` opens a session's URL in the system
  browser through Wails' `Browser.OpenURL`.
- `-install <dir>`, `-uninstall <id>` — filesystem only; no Push needed.
- `-version` — prints `internal/version.Version`, or "dev" unless a build
  used the release workflow's `-ldflags`.
- `-devices` — lists every attached Push unit and MIDI cable. It claims
  nothing, and is safe to run with Live open. Paste its output into a bug
  report.
- `-device <serial:XXXX|usb:BUS.ADDR>` and `-midi-in <name>` — pick a
  specific unit or cable when more than one Push is attached. `pushapp`
  itself stays single-device. See
  [plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md)
  for `pushapp-ui`'s multi-session pairing instead.

```bash
cd cmd/pushapp-ui
wails3 dev              # hot-reload window; needs wails3 + Node/npm
wails3 build            # produces bin/pushapp-ui
```

Full flag reference and probe tools:
[docs/guides/debugging.md](docs/guides/debugging.md).

## Releases

This project uses Semantic Versioning. It is pre-1.0, so expect breaking
changes between minor versions: `vMAJOR.MINOR.PATCH[-alpha|-beta|-rc.N]`.
Current stage: `-alpha`.

Cutting a release:

```bash
git tag v0.1.1-alpha
git push origin v0.1.1-alpha
```

Pushing a `v*` tag triggers the `release` job in
`.github/workflows/build.yml`. The job waits on `build` and `build-pi`,
zips their artifacts, and publishes a GitHub Release for that tag through
`softprops/action-gh-release`. The job sets the pre-release flag
automatically for `-alpha`, `-beta`, and `-rc` tags.

Update [CHANGELOG.md](CHANGELOG.md) in the same commit as the tagged
code, under `## [Unreleased]`. When you tag the release, retitle that
section to the new version.

`build.yml` does **not** trigger automatically on PRs or plain pushes to
`main` (see the comment at its top). Only a manual `workflow_dispatch`
run or a `v*` tag runs CI. This project prefers that nothing runs without
an explicit request, partly to save free-tier Actions minutes.

## Cross-platform builds

**Do not cross-compile this app.** `gousb` (libusb) and `rtmididrv`
(vendored RtMidi C++) are both cgo. **Build natively on each target OS.**
`.github/workflows/build.yml` does this on real macOS, Linux, and Windows
runners. Per-OS setup: [docs/platform/macos.md](docs/platform/macos.md),
[docs/platform/linux.md](docs/platform/linux.md),
[docs/platform/windows.md](docs/platform/windows.md).

Wails' own [cross-platform build
guide](https://v3.wails.io/guides/build/cross-platform/) does not change
this. Its Docker cross-toolchain covers apps with no extra C
dependencies, or only the dependencies its image bundles. This app's
libusb dependency and its per-OS RtMidi backend (CoreMIDI, ALSA, WinMM)
would need a custom image that carries those for every target. Even then,
the runtime DLL problem (see the missing-DLL section of
`docs/platform/windows.md`) would stay unsolved, because it is a shipping
problem, independent of where you compiled the binary.

For a one-off diagnostic build on a platform with no local toolchain, and
with no release cut needed, use `.github/workflows/diagnostics.yml` (run
`gh workflow run diagnostics.yml`, or use the Actions tab). It builds
`probe`, `frametest`, `mapcheck`, `pushapp`, and `identifytest` natively
per OS, in about two minutes. This workflow is deliberately separate from
`build.yml`'s own disabled copy of that build (see the comment there). Do
not re-enable that copy instead.

## Architecture decisions already made

Rationale in [docs/archive/feasibility.md](docs/archive/feasibility.md)
§6 and
[docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

- **Go**, single static binary, for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Cost: no cross-compilation, and the
  LGPL-2.1 license.
- **`gitlab.com/gomidi/midi/v2` with `drivers/rtmididrv`** for OS MIDI.
  This avoids a brew or apt dependency on all three OSes. Do not add
  rtmidi or portmidi as system packages.
- **This app reads Push's MIDI through the OS, never through libusb.**
- **Wails v3** for the UI. This depends on `webkit2gtk` on Linux.

## Known constraints (high-churn — check docs for current status)

- **Screen exclusivity.** When Live runs as Push's control surface,
  claiming interface 0 fails cleanly with `LIBUSB_ERROR_ACCESS`. Handle
  this case explicitly: report "Live owns the display" and degrade. Do
  not crash.
- **Windows MIDI input port naming.** Windows does not expose jack
  strings the way CoreMIDI and ALSA do, so name-based auto-detect cannot
  work there. The fix is a manual port picker, confirmed 2026-08-18 on
  real Push 3 hardware (Windows 11 VM with USB passthrough).

  The Windows MIDI backend also appends an undocumented `" <n>"` to every
  MIDI port name, not only Push's, and numbers it independently for in
  versus out. This broke role detection and cable-number detection
  outright, until the code stripped the suffix. Confirmed live
  2026-08-19 on real Windows hardware. Detail:
  [docs/platform/windows.md](docs/platform/windows.md).
- **Multi-device pairing.** `pushapp-ui` can claim and drive several Push
  units at once. `internal/display` and `internal/midi` identify units
  by USB serial, or by bus and address when a unit reports no serial.
  They group MIDI cables by physical unit rather than by name alone,
  because two identical units can report byte-identical MIDI port names
  (confirmed on macOS).

  `internal/identify` flashes a unit's screen or pads, so you can tell
  two visually identical units apart when you pair them manually.
  `cmd/pushapp` itself stays single-device; use `-devices`, `-device`,
  and `-midi-in` there instead. Detail:
  [plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md).
- **Two sessions drawing text at once used to crash the whole process.**
  This bug was fixed 2026-08-24 in `ableton-push-hack/core/gfx/text`;
  this repo's own code never touched it.

  `golang.org/x/image/font.Face` is documented as not safe for
  concurrent use. But `core/gfx/text` handed out one shared `Face`
  singleton, and one per cached weight and size, to every caller.
  `pushapp-ui` runs one render goroutine per connected session, so when
  two Push units drew text at the same moment, they corrupted the font
  rasterizer's internal buffers. This was confirmed live with two real
  units connected at once, and reproduced deterministically under `go
  test -race`.

  The fix is a package-level mutex that serializes every call into a
  shared `Face`. See `core/gfx/text/text.go`'s `faceMu`, and
  `TestConcurrentDrawNoRace` in that package, for the regression guard.
- **Disconnect detection.** `cmd/pushapp-ui` notices when someone
  unplugs a Push mid-session, instead of showing a stale module list
  against a dead port. `display.ErrDisconnected` bubbles up through
  `host.Runtime.Run` to `hostManager`, which tears that session down. It
  records the reason, keyed by unit, in `PushService.Overview()`'s
  `unitErrors`. Other sessions stay unaffected.
- **Do not run `pushapp` with Live open, unless Push's own User Mode is
  engaged.** Confirmed 2026-08-20: User Mode is a real device-level
  workaround for **both halves** of the contention, not just pad input.

  User Mode cuts Live off from pad MIDI entirely, while it leaves the
  display claim and button routing untouched. Pad LED writes are routed
  the same way: the Live Port renders only outside User Mode, and the
  User Port renders only inside User Mode.

  A host that targets the User Port for LED writes can paint its own pad
  colors while it fully coexists with Live. `internal/midi` already
  routes this correctly: `OpenRef` pairs each cable with its own
  same-role output, confirmed 2026-08-20 with `pushapp -midi-in "... User
  Port"`. This needs no code change. Only `Open()`'s auto-detect, and the
  bare cable open in `internal/identify.FlashLEDs`, are Live-hardcoded by
  design.

  Without User Mode, co-existence mode leaves Push's MIDI interface
  bound to the OS driver, even while Live does not own the display. Both
  processes then drive the same pad LEDs, which causes visible fighting,
  not just a display conflict. There is no arbitration between the two.
  See [docs/protocol/led-output.md](docs/protocol/led-output.md) and
  [docs/protocol/midi-input.md](docs/protocol/midi-input.md#user-modes-effect-on-routing).

  Live's actual display claimant is a background helper that Live spawns
  (`Push3.app` or `Push2DisplayProcess.app`), not a `launchd`-managed
  process. See
  [docs/protocol/usb-and-safety.md](docs/protocol/usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20).
- **Channel convention.** Every API in this repo uses channels 1-16. The
  code converts them to the wire's 0-15 range inside `midiout`.

Full list, including Push 2/3 deltas and unmeasured items:
[plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md),
[docs/protocol/push2-vs-push3.md](docs/protocol/push2-vs-push3.md).
