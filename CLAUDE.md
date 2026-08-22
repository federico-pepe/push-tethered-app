# CLAUDE.md

Guidance for Claude Code (claude.ai/code) working in this repository. This is
a slim agent manual — durable reference lives in [docs/](docs/), read it
first for anything beyond the safety rules and pointers below.

> **Doc sync rule:** update this file, `README.md`, `MANUAL.md`, and `docs/`
> when a change is *meaningful* to a future reader — new behaviour, a changed
> protocol fact, a new API, a resolved or newly-found issue. Not every commit
> needs a doc update; skip internal refactors and trivial edits. When you do
> update, keep it in the same commit as the change. New protocol fact →
> `docs/protocol/`; resolved open question → fold into docs and delete from
> [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md); anything
> an **end user** needs to operate or configure the app correctly (pairing,
> port roles, running alongside Live, troubleshooting a specific error
> message) → [MANUAL.md](MANUAL.md), not the UI itself — keep the app's own
> screens terse and put explanation in the manual instead of inline text.
>
> **Plans live in `plans/`**, one file per plan, named
> `YYYY-MM-DD-name-of-the-plan.md`. `docs/` holds durable reference (protocol
> facts, measurements, rationale) for **contributors**; `MANUAL.md` holds the
> same kind of durable truth but written for **end users** — how to use the
> app, not how it's built; `plans/` holds intent (what we're about to do and
> why, including open decisions). When a plan is done, distill its settled
> contract into `docs/architecture/` (and into `MANUAL.md` if it changes how
> a user operates the app); leave the plan itself as historical reasoning.
>
> **`docs/archive/` is frozen** — never edit, move, or delete anything inside
> it, never add to it unless explicitly asked. It holds superseded docs kept
> for history.

## Project

`push-tethered-app` — cross-platform desktop app that owns an **Ableton Push 2 /
Push 3 in tethered (controller) mode**: display, pads, buttons, encoders, LEDs.
**It is a module host** — `pushapp` owns the hardware and runs **modules**,
small programs anyone can write in Go or any other language, that draw the
screen and handle the controls. No DAW is involved at any layer.

**Status: pre-alpha, running, confirmed on Push 2 and Push 3 hardware,
including pairing and driving two units at once from `pushapp-ui`** (macOS,
Windows, and Linux/Raspberry Pi — see
[plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md)). Full
picture (built-in modules, process-loaded modules, `pushapp-ui`): root
[README.md](README.md) and [docs/README.md](docs/README.md).

Decision history: [plans/2026-08-17-module-host.md](plans/2026-08-17-module-host.md)
(module-host shape), [plans/2026-08-17-process-loader.md](plans/2026-08-17-process-loader.md)
(any-language modules). [plans/2026-08-16-product-shape-decision.md](plans/2026-08-16-product-shape-decision.md)
is **closed** — reasoning trail only, don't plan against it.

## Read this first

- [MANUAL.md](MANUAL.md) — end-user manual: pairing, port roles, running
  alongside Live, troubleshooting
- [docs/README.md](docs/README.md) — reading paths by task (write a module,
  build/contribute, protocol facts)
- [docs/protocol/usb-and-safety.md](docs/protocol/usb-and-safety.md) — before
  any hardware interaction
- [plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md) — what's still unresolved
- [docs/archive/feasibility.md](docs/archive/feasibility.md) — frozen, the
  original protocol evidence and stack rationale (don't edit)

## Relationship to `ableton-push-hack`

Sibling of `~/Documents/GitHub/ableton-push-hack` (Push 3 *standalone*,
deployed over SSH). `core/` is reused, not copied, via a `replace` in
`go.mod` — **never fork or vendor it**, fix upstream so both projects
benefit. That repo's hard safety rules (no `/boot`, `/opt`, `/etc`) don't
apply here — but see USB safety below. Full detail:
[docs/hardware-reference.md](docs/hardware-reference.md).

## Non-negotiable safety rules

These stay here even though they're also in `docs/` — an agent needs them
in-context before touching hardware, not one click away.

- **Claim only interface 0 (display).** Claiming MIDI/audio interfaces takes
  them away from the OS and the DAW.
- **Never write to `xPort` (interface 6) speculatively** — vendor-specific,
  undocumented.
- **No firmware operations. Ever.** No DFU, no control transfers with
  unknown vendor requests.
- **Never do a blind "press every button" sweep.** Run `cmd/pushapp` first —
  once a host drives the screen, the top-row buttons become plain MIDI and
  are safe to press. The leftmost button above the screen switches Push 3
  into standalone mode if the display isn't already held.
- **Always clear LEDs on every exit path, including SIGINT** — leaving the
  device lit makes the next run ambiguous.
- **Never call `dev.SetAutoDetach(true)`** — it's config-wide, not
  interface-wide, and tears audio/MIDI away from the OS class drivers.
- **ASCII only** when drawing — `core/gfx/text` renders any non-ASCII
  character as a missing-glyph box.

Full protocol detail (display format, MIDI decode order, MPE, LED messages,
button-sweep recovery): [docs/protocol/](docs/protocol/).

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
examples/modules/ process-loaded example modules (Python, Node.js)
tools/            macOS-only Swift probes (midimon, ledtest)
```

Full package-by-package rationale: [docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

### `cmd/pushapp-ui` is a separate Go module — do not add it to root's `./...`

It has its own `go.mod` with two `replace` directives (root repo,
`ableton-push-hack/core`), needs `wails3` (the CLI) + Node/npm to build, and
its config lives in `build/config.yml` (v3, not v2's `wails.json`). CI builds
it on all three OSes. Full detail:
[docs/guides/development-setup.md](docs/guides/development-setup.md).

## Writing a module

The contract is `internal/module.Module` — `Meta`, `Init`, `Handle`, `Draw`,
`Close`. `modules/monitor` is the reference implementation. Modules can also
be **any executable, any language** — `internal/host/procmod` runs one as a
child process over JSON-over-stdio; `examples/modules/` has working Python
and Node.js modules.

Guides: [writing-a-go-module.md](docs/guides/writing-a-go-module.md),
[writing-a-process-module.md](docs/guides/writing-a-process-module.md),
[writing-a-python-module.md](docs/guides/writing-a-python-module.md),
[writing-a-javascript-module.md](docs/guides/writing-a-javascript-module.md).
Architecture: [architecture/module-host.md](docs/architecture/module-host.md),
[architecture/process-modules.md](docs/architecture/process-modules.md).

The drawing widget set (`core/gfx/widgets`, shared with `ableton-push-hack`)
is catalogued in [docs/architecture/design-system.md](docs/architecture/design-system.md)
(widget-to-op-to-Frame-method map, how to preview with `cmd/screensim`);
its design decisions live in `ableton-push-hack`'s `DESIGN.md`. Start
there before adding a new widget or drawing pattern. Design-system work
in progress: [plans/2026-08-21-design-system-screensim.md](plans/2026-08-21-design-system-screensim.md).

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
go build ./... && go vet ./... && go test ./...
```

`pushapp` flags: `-fps`, `-module <id>`, `-list`, `-no-display` (MIDI only),
`-no-leds`, `-midi-out <name>`, `-no-midi-out`, `-ext-midi-in <name>`,
`-no-ext-midi-in` (external MIDI input for modules that declare
`NeedsMIDIIn` — `internal/midiin`, raw bytes delivered as
`module.ExternalMIDI`; unlike MIDI-out, missing input is never fatal to
activation, see `internal/module/module.go`'s `Meta.NeedsMIDIIn` doc),
`-capture`, `-capture-raw`,
`-install <dir>`, `-uninstall <id>` (filesystem-only, no Push needed),
`-version` (prints `internal/version.Version`, "dev" unless built with the
release workflow's `-ldflags`), `-devices` (lists every attached Push unit
and MIDI cable, claims nothing, safe with Live open — paste this into a bug
report), `-device <serial:XXXX|usb:BUS.ADDR>` and `-midi-in <name>` (pick a
specific unit/cable when more than one Push is attached; `pushapp` itself
stays single-device — see
[plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md) for
`pushapp-ui`'s multi-session pairing instead).

```bash
cd cmd/pushapp-ui
wails3 dev              # hot-reload window; needs wails3 + Node/npm
wails3 build            # produces bin/pushapp-ui
```

Full flag reference and probe tools: [docs/guides/debugging.md](docs/guides/debugging.md).

## Releases

Semantic Versioning (pre-1.0, so expect breaking changes between minors):
`vMAJOR.MINOR.PATCH[-alpha|-beta|-rc.N]`. Current stage: `-alpha`.

Cutting a release:

```bash
git tag v0.1.1-alpha
git push origin v0.1.1-alpha
```

Pushing a `v*` tag triggers `.github/workflows/build.yml`'s `release` job:
it waits on `build`/`build-pi`, zips their artifacts, and publishes a GitHub
Release for that tag (pre-release flag set automatically for
`-alpha`/`-beta`/`-rc` tags) via `softprops/action-gh-release`. Update
[CHANGELOG.md](CHANGELOG.md) in the same commit as the tagged code, under
`## [Unreleased]`, then retitle that section to the new version when tagging.

`build.yml` does **not** trigger automatically on PRs or plain pushes to
`main` (see the comment at its top) — only `workflow_dispatch` (manual) and
`v*` tags run CI. Nothing runs without being explicitly asked for, on this
project's own preference as much as free-tier Actions minutes.

## Cross-platform builds

**No cross-compiling this app.** `gousb` (libusb) and `rtmididrv` (vendored
RtMidi C++) are both cgo — **build natively on each target OS**.
`.github/workflows/build.yml` does this on real macOS/Linux/Windows runners.
Per-OS setup: [docs/platform/macos.md](docs/platform/macos.md),
[docs/platform/linux.md](docs/platform/linux.md),
[docs/platform/windows.md](docs/platform/windows.md). Wails' own
[cross-platform build guide](https://v3.wails.io/guides/build/cross-platform/)
does not change this — its Docker cross-toolchain covers apps with no extra
C dependencies (or only what its image bundles); this app's libusb and
per-OS RtMidi backend (CoreMIDI/ALSA/WinMM) would need a custom image
carrying those for every target, and would still leave the runtime DLL
story (`docs/platform/windows.md`'s missing-DLL section) unsolved, since
that's a shipping problem independent of where the binary was compiled.

For a one-off diagnostic build on a platform with no local toolchain (no
release cut needed), use `.github/workflows/diagnostics.yml`
(`gh workflow run diagnostics.yml` or the Actions tab) — builds
`probe`/`frametest`/`mapcheck`/`pushapp`/`identifytest` natively per OS in
about two minutes. Deliberately separate from `build.yml`'s own disabled
copy of that build (see the comment there) rather than re-enabling it.

## Architecture decisions already made

Rationale in [docs/archive/feasibility.md](docs/archive/feasibility.md) §6
and [docs/architecture/stack-and-layout.md](docs/architecture/stack-and-layout.md).

- **Go**, single static binary, for `core/` reuse.
- **`gousb`** (cgo → libusb) for USB. Cost: no cross-compilation, LGPL-2.1.
- **`gitlab.com/gomidi/midi/v2` + `drivers/rtmididrv`** for OS MIDI — no
  brew/apt dependency across all three OSes. Do not add rtmidi/portmidi as
  system packages.
- **Push's MIDI is read through the OS, never libusb.**
- **Wails v3** for the UI. Depends on `webkit2gtk` on Linux.

## Known constraints (high-churn — check docs for current status)

- **Screen exclusivity.** Live running as Push's control surface makes
  claiming interface 0 fail with `LIBUSB_ERROR_ACCESS`, cleanly. Handle
  explicitly: report "Live owns the display" and degrade, don't crash.
- **Windows MIDI input port naming** doesn't expose jack strings the way
  CoreMIDI/ALSA do — name-based auto-detect can't work there. Fixed with a
  manual port picker, confirmed 2026-08-18 on real Push 3 hardware (Windows
  11 VM + USB passthrough). This driver's Windows backend also appends an
  undocumented `" <n>"` to every MIDI port name (not just Push's, and
  independently numbered for in vs out), which broke role and cable-number
  detection outright until stripped — confirmed live 2026-08-19 on real
  Windows hardware. Detail:
  [docs/platform/windows.md](docs/platform/windows.md).
- **Multi-device pairing.** `pushapp-ui` can claim and drive several Push
  units at once — `internal/display`/`internal/midi` identify units by USB
  serial (or bus/address when a unit reports none) and group MIDI cables by
  physical unit rather than by name alone, since two identical units can
  report byte-identical MIDI port names (confirmed on macOS). `internal/identify`
  flashes a unit's screen or pads so two visually identical units can be told
  apart when pairing manually. `cmd/pushapp` itself stays single-device; use
  `-devices`/`-device`/`-midi-in`. Detail:
  [plans/2026-08-19-multi-device.md](plans/2026-08-19-multi-device.md).
- **Disconnect detection.** `cmd/pushapp-ui` notices when a Push is unplugged
  mid-session (`display.ErrDisconnected` bubbles up through
  `host.Runtime.Run` to `hostManager`, which tears that session down and
  records the reason, keyed by unit, in `PushService.Overview()`'s
  `unitErrors`) rather than showing a stale module list against a dead port.
  Other sessions are unaffected.
- **Don't run `pushapp` with Live open** — unless Push's own User Mode is
  engaged, confirmed 2026-08-20 as a real device-level workaround for
  **both halves** of contention, not just pad input: it cuts Live off from
  pad MIDI entirely while leaving the display claim and button routing
  untouched, **and** pad LED writes are exclusively routed the same way —
  Live Port renders only outside User Mode, User Port only inside it. A host
  that targets User Port for LED writes can paint its own pad colours while
  fully coexisting with Live — `internal/midi` already routes this
  correctly (`OpenRef` pairs each cable's own same-role output, confirmed
  2026-08-20 with `pushapp -midi-in "... User Port"`), no code change
  needed; only `Open()`'s auto-detect and `internal/identify.FlashLEDs`'s
  bare cable open are Live-hardcoded by design. Without User Mode,
  co-existence mode leaves Push's MIDI
  interface bound to the OS driver even while Live doesn't own the display,
  so both processes end up driving the same pad LEDs — visible fighting, not
  just a display conflict. There's no arbitration between the two. See
  [docs/protocol/led-output.md](docs/protocol/led-output.md) and
  [docs/protocol/midi-input.md](docs/protocol/midi-input.md#user-modes-effect-on-routing).
  Live's actual display claimant is a background helper it spawns
  (`Push3.app`/`Push2DisplayProcess.app`, not `launchd`-managed) — see
  [docs/protocol/usb-and-safety.md](docs/protocol/usb-and-safety.md#ableton-background-processes-confirmed-2026-08-20).
- **Channel convention: 1-16 at every API in this repo**, converted to the
  wire's 0-15 inside `midiout`.

Full list, including Push 2/3 deltas and unmeasured items:
[plans/2026-08-18-open-items.md](plans/2026-08-18-open-items.md),
[docs/protocol/push2-vs-push3.md](docs/protocol/push2-vs-push3.md).
