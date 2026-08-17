# Module host — the shape of v1

**Status: decided 2026-08-17, in progress.** Supersedes the open question in
[2026-08-16-product-shape-decision.md](2026-08-16-product-shape-decision.md).

## Context

The 16/08 plan framed three candidate products (A Live companion / B remapper /
C creative surface) and recommended B. The decision is **none of them as
stated** — it is a fourth shape that subsumes B and C:

> `pushapp` is a **host** that owns Push hardware and runs **modules**. Anyone
> can write a module, with or without the help of AI. Ableton Live is not
> involved at any layer.

Three consequences fall straight out, and they are the reason this is worth
doing:

1. **Option B becomes a module, not a product.** A remapper is example module
   #3. The original stated goal is preserved without the whole app being shaped
   around it.
2. **Option A is dead and stays dead.** No DAW coupling means the screen
   exclusivity tension (§4.1) never arises — we simply require that Live is not
   holding the display, and `ErrBusy` already degrades cleanly.
3. **"Full ownership" does not mean claiming USB interface 5.** It means *we are
   the only host*. OS MIDI via `rtmididrv` already works on macOS, Linux and
   Windows with zero driver install, and WinMM's exclusive-open only ever hurt
   when sharing Push with a DAW. **The libusb MIDI backend is removed from
   scope entirely** — a possible later latency optimisation, not a requirement.

Decisions taken:

| Axis | Decision |
|---|---|
| Module ABI | Go in-tree first; external-process loader second, same contract |
| Draw API | Display list rendered by the host, raw-image op as escape hatch |
| MIDI out to other software | **In v1** |
| Concurrency | One module active; switching happens in the app UI, not on hardware |
| Platforms | macOS + Linux + Windows, Pi 4/5 as a stretch |
| Devices | Push 2 **and** Push 3 from day one |

### The MIDI-out fact that settles Windows

Verified in the vendored RtMidi sources at `gitlab.com/gomidi/midi/v2@v2.3.24`,
not recalled:

- `rtmididrv.Driver.OpenVirtualOut(name)` exists — `drivers/rtmididrv/driver.go:105`.
- macOS CoreMIDI creates a real virtual destination — `RtMidi.cpp:1637`.
- Linux ALSA seq creates one — `RtMidi.cpp:2553`.
- **Windows WinMM refuses**: `"MidiOutWinMM::openVirtualPort: cannot be
  implemented in Windows MM MIDI API!"` — `RtMidi.cpp:3128`. WinUWP the same,
  `:3947`. It is a *warning*; no port is created.

Therefore the host does not "create a virtual port" — it **owns a named output
port**, obtained one of two ways:

- **create** (macOS, Linux): `OpenVirtualOut(name)`.
- **attach** (Windows): enumerate existing outputs, match a user-supplied name,
  open it. The user installs loopMIDI (free) or uses Windows MIDI Services.

One code path downstream. No driver shipped, no commercial licence, no Zadig.
Windows costs one documented install step rather than blocking v1.

---

## Architecture

### The seam that does not exist today

`cmd/pushapp/main.go` is flat: one `state` struct at `:46`, one `handle`
type-switch at `:68`, one `render` with hardcoded coordinates at `:242`, LED
writes inline on the driver thread at `:82`. The hardware packages
(`internal/display`, `internal/midi`, `internal/pushmap`) are clean and stay as
they are; everything above them gets replaced by a host + module contract.

The precedent to lift is upstream: push-manager's `Panel` interface
(`hacks/push-manager/src/ui_shadow.go:157`) with a host render loop — proven on
hardware. Lift the shape, fix its known gaps: pads never reach panels, no
init/teardown hook, a 4-soft-button cap, and a hardcoded panel slice.

### New packages

```
internal/module/     the ABI: Module, Host, Frame/Op, Event, Meta, Store
internal/host/       the host: registry, control API, event fan-out, render loop
internal/host/render.go   op registry: display list -> *image.NRGBA via core/gfx
internal/midiout/    named output port: create (mac/linux) or attach (windows)
modules/monitor/     example 1 — today's pushapp behaviour
modules/seq/         example 2 — step sequencer, proves MIDI out
modules/remap/       example 3 — encoders/buttons -> CC/notes (old option B)
```

### The contract

```go
// internal/module/module.go
type Module interface {
    Meta() Meta
    Init(h Host) error      // on activation
    Handle(ev Event)        // all input
    Draw(f *Frame)          // build one frame's display list
    Close() error           // on deactivation
}

type Meta struct {
    ID, Name, Author, Version string
    NeedsMIDIOut bool
}

type Host interface {
    Device() pushmap.Device            // Push 2 vs Push 3 — never hide this
    SetPad(note, colour byte)
    SetButton(cc, brightness byte)
    SendCC(ch, cc, val byte) error     // to the owned out port
    SendNote(ch, note, vel byte) error
    SupportedOps() []string
    Log(format string, args ...any)
    Store() Store                      // per-module persisted JSON
}
```

Three non-obvious requirements, each with a reason:

**1. `module.Event` is a separate, unsealed, serialisable mirror of
`midi.Event`.** `internal/midi/midi.go:46` seals `Event` with an unexported
method, which is right for the decoder but makes it impossible for a
process-loaded module to reconstruct events in another package. The host
translates `midi.Pad/Button/Encoder/Touch/Expression` into `module.*`
equivalents with JSON tags. Five small structs duplicated, in exchange for an
ABI that is stable and wire-ready independent of the decoder.

**2. `Handle` and `Draw` are never concurrent.** Today LED writes happen inline
on the RtMidi driver thread (`main.go:82`) and `Port.send` has no
synchronisation at all. The host instead pushes decoded events into a buffered
channel and runs a single **module goroutine** that drains events and calls
`Draw` on the frame tick. Module authors need no mutexes. The driver thread
never blocks — on a stalled module the host drops oldest events and counts them.

**3. The host sanitises text.** `core/gfx/text` renders a missing-glyph box for
any non-ASCII (CLAUDE.md), and per §9.4 that class of bug is invisible in logs
reporting a healthy frame rate. Rendering a display list means it is enforced in
one place instead of trusting every module author to remember.

*Corrected during implementation:* this originally also claimed the host would
clip ops to the panel box. It does not need to — `gfx.FillRect` goes through
`draw.Draw` and `text.Draw` through `font.Drawer`, and **both already clip to the
destination bounds**. Off-canvas ops are simply invisible. Adding a second
clamping layer would have been theatre; `TestOffCanvasOpsAreHarmless` pins the
upstream behaviour instead, so a regression there fails a test rather than
reaching hardware.

*Upstream follow-up, done 2026-08-17:* `text.Truncate` appended U+2026 when it
cut a string, so the likeliest source of non-ASCII was a helper modules are
*encouraged* to use — and it was drawing glyph boxes on real hardware in
push-manager's file browser, not just latent here. **Fixed in
`ableton-push-hack` (branch `fix-truncate-ascii`)**: the marker is now `"..."`,
a latent `maxRunes <= 0` panic is gone, and `core/gfx/text` gained its first test
file asserting every output byte is printable ASCII.

The host's `asciiOnly` stays regardless, as defence against older `core/`
checkouts and any other source of non-ASCII. It maps `…` to `.` and everything
else non-printable to `?`, and stays a **1:1 character** substitution on purpose:
expanding one rune to three would change a string's rendered width mid-frame and
could overflow a layout the module already measured with `text.Width`.

### Display list — designed to grow with `core/gfx`

`Frame` records ops; it is not an image. Because `core/gfx` and
`core/gfx/widgets` are planned to expand with more components, **the Op set must
not be a closed hand-maintained enum** — otherwise every new upstream widget
forces an ABI edit here and breaks older modules or older hosts.

So an op is open-ended:

```go
type Op struct {
    Kind   string          `json:"kind"`   // "rect", "text", "list", "knob", ...
    Params json.RawMessage `json:"params"`
}
```

and the renderer is a **registry**, not a switch:

```go
// internal/host/render.go
func RegisterOp(kind string, fn func(dst *image.NRGBA, t widgets.Theme, params json.RawMessage) error)
```

Adding a widget after it lands upstream is then two small additions — one
`RegisterOp` handler plus one typed constructor method on `Frame` — with no
change to the ABI, no version bump, and no existing module touched.

Three rules make that safe:

- **Capability handshake.** `Host.SupportedOps()` lets a module ask what this
  host knows. An unknown `Kind` is a logged, counted, skipped op — never a panic
  and never a silent mis-render.
- **Typed constructors, untyped wire.** Modules never hand-build `Op` structs;
  they call `f.Rect(...)`, `f.List(...)`, `f.Knob(...)`. The `string` +
  `RawMessage` shape exists for the wire and the registry, not for module
  authors.
- **`Kind` names are frozen once shipped.** Renaming an op is a breaking change;
  adding one is not.

Initial handler set, each mapping to something that already exists:

| Kind | Renders via |
|---|---|
| `rect` | `gfx.FillRect` |
| `text` | `text.Draw` (+ `text.Width`, `text.Truncate`) |
| `border` / `hline` / `vline` / `meter` / `arc` | `widgets` primitives |
| `list` | `widgets.RenderList` (`ListView` is already a per-frame value) |
| `kvrows` | `widgets.DrawKVRows` |
| `botstrip` | `widgets.DrawBotStrip` |
| `image` | `gfx.DrawIcon` / direct blit — the escape hatch |

Next in line as upstream work lands: `knob` (`widgets.Knob` is a type with no
renderer today, `primitives.go:88`) and `padgrid`.

Buys four things at once: serialisable for the process loader, clippable,
themeable by the host, and cheap on a Pi.

### Panel ownership and where switching lives

**There is no hardware switcher and no host chrome.** On launch, `pushapp` takes
the hardware exactly as it does today, and the active module owns the entire
960×160 panel plus every pad, encoder, button and touch sensor unconditionally.
Nothing is reserved.

This is a deliberate departure from push-manager's tab model. That model exists
because on **standalone** Push there is no computer UI to switch from — it is a
constraint of the standalone product and must not be copied here. Tethered has a
desktop app, so **install, uninstall and switch all live in the app's own UI**
(Wails v3, already decided — see CLAUDE.md's architecture decisions). Three
things follow:

- No button is stolen from modules, so the 4-soft-button cap that
  `discovery/shadow-ui-8button-strip.md` exists to fix never applies here.
- No layout constants, no clipping conflict between chrome and module content.
- The standalone-mode-button hazard in CLAUDE.md's hardware-safety section is a
  *standalone-device* concern and does not constrain this design.

The host therefore exposes a **control API** — a plain Go interface
(`List() []Meta`, `Activate(id) error`, `Active() Meta`, `Install(path) error`,
`Uninstall(id) error`) that Wails binds directly. In-process, no socket needed.
`Install`/`Uninstall` only become meaningful once modules live on disk, so they
land with the process loader in phase 4; before that the UI switches among
built-in modules.

**Keep a headless path.** A `-module <id>` flag must run the host with no window
at all. Wails needs `webkit2gtk` on Linux (the one place the stack isn't
standalone, per CLAUDE.md), and a Pi 4/5 in kiosk use shouldn't have to pay for
a webview it never shows.

### MIDI out

```go
// internal/midiout
func Open(name string) (*Out, error)   // try OpenVirtualOut, else attach by name
func (o *Out) Mode() string            // "virtual" | "attached" — report which
```

Try create, fall back to attach-by-name, log which happened. No build tags, and
it picks up Windows MIDI Services automatically if that ever lands.
**Must exclude any port whose name contains `Push`** — attaching to Push's own
port would build a feedback loop.

### What goes upstream to `ableton-push-hack/core/` and what does not

The split follows the precedent already set in
`discovery/shadow-ui-component-framework.md` (components were promoted to core;
the Shadow-UI *frame* deliberately was not):

- **Upstream:** all *widgets*, including the planned `core/gfx`/`widgets`
  expansion. Nothing here blocks on that work or duplicates it — the op registry
  means each new widget is additive. Nearest candidates the example modules will
  want: a pad-grid widget, an actual renderer for `widgets.Knob`
  (`primitives.go:88`), and clipping support if clipping belongs at that layer
  rather than in the host.
- **Stays here:** the `Module`/`Host`/`Frame` ABI, the host, the renderer, the
  loader, USB display transport, OS MIDI, `pushmap`. The Op set is a host
  contract, not a drawing component.
- **Do not** create `core/push2`; Push 2 deltas stay in
  `internal/pushmap/push2.go`.

### Config

Per-module JSON under `os.UserConfigDir()`, host does atomic writes, module sees
only `Store.Get/Set` over a `json.RawMessage`. Do **not** press `core/hackcfg`
into service — it is 37 lines of `{ID,Name,Version,Port}` shaped for on-device
services.

---

## Phasing

**Phase 0 — MIDI-out spike. DONE 2026-08-17.** `internal/midiout` +
`cmd/midiouttest`. Verified on macOS: `Open` created the port in `virtual` mode,
CoreMIDI published it as a system MIDI **input** named `Push Tethered App` within
~200ms, and a second process attached to it received all 8 notes and the CC
sweep byte-for-byte on the right channel. The `-listen` flag makes the probe its
own receiver, so this is machine-checkable on any OS with no synth involved.

One trap worth remembering: this project uses channels **1-16** at every API,
converted to the wire's 0-15 inside `midiout`, but `gomidi`'s
`Message.String()` prints channels **0-based** — a note sent on channel 3 logs as
`channel: 2`. Pinned by `TestStatusChannelConversion`.

**Phase 1 — contract + host. DONE 2026-08-17.** `internal/module`,
`internal/module/moduletest`, `internal/host`, `modules/monitor`, and
`cmd/pushapp` cut down to wiring. Verified on Push 3 hardware: the monitor renders
identically to the pre-host app at 29.3 fps, confirmed by extracting a frame from
`-capture` rather than trusting the frame counter.

Three things learned or changed while building it:

- **`module.Frame.AppendRaw` was added to the ABI.** Tests need to inject an op
  the typed constructors cannot produce, to prove a module built against a newer
  `core/gfx` degrades instead of breaking the frame. That is not test-only
  scaffolding: the process loader needs exactly the same entry point to rebuild
  a display list received as JSON.
- **Drawing types are aliases of `core/gfx/widgets`, not copies** —
  `type ListView = widgets.ListView` and so on. Upstream stays the single source
  of truth and an upstream addition appears here for free. Caveat recorded in
  the code: `ListRow.Icon` is an `*image.NRGBA` and will not survive a process
  boundary, so icons are in-process only until the loader gains an image handle.
- **MIDI out is opened lazily, and getting that right took two attempts.** The
  first hardware run published a `Push Tethered App` port on every launch even
  though the monitor never sends a byte — on macOS and Linux that call publishes
  to the whole system. Fix #1 decided from the set of *compiled-in* modules,
  which broke the moment `thru` was added: one sending module in the binary made
  every run publish a port again. Fix #2, which is correct: `host.Options` takes
  an `OpenMIDIOut` **function**, and the Runtime calls it on activation of a
  module that declares `NeedsMIDIOut` — never earlier. A failed open is cached,
  so a module sending on every pad press cannot retry a doomed open at input
  rate (on Windows each attempt enumerates every MIDI port on the machine). The
  Runtime owns the port's lifetime and closes it on shutdown.

**`modules/thru` was added after phase 1** to close the gap that `moduleHost`'s
`SendCC`/`SendNote`/`NoteOff` had never executed — `midiout` was proven only by
`cmd/midiouttest`, which bypasses the host entirely. It forwards pads to notes,
the eight screen encoders to CC 1-8 (relative accumulated to absolute, clamped),
and buttons to their own CC. It is the identity case of the phase-2 remap module
rather than a throwaway probe. Deliberate limits: MPE member channels are
collapsed onto one output channel and `Expression` is ignored, because
predictable output matters more than faithful output for something whose job is
to be verifiable.

Still outstanding from this phase: `Store` is a stub (`memStore`) so the
interface does not churn later, but nothing persists yet.

**Phase 2 — config store, two more modules. DONE 2026-08-17.** Control API and
`-module <id>` already existed from phase 1; this phase built the real
persistence they were stubbed for, plus `modules/seq` and `modules/remap`.
Verified on Push 3 hardware: `seq`'s playhead was confirmed advancing correctly
by sampling multiple frames from `-capture` (a single frame at BPM 120 landed
back on step 0 by coincidence — 2.0s / 0.25s-per-step = exactly 8 steps —
which would have read as a stuck sequencer if only one frame had been checked);
`remap` was confirmed loading a hand-edited override end to end (`1 override(s)
loaded`, then rendering `note:40 -> note 45 [20-100]` via `KVRows`, the one
widget none of the other three modules exercise).

- **Persistence: `internal/host/store.go`.** One JSON file per module ID under
  `os.UserConfigDir()/push-tethered-app/modules/`, atomic write via temp file +
  rename. `moduleHost.Store()` now returns this instead of the phase-1 no-op
  stub. If the config directory can't be resolved, it degrades to an in-memory
  store rather than failing activation — persistence is not load-bearing for a
  module's actual job. `userConfigDir` is a package-level function variable so
  tests point it at `t.TempDir()` instead of touching the real OS location.
- **A module never sees its own config path — only `Store.Get/Set`.** But
  there is no config UI yet (phase 3), so a user has no way to hand-edit a
  module's JSON without knowing where it lives. Resolved by having the host log
  it on activation (`module remap: config at ...`) — purely informational,
  doesn't leak the path into the `Host` interface.
- **`modules/seq`** — an 8-step, 8-lane gate sequencer using the pad grid
  itself as the sequencer (columns = steps, rows = lanes). Proves MIDI driven by
  wall-clock timing rather than input: the step-advance logic (`tick`) takes an
  explicit `time.Time` rather than reading the clock itself, so it is fully
  testable without a real clock or a sleep — `Draw` is the only thing that ever
  calls it with `time.Now()`. Known rough edge, documented in the code: because
  the step index is derived from total elapsed time rather than an incrementing
  counter, changing BPM mid-playback can visibly jump the current step, not
  just its speed. Not worth the complexity of re-anchoring for an 8-step proof.
- **`modules/remap`** — the actual option-B remapper, ported from
  `hacks/push-manager/src/remap.go`. With no overrides configured its behaviour
  is identical to `thru` — `thru` is this module's identity case, proved by
  `TestNoOverridesBehavesLikeThru`. One deliberate deviation from the ported
  model, documented in the package doc: push-manager's `srcKey` includes the
  source channel; this module's does not, because pad note-ons are the one
  control that's multi-channel (MPE rotates a pad's channel between sessions
  with no user action), so keying on channel would make a saved override
  silently stop matching after a channel rotation.
- **A real hardware bug surfaced here, not caught by any existing test**:
  `remap`'s empty-overrides message used an em-dash in the source. The host's
  `asciiOnly` sanitiser did exactly its job and silently turned it into `?` on
  the real screen — but nothing failed, because every module's
  "Draw emits only known op kinds" test checked op *Kind* only, never op
  *content*. Fixed the string, and closed the actual gap: added
  `moduletest.NonASCIIStrings(f)`, which decodes every text-bearing field in a
  `Frame`'s display list, and wired a `TestDrawTextIsASCII` into all four
  modules so this class of bug fails a test next time instead of only showing
  up on the panel.

**Phase 3 — app UI. DONE 2026-08-17 (Go side verified; window not yet
eyeballed).** Minimal switcher, deliberately scoped down from the original
phase description: list modules, show which is active, switch. No per-module
settings editor — `seq`'s BPM and `remap`'s overrides are still edited by
hand-editing the config file the host logs on activation, same as from the
CLI, until a later phase adds a settings view. Headless `-module` path in
`cmd/pushapp` is untouched.

- **`cmd/pushapp-ui` is a separate nested Go module**, the same pattern
  `ableton-push-hack/core` already uses, chosen over folding it into the main
  module specifically to keep the existing CI job (`go build ./... && go vet
  ./... && go test ./...` at repo root) completely untouched — Wails and
  webkit2gtk never enter that graph unless someone explicitly builds the UI.
  Confirmed: `go list ./...` from repo root does not mention `pushapp-ui` at
  all. Cost, paid once: its `go.mod` carries its own two `replace` directives
  (back to the repo root, and to `ableton-push-hack/core`), mirroring the
  fragile-relative-path problem the root module already has — CI's existing
  core-checkout step will need one more `go mod edit -replace` line to cover
  it, not yet done.
- **`internal/bootstrap` is new**: the hardware-opening sequence (claim MIDI,
  claim the display with the `ErrBusy` degrade path, wire the lazy `OpenMIDIOut`
  opener, wire an optional capture recorder, build the `Runtime`) was inlined in
  `cmd/pushapp/main.go` alone through phase 2. With a second real caller
  (`cmd/pushapp-ui`), duplicating it would risk the two entry points' error
  handling silently drifting apart, so it moved to a shared package first.
  `cmd/pushapp/main.go` now calls it too — refactor verified byte-for-byte
  behaviourally identical on Push 3 hardware before building anything new on
  top of it.
- **`internal/` packages are importable across the module boundary on
  purpose.** Go's internal-package visibility rule is based on import *path*
  text, not module identity — a separate module can import
  `.../push-tethered-app/internal/host` as long as its own declared module
  path also starts with `github.com/federico-pepe/push-tethered-app/`. That is
  why `-mod` was set explicitly to
  `github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui` rather than left
  to whatever `wails3 init` would have derived from a git URL.
- **`PushService`** (`cmd/pushapp-ui/pushservice.go`) is a thin JSON-shaping
  wrapper over `Runtime.List`/`Active`/`Activate` — no new behaviour, bound to
  the frontend via Wails' service mechanism. `ModuleInfo` is a dedicated bound
  type rather than exposing `module.Meta` directly, so the frontend's contract
  doesn't move if `Meta` gains fields later.
- **The frontend polls, it doesn't subscribe.** There is no "module switched"
  event yet — `main.ts` re-fetches `ListModules` every 2s and after every
  `ActivateModule` call. Phase 4's process loader is the natural point to add
  a real event, once there's a loader-level reason to push state rather than
  poll it.
- **Verified**: `go build`/`go vet` clean on the actual app package; a full
  `wails3 build` succeeds end to end (icons, bindings generation, frontend
  build via Vite, native Go build) and produces a working 8.6MB
  `cmd/pushapp-ui/bin/pushapp-ui`. Bindings were inspected directly — Wails
  generated exactly 1 service, 2 methods and 1 model, matching `PushService`
  and `ModuleInfo`, with the Go doc comments carried through into the
  generated JS. **Not verified: the window itself.** There is no way to drive
  or screenshot a live GUI window from this environment, so whether the list
  actually renders correctly and switching feels right needs a human running
  `wails3 dev` (hot reload) or the built binary and looking at it.
- **`go build ./...` inside `cmd/pushapp-ui` fails on `build/ios` and
  `build/android`** — Wails' own generated mobile entry-point stubs, which
  only satisfy their build constraints under their respective mobile
  toolchains, not a bare desktop `go build`. Not a bug; scope every build/vet
  command to the app package itself (`go build .`, or let `wails3 build`
  choose the target) rather than `./...` in this directory.

**Phase 4 — process loader.** `internal/host/procmod`: a module is any
executable, contract marshalled over stdio or a unix socket. This is what makes
`Install`/`Uninstall` real and delivers "anyone can create a module" — ship one
Python and one JS example.

**Phase 5 — platform completion.** Windows on real hardware with loopMIDI; Pi
4/5 per [2026-08-17-raspberry-pi-support.md](2026-08-17-raspberry-pi-support.md)
(headless path, no webview).

---

## Critical files

Modified: `cmd/pushapp/main.go` (gutted to wiring), `go.mod` (nothing new needed
for phases 0–2).
Unchanged: `internal/display/display.go`, `internal/midi/midi.go` (plus the
`module.Event` translation), `internal/pushmap/`.
New: `internal/module/`, `internal/host/`, `internal/midiout/`, `modules/*`.

Reuse rather than rewrite: `core/gfx` (`FillRect`, `DrawIcon`), `core/gfx/text`
(`Draw`, `Width`, `Truncate`), `core/gfx/widgets` (`Theme`/`Default`,
`RenderList`, `DrawKVRows`, `DrawBotStrip`, `DrawMeter`, `DrawArc`,
`DrawBorder`), `core/push3` (`PadNote`, `PadCoord`, `DecodeRel`, `ScaleVal`,
`ClampInt`, `IsEncoderCC`, `NamedColors`), `core/display` (`ToBGR565`,
geometry).

## Verification

Unit, hardware-free — and this is where the repo is weakest today (one test
file, no fakes):

- `internal/module`: display-list ops round-trip through JSON unchanged.
- `internal/host/render.go`: golden-image tests — render a fixed op list into an
  `*image.NRGBA` and compare. Also assert out-of-bounds ops are clipped, not
  panicking, that non-ASCII text is sanitised, and that an **unregistered op
  kind is skipped and counted** rather than fatal — that last one is the
  regression guard for the whole extensible-op design.
- `internal/midi`: `DecodeFor` is a pure function over `[]byte` and currently
  untested — add table tests, including Active Sensing `0xFE` and the CC 15 /
  CC 111 per-device split.
- A fake `module.Host` so a module's `Handle` can be tested with no hardware.

On hardware (controller mode, Live closed — the screen must be eyeballed, exit 0
is not evidence):

1. `go build ./... && go vet ./... && go test ./...`
2. `go run ./cmd/pushapp` — monitor module identical to today. Pads light, log
   scrolls, encoders accumulate, LEDs clear on SIGINT.
3. `go run ./cmd/pushapp -capture out.mp4` and inspect a frame — both §9.4 bugs
   were invisible in healthy-looking logs.
4. `go run ./cmd/pushapp -module seq` — confirm the module owns the whole panel
   and every control, and that switching releases the previous module's LEDs
   cleanly.
5. Sequencer module → virtual port → a soft synth or MIDI monitor app receives
   notes.
6. Repeat 2–5 on **Push 2** — device identity flows through `Host.Device()`.

## Docs to update as phases land (doc-sync rule)

- `plans/2026-08-16-product-shape-decision.md` → status **decided: Option D**,
  with why A/B/C were each subsumed or dropped.
- `docs/open-questions.md` → delete the resolved §1 entries (product shape;
  Windows virtual MIDI, now answered by create-or-attach). Keep Windows
  hardware, keep Pi. Add as newly open: the on-disk module manifest format for
  `Install`/`Uninstall`, and whether Wails survives the headless-Pi requirement.
- `CLAUDE.md` + `README.md` → the module host shape, the layout table, the
  create-or-attach MIDI-out fact, and the removal of libusb-MIDI from scope.
