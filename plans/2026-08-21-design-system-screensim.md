# Push design system: screen-simulator tool + roadmap

## Status (2026-08-21)

All roadmap items below are implemented — layout grid, breadcrumb bar,
horizontal-scroll list, soft-button groups, the missing-widgets basket
(meter-v, knob, knob-full, fader, envelope, pad-grid), text scaling, and
alternate fonts/weights (`text.NewFace`/`DrawWith`, answering IDEAS.md's
font questions with code, not just a recommendation). All 5 compiled-in
Go modules, both example languages (Python/JS), and `hacks/push-manager`
(already compliant, audited not re-migrated) now use the shared widgets
for title bars and status bars — `hacks/keyboard-visualizer` deliberately
excluded, no equivalent widget exists for it. Full catalog and how-to:
[docs/architecture/design-system.md](../docs/architecture/design-system.md).
Decisions: `ableton-push-hack`'s `DESIGN.md`.

`modules/uidemo` is the hardware-verification module this plan's last
step asked for — one page per widget cluster, each driven by a real
control (D-Pad, encoders, pads, soft-buttons). It's built, tested,
rendered through every page via `cmd/screensim -scene mod:ui-demo:<N>`,
and **run on real Push 3 hardware, confirmed 2026-08-22** — the open item
this status block used to flag is closed.

**2026-08-22 update:** the outline-font path this plan's "Font questions"
section only recommended is now actually adopted as every module's real
on-screen look: the basic face swapped from `basicfont.Face7x13` to an
embedded Tamzen7x13r outline font (freely licensed, drawn uppercase), and
the styled face swapped from `gofont`'s generic
Regular/Bold/Italic/BoldItalic to Helvetica Neue
(Thin/Medium/ThinItalic/MediumItalic) — Regular/Bold/Italic/BoldItalic at
the `Weight` level, but the concrete face is now Helvetica Neue rather
than gofont wherever it's available. Helvetica Neue is **not** embedded or
committed (Apple/Monotype-licensed, both repos public): it loads at
runtime from a gitignored local directory
(`PUSHAPP_STYLED_FONT_DIR`), falling back to the original `gofont` TTFs
when absent, so a fresh clone or CI still builds — the styled face is a
fallback-gated opt-in for the font *file*, even though `StyledText` itself
remains an unconditional part of the API. Both faces sanitize to ASCII
themselves now, same reasoning this plan already called out.
`modules/ui-text-demo`
is a new module built specifically to dial in this swap live on hardware
(face/weight/size/palette-color/margin, one per encoder) — the widget
author this old status paragraph said hadn't shown up yet. Full detail:
[docs/architecture/design-system.md](../docs/architecture/design-system.md#fonts-and-sizing).
Also added: `core/push3.Palette`/`ColorForIndex`, resolving a raw hardware
LED palette index to RGBA for on-screen preview — same "screen and LEDs
should agree" motivation, done alongside the font work since
`ui-text-demo` needed a color control and hand-rolling a second RGB table
next to `NamedColors` was the wrong move.

**2026-08-22 update (visual polish, first pass):** three targeted
follow-ups, done together since they touched the same knob/arc/line code:
`widgets.Default` and `widgets.groupColors` now resolve every color
through `push3.Palette`/`ColorForIndex` instead of hand-picked RGB
literals (nearest palette match to each original value), so every color a
built-in widget draws is traceable to a real, named Push color; an unset
`color.NRGBA{}` on any module `Frame` op now renders white
(`internal/renderframe.defaultColor`) instead of invisible transparent
black; and `DrawArc`/`drawLine` (`core/gfx/widgets/primitives.go`) are now
anti-aliased by default via coverage-based blending
(`blendPixel`/`drawArcWidth`/`drawLineWidth`) rather than the original
step-and-round approach, so every caller — `DrawKnob`, `DrawKnobFull`,
`DrawEnvelope`, and any hack calling the primitives directly — gets a
smooth edge instead of a stair-stepped one. `DrawKnob`/`DrawKnobFull` also
draw at `knobStroke = 2` px instead of 1. Full rationale:
`ableton-push-hack/DESIGN.md`'s "Anti-aliased primitives" and updated
"Palette" sections. Endless-encoder handling in `modules/uidemo` and
`modules/ui-text-demo` was also fixed alongside this to clamp
(`push3.ClampInt`) at the write-time accumulator rather than wrapping —
turning past a bounded control's limit now stops there and reverses
immediately, instead of rolling back to the minimum.

Not done: further visual polish beyond this pass (color/AA/stroke-width)
— deliberately deferred, per this round's own instruction.

## Context

`IDEAS.md` (Desktop) kicks off a shared, reusable widget/design system for
Push modules — usable both by `ableton-push-hack` "hacks" and
`push-tethered-app` "modules" (the latter built on top of the former via a
`go.mod replace` on `core/`). Two pieces of prior research already exist and
are **not** being redone: `ableton-push-hack/discovery/shadow-ui-component-framework.md`
(status DONE) already delivered `core/gfx/widgets` (Theme, SoftButton/BotStrip,
ListView/DrawListRows/DrawScrollbar/RenderList, KVRow, DrawBorder/HLine/VLine/
Meter/Header/Arc, an unused forward-looking `Knob` type), and
`push-tethered-app/plans/2026-08-18-frame-text-scale.md` (open/parked) already
designed integer text upscaling.

The gap: no module in `push-tethered-app` actually uses `List`/`Header`/
`KVRows`/`BotStrip` yet (monitor/seq hand-roll everything), and several things
IDEAS.md asks for don't exist anywhere — pagination, horizontal scroll,
true multi-select button groups with LED grouping, Fader, circular Knob,
envelope curve, and (most blocking) **any way to preview widget/module
output as an image without building the full app or touching hardware**.

User confirmed via Q&A: new widget code extends the *existing*
`core/gfx/widgets` package (not a new one); this session builds **only** the
screen-simulator/export tool; the rest gets sequenced into a roadmap, not
implemented now; DESIGN.md (in `ableton-push-hack`) gets authored as
decisions are actually made, not upfront.

## Verified current state (don't re-derive)

- `internal/module/moduletest/ascii.go` only statically scans built `Frame`
  ops — no rendering. `fake.go` deliberately avoids importing `internal/host`
  because that drags in `gousb`/libusb transitively (via `internal/display`).
- `internal/host/render.go` already has the exact renderer needed:
  `Render(f *module.Frame, dst *image.NRGBA, t module.Theme) RenderStats`,
  op registry (`RegisterOp`/`ops`), and `init()` registrations for every op
  kind. Its only problem is living in the gousb-tainted `package host`.
- `internal/capture/capture.go`'s `panelize()` already solves "make an
  exported image match what the hardware actually shows" by round-tripping
  through `core/display`'s `ToBGR565`/`FromBGR565` (BGR565 quantization),
  with a `Raw` escape hatch — reuse this pattern, don't reinvent it.
- `core/gfx/widgets/primitives.go` already has `DrawMeter` (horizontal) and
  `DrawArc` — these substantially cover "meter" and "arc knob" already.
- No standalone breadcrumb-bar Frame op exists (`DrawBreadcrumbBar` is only
  reachable via `Frame.List`). No pagination, horizontal-scroll, or
  image-export tool exists anywhere in either repo.
- `golang.org/x/image` (already a dependency in both repos, v0.41.0) vendors
  `font/gofont/*` (goregular/gobold/gobolditalic/goitalic/gomono/etc.) and
  `font/opentype`+`font/sfnt` — real outline fonts, usable as a drop-in
  `font.Face` swap for `basicfont.Face7x13` without a new dependency.

## This session's build: screen-simulator / export tool

**Step 1 — extract the renderer out of `internal/host` (prerequisite).**
Move `Render`, `RegisterOp`, `SupportedOps`, `RenderStats`, `asciiOnly`, the
`ops` registry, and the `init()` op registrations from
`internal/host/render.go` into a new leaf package with zero gousb-tainted
imports — `internal/renderframe` (only imports `core/gfx`, `core/gfx/text`,
`core/gfx/widgets`, `encoding/json`, `image`, `internal/module`, none of
which touch gousb). `internal/host` then calls into it at its one call site
(`host.go:417`) — no ABI or behavior change. `render_test.go` moves with the
code it tests. Verify: `go build ./... && go vet ./... && go test ./...` in
`push-tethered-app`, confirm no import cycle (renderframe doesn't import
`internal/host`).

**Step 2 — the tool itself: `push-tethered-app/cmd/screensim/main.go`.**
Lives here (not in `push-hack`) because it needs both `core/gfx/widgets`
*and* the module ABI (`internal/module`, new `internal/renderframe`); a
push-hack-only widget author still benefits by using direct-draw mode
without touching `internal/module` at all.

Two input modes, both producing a `*image.NRGBA` sized 960x160
(`push3.VisW x push3.VisH`):
1. **Frame mode** — a named registry of `func() *module.Frame` "scenes"
   (style like `frametest`'s `renderTestImage`), rendered via
   `renderframe.Render(frame, img, theme)`.
2. **Direct-draw mode** — a named registry of `func(img *image.NRGBA)`
   scenes calling `core/gfx`/`core/gfx/widgets` directly, for prototyping a
   widget before it has a Frame op at all.

Export via stdlib `image/png`. Reuse `capture.go`'s panelize pattern: a
`-panel-accurate` flag (default true) round-trips through
`core/display.ToBGR565`/`FromBGR565` before encoding; `-raw` skips it. CLI
shape:
```
screensim -scene monitor.idle -out /tmp/out.png
screensim -scene seq.playing -raw -out /tmp/out_raw.png
screensim -list-scenes
```
A `-grid` flag overlays 8-column division lines (`960/8=120px`) plus
top/bottom bar boundaries as plain rect/line overlays — useful now, and
becomes the visual check for the layout-grid roadmap item below. A couple of
`go test` cases assert a known scene produces a correctly-sized, non-blank
PNG — no golden-image testing yet (natural follow-up once real widgets
exist to regress against).

**Critical files:**
- `internal/host/render.go` — code to extract
- `internal/host/host.go:417` — call site to update after extraction
- `internal/capture/capture.go` — panelize/ToBGR565/FromBGR565 pattern to reuse
- `internal/module/frame.go` — `Frame`/`Op`/typed constructors for Frame-mode scenes
- `core/gfx/widgets/primitives.go` — direct-draw mode's target API

## Roadmap (sequenced, design-level only — not built this session)

1. **Layout grid model** — new package (`core/gfx/layout` or extend
   `core/gfx/widgets`): 8 columns over 960px, common splits (4+4, 6+2, etc.),
   optional top/bottom bar heights carved off first, producing a "content
   rect" other widgets compose against. Verify visually with `screensim
   -grid` before rewriting any module to use it.
2. **Breadcrumb bar as first-class** — promote `DrawBreadcrumbBar` to its own
   `Frame.Breadcrumb(...)` + `"breadcrumb"` op + host/renderframe registration,
   so a module can have a top bar without a full scrolling list. Additive,
   no ABI break.
3. **Pagination** — nothing exists today. Needs a page-index/total concept
   (fields alongside `ListView` or a new `PageView`), a rendering convention
   (indicator, likely breadcrumb-bar or bottom-right), and an input
   convention (which control advances page — touches `Handle(ev Event)`,
   not just `Draw`). Design in DESIGN.md before implementing.
4. **Horizontal-scroll list** — current `ListView`/`DrawListRows`/
   `DrawScrollbar` are vertical-only by construction. Add a parallel
   `DrawListCols`-style sibling (mirrors how HLine/VLine are already
   separate) rather than generalizing the existing, already-tested vertical
   path in place. New `"hlist"` op, additive.
5. **True multi-select toggle groups + LED grouping** — `SoftButton` today
   is per-button with no group concept. LED grouping is a hardware/input
   concern (`Host.SetButton`) as much as a drawing one, so this spans both
   `core/gfx/widgets` (visual grouping styling) and `internal/module`'s
   `Host` interface. Design in DESIGN.md as touching both repos before any
   code lands, per CLAUDE.md's "fix shared code upstream" rule.
6. **Missing widgets**, roughly cheapest-to-hardest:
   - Vertical meter — small sibling of existing `DrawMeter` (swap w/h roles).
   - Arc knob — compose the *already-existing* `DrawArc` with the
     *already-defined-but-unused* `Knob` type into a `DrawKnob(...)` that
     also renders `Knob.Value`/`Knob.Label` (IDEAS.md's "value must always
     be displayed" becomes a hard requirement of the renderer, not left to
     callers).
   - Full-circle knob — distinct primitive from arc knob (rotated indicator
     around a full circle vs. a partial sweep); needs its own draw function.
   - Fader — vertical linear control, closer to a rotated `DrawMeter` fill
     plus a handle marker and value readout.
   - Envelope curve — genuinely new, a point-array-to-polyline renderer;
     least reusable from current code, scope last.
   - Pad Grid — IDEAS.md says "already available" in push-tethered-app;
     audit where (likely `modules/seq`) and whether it's already shared or
     still hand-rolled — if hand-rolled, extract into `core/gfx/widgets`
     the same way lists/kvrows were unified, before treating it as new work.
7. **Text scaling follow-through** — plan is already fully designed and
   parked; once the simulator exists, use it to visually verify `DrawScaled`
   at each scale factor and settle the plan's open questions (which scale
   factors, does Header/KVRow scale too) empirically rather than
   speculatively.
8. **DESIGN.md authoring** (`ableton-push-hack/DESIGN.md`, doesn't exist
   yet) — write as decisions from steps 1-6 are made, as a living
   index/summary (palette, grid constants, op-naming conventions, "why"
   notes) that links to per-feature discovery docs rather than duplicating
   them, matching how `discovery/` is already organized.

## Font questions (answers for DESIGN.md, not decided here)

**Different fonts?** Yes, no new dependency — `golang.org/x/image`'s
`font/gofont/*` + `font/opentype`/`font/sfnt` are already vendored;
`basicfont.Face7x13` swaps for an `opentype.Face` as a drop-in `font.Face`
change (`text.go`'s `font.Drawer` usage is already `Face`-agnostic).
Trade-off to weigh explicitly: `Face7x13` being a fixed 1-bit bitmap is
*why* ASCII-only is enforceable today (the font has no other glyphs);
antialiased outline fonts can render arbitrary Unicode, so adopting them
means moving ASCII enforcement to an explicit check at the `Draw` call site
instead of relying on font coverage. Recommendation: keep `Face7x13` as
default/fallback, add an opt-in alternate `text.DrawWith(face, ...)` —
additive, gated behind that enforcement change, decided in DESIGN.md.

**Different sizes / bold / italic?** Sizing has two independent paths: the
already-parked integer-scale plan (`DrawScaled`, cheap, stays monochrome,
ship first, item 7 above) for clean integer multiples of 7x13; true
arbitrary sizing needs the opentype path. Bold/italic is *only* reachable
via the `gofont` outline family (`gobold`, `gobolditalic`, `goitalic`,
etc. — already vendored) since `Face7x13` is a single fixed face with no
variants — so weight/style support is a strict subset of the "different
fonts" decision, same trade-offs, same DESIGN.md gate.

## Verification for this session's work

- `go build ./... && go vet ./... && go test ./...` in `push-tethered-app`
  after the `internal/renderframe` extraction and after adding `cmd/screensim`.
- Run `screensim -list-scenes`, then render at least one Frame-mode and one
  direct-draw-mode scene, inspect the output PNGs, confirm 960x160 dimensions
  and non-blank content, and confirm `-panel-accurate` vs `-raw` visibly
  differ (BGR565 quantization banding) on a scene with gradients/color detail.
- No hardware needed for this step — that's the point of the tool.
