# Design system

**Status:** implemented (basics; visual polish is a deliberately later pass)
**Last verified:** 2026-08-22
**Authoritative code:** [`core/gfx/`](https://github.com/federico-pepe/ableton-push-hack/tree/main/core/gfx)
(ableton-push-hack), [`internal/module/frame.go`](../../internal/module/frame.go),
[`internal/renderframe/`](../../internal/renderframe/)

This is a shared, reusable set of drawing components for the 960x160 screen
of Push. Both this repo's modules and the hacks in `ableton-push-hack` use
it.

The decision history and the reasons behind each choice live in
`ableton-push-hack`'s `DESIGN.md`, not on this page. This page maps what
exists and how a module or hack calls it. Roadmap and status:
[`plans/2026-08-21-design-system-screensim.md`](../../plans/2026-08-21-design-system-screensim.md).

> **Invariant: every widget must support the full Push color palette, with
> a sensible fallback when the color is unset.** This rule applies to every
> widget in `core/gfx/widgets`, existing or future. It is not limited to
> the widgets that got color added on 2026-08-22. See "Color" below for
> the exact requirements. See the package doc on `core/gfx/widgets`
> (`theme.go`) for the full, authoritative statement of the rule.

## Layering

```
core/gfx            rect fill, icon compositing — no font, no widgets
core/gfx/text        Draw/Width (Tamzen7x13r, the basic face, default)
                     DrawScaled/WidthScaled (integer upscale of the basic face)
                     NewFace/DrawWith/WidthWith (opt-in Helvetica Neue, antialiased)
core/gfx/layout      8-column grid, top/bottom bar content rect
core/gfx/widgets     composite components built on the three packages above
internal/module      the ABI: Frame's typed methods build an Op display list
internal/renderframe the registry that turns an Op list back into pixels
```

A module never draws pixels itself. `Draw(f *Frame)` calls typed methods on
`Frame`, for example `f.Header(...)`, `f.List(...)`, and `f.Knob(...)`.
Each method appends one `Op{Kind, Params}` to a display list.

The host hands that list to `internal/renderframe.Render`. For a
process-loaded module, the same JSON schema over stdio does this instead.
`internal/renderframe.Render` looks up a registered handler for each `Kind`
and calls the matching `core/gfx/widgets` function.

This is why the op set is **open**. To add a widget, add one `RegisterOp`
call and one typed `Frame` method. This never changes the `Module`
interface. As a result, an old module still works against a newer host. A
module built against a newer widget set degrades gracefully on an older
host: an unknown op is skipped and counted, not fatal.

Everything in `core/gfx/widgets` operates on a plain `*image.NRGBA`. It has
no knowledge of USB, BGR565, or the module ABI. As a result, a hack in
`ableton-push-hack` can call `widgets.DrawKnob` directly, with no
dependency on `push-tethered-app`.

## Screen model

The screen is 960x160px with 8 columns (`core/gfx/layout.Cols`). This
matches the 8 soft-buttons and 8 encoders on each side of the screen, so a
column-aligned control lines up with the physical control under it.

`layout.ColSpan(w, startCol, span)` gives the pixel `(x, width)` for any
span of columns. For example, a 4+4 split is `ColSpan(w,0,4)` plus
`ColSpan(w,4,4)`. Use the same method for a 6+2 or 5+3 split.

`layout.Content(w, h, layout.Bars{TopH, BottomH})` carves off an optional
top bar, bottom bar, or both. It returns the rect that everything else
composes against.

## Widget catalog

Each row lists a `core/gfx/widgets` function, its `Frame` method, and its
op `Kind` string. The third column is the value a process-loaded module in
Python, JS, or another language puts in `{"kind": "...", "params": {...}}`.
See [writing-a-process-module.md](../guides/writing-a-process-module.md).

| Widget | `Frame` method | op kind | What it is for |
|---|---|---|---|
| `DrawHeader` | `Header` | `header` | Filled title bar, left-aligned text |
| `DrawStatusBar` | `StatusBar` | `statusbar` | Bottom status/error line — `StatusBg`/`OffColor` |
| `DrawBreadcrumbBar` | `Breadcrumb` | `breadcrumb` | Top bar with a path, or a status override |
| `RenderList` | `List` | `list` | Vertical scrolling list, cursor, scrollbar |
| `RenderListH` | `HList` | `hlist` | Horizontal scrolling list (columns, not rows) |
| `DrawKVRows` | `KVRows` | `kvrows` | Label:value rows |
| `DrawBotStrip` | `BotStrip` | `botstrip` | The 8 under-screen soft-buttons + a hint |
| `DrawMeter` / `DrawMeterV` | `Meter` / `MeterV` | `meter` / `meterv` | Horizontal / vertical level bar |
| `DrawArc` | `Arc` | `arc` | Raw circular arc primitive, anti-aliased |
| `DrawKnob` | `Knob` | `knob` | Radial-progress knob (arc sweep + value + label), 2px anti-aliased stroke |
| `DrawKnobFull` | `KnobFull` | `knobfull` | Rotary-pointer knob (full circle + angle pointer), 2px anti-aliased stroke |
| `DrawKnobArc` | `KnobArc` | `knobarc` | Gauge knob: 300° arc, 7 o'clock to 5 o'clock, 60° gap at the bottom |
| `DrawFader` | `Fader` | `fader` | Vertical linear control, handle + value |
| `DrawEnvelope` | `Envelope` | `envelope` | Polyline through normalized points, anti-aliased |
| `DrawPadGrid` | `PadGrid` | `padgrid` | `cols x rows` cell grid, row 0 at the bottom |
| `DrawBorder`/`HLine`/`VLine` | `Border`/`HLine`/`VLine` | `border`/`hline`/`vline` | 1px outline / lines |
| — (escape hatch) | `Image` | `image` | Blit an arbitrary `*image.NRGBA` |
| `text.Draw`/`DrawScaled` | `Text`/`TextScaled` | `text` | Basic face (Tamzen7x13r), uppercased, optionally integer-upscaled (`Scale`) |
| `text.DrawWith` | `StyledText` | `styledtext` | Helvetica Neue, `Weight` + point `Size` |

`DrawKnob`, `DrawKnobFull`, `DrawKnobArc`, and `DrawFader` all share the
`Knob` param type: `Label`, `Value`/`Min`/`Max`, and `Color`. `Color` is
the fill or pointer color: `knob`'s sweep, `knobfull`'s pointer line,
`knobarc`'s fill arc, and `fader`'s filled bar.

Its zero value falls back to `Theme.Select`, not white. This differs from
every other color-bearing op param (see "Color defaulting" below), because
white is itself a valid, deliberate `Color` choice a module can make.
Without this fallback, white would be indistinguishable from "unset."
The `knobs-js` module (published separately at
[federico-pepe/pta-module-knobs](https://github.com/federico-pepe/pta-module-knobs),
installable via `-catalog-install knobs-js` — see
[catalog/schema.md](../../catalog/schema.md)) gives every knob its own
palette `Color` to demonstrate the override. `cmd/screensim -scene
controls` does the same for `KnobArc` and `Fader`, side by side with two
Theme-default knobs.

`Knob` also has `ValueScale` (`int`). This field enlarges the value
readout only; `Label` always draws at 1x. It works through
`text.DrawScaled`, the same font at an integer nearest-neighbor larger
size. A value of zero or 1 means 1x, identical to every knob's look before
this field existed. This follows the same zero-value-is-the-old-default
contract as `Color`. Added 2026-08-24.

A scaled-up value collides with `Label`'s fixed position directly below
the knob, once the knob composition stacks two lines there
(`DrawKnobFull`'s value-then-label). The `knobs-js` module (now at
`ValueScale: 2`, tuned down live from an initial 4x that overflowed a
`r: 30` knob) works around this. It leaves `Label` unset and draws each
knob's label itself, in its own `Color`, in one row below every knob,
where a generic status-bar hint used to sit. This replaces reliance on the
widget's own gray, one-size label.

The same module also demonstrates positioning knobs by physical encoder
column (`960/8 = 120px` per column, the same convention the pad grid's 8
columns use). This replaces spreading knobs evenly across the full width.
One knob sits per column, and all 8 columns are used.

`Knob` also has `Bipolar` (`bool`), for `DrawKnobArc` only. It changes the
fill from "grows from `Min`" to "grows from the middle of `[Min,Max]`
outward, in the direction `Value` moved." Nothing draws when `Value` sits
exactly at the middle.

False, the zero value, gives the original behavior. Added 2026-08-24 for a
pan, detune, or LFO-offset-style control. In these controls, a symmetric
range's untouched center value reads as an empty ring, not a permanently
half-full one. The `knobs-js` module's "PAN 1" and "PAN 2" (range
-50 to +50, starting at 0) demonstrate this.

Every widget gets two things from the host for free. A module does not
need to think about them:

- **ASCII enforcement.** `internal/renderframe` sanitizes every string
  field before it reaches `core/gfx/text`. `core/gfx/text` sanitizes the
  field again itself. Both the basic and styled faces are now outline
  fonts, so neither has the old fixed bitmap's free "no glyph past ASCII"
  guarantee. A non-ASCII byte becomes `?` (or `.` for a truncation
  ellipsis) instead of a silent rendering bug. Write ASCII, and do not
  rely on the substitution to look good.
- **Theme.** `Header`, `KVRows`, `List`, `HList`, `BotStrip`, `Breadcrumb`,
  and `StatusBar` all take colors from `Host.Theme()` (`widgets.Theme`,
  starting point `widgets.Default`) instead of literal colors. As a
  result, a module's UI matches whatever palette the host runs. Every
  entry in `widgets.Default` (and `widgets.groupColors`, the soft-button
  group underline colors) is resolved through `push3.Palette`/
  `ColorForIndex`, not a hand-picked RGB literal. See Color below.
- **Color defaulting.** Any color-bearing op field that a module leaves at
  its JSON zero value (`color.NRGBA{}`, that is, omitted) renders
  **white**, not invisible transparent black. `internal/renderframe.defaultColor`
  applies this rule to every `rect`, `text`, `styledtext`, `border`,
  `hline`, `vline`, `meter`, `meterv`, `arc`, `padgrid`, and `envelope` op
  before drawing.

## Grouping and pagination — conventions, not widgets

IDEAS.md asked for two things. Neither needed new drawing code:

- **Soft-button groups**: `SoftButton.Group` (int, 0 = none) draws a thin
  colored underline that clusters an arbitrary subset of the 8 slots. This
  is purely visual, because soft-buttons have no physical per-button LED;
  their state feedback is the on-screen label color. The selection logic,
  which button or buttons in a group are "on", lives in the calling
  module through `module.ButtonGroup` (`Toggle`/`IsSelected`), a plain
  state tracker that is not part of the ABI. It supports two semantics,
  picked per group: `Exclusive: true` for radio behavior, and `false` for
  independent toggles, for example mute/solo or multi-select filters.
- **Whole-frame pagination** (more content than fits in one pass over the
  8 encoders and 8 soft-buttons): no widget or op exists for this, by
  design. A module already redraws its full `Draw` every frame and owns
  whatever state it wants. So "page 2 of 2" is just an int field the
  module branches on, advanced from `Handle` on an ordinary `Button`
  event. `"D-Pad left"`/`"D-Pad right"` arrive like any other button, and
  no new Host API is needed.

## Fonts and sizing

There are two font families, with different distribution stories, because
only one is freely redistributable:

- **Basic face — Tamzen7x13r** (`text.Draw`/`DrawScaled`/`Width`) is the
  default everywhere. It is freely licensed (Scott Fial's Tamsyn/Tamzen),
  so it is **embedded** (`//go:embed` from `core/gfx/text/assets/`), with
  no system font install needed.

  It replaced the old fixed `basicfont.Face7x13` bitmap on 2026-08-22. It
  is an outline font (`font/opentype`, `HintingFull`), rendered at
  13pt/72dpi to match the old 7x13 cell exactly (`Width` still hardcodes a
  7px advance).

  The font is **always drawn uppercase**. `text.Draw` uppercases its
  input before sanitizing, because Tamzen at this size reads better
  all-caps than lowercase, which has no true descenders drawn in the
  cell.

  `DrawHeader`'s and `DrawStatusBar`'s baseline offsets (`primitives.go`)
  were retuned for Tamzen's ink shape, which sits differently in its em
  box than the old bitmap's did. Do not assume the two fonts share a
  baseline constant if you add a new bar widget.
- **Styled face — Helvetica Neue** (`NewFace`/`DrawWith`/`WidthWith`,
  `Frame.StyledText`) is opt-in, at an arbitrary point size. `Weight`
  maps Regular to Thin, Bold to Medium, Italic to ThinItalic, and
  BoldItalic to MediumItalic (`core/gfx/text/face.go`).

  This face is **not embedded and not committed**, because Helvetica
  Neue's `.otf` files are Apple/Monotype-licensed and both repos are
  public. `source()` reads the four files at runtime from
  `PUSHAPP_STYLED_FONT_DIR`, a developer-populated, gitignored local
  directory (see this repo's own `/assets`, gitignored the same way).
  When that env var or file is missing, it falls back to the original
  vendored `gofont` TTFs, so a fresh clone or CI still builds and
  renders, with the generic weights.

  It is built with `font.HintingNone`. Helvetica Neue's OTF (CFF/PostScript
  outlines) carries no TrueType hint program, so `HintingFull` was a
  no-op hint pass that made antialiasing look worse, not better, on the
  low-res, coarse-color-depth (BGR565) panel of Push. `HintingNone` lets
  the rasterizer's true coverage-based AA through instead.

  `modules/remap`'s editor uses `TextScaled`, an integer nearest-neighbor
  upscale of the basic face, not Helvetica, to make the dialed-in value
  read as the important number rather than just a different color from
  its label.

`modules/ui-text-demo` is a live tuning bench for both faces. Every
encoder drives one parameter: face, weight, size, palette color, or
margin. This lets a developer dial in and check a rendering change on
real hardware, instead of guessing constants and rebuilding
`cmd/screensim` scenes each time. See its package doc for the full
control map.

### Color

`core/push3.Palette`/`ColorForIndex` was added alongside the font swap,
for the same reason: the screen and LEDs must agree. It resolves a raw
0-127 hardware palette index to its RGBA value, rounded down to the
nearest of the 90 named entries in `NamedColors`. This is the same
SysEx-sourced table that `internal/midi`'s pad and button LED writes
already use by index.

A widget or module that wants to *preview* an LED color on screen, or
that offers "cycle through the palette" as a single control instead of
raw RGB sliders, must read from this table rather than hand-copy hex
values. See [docs/protocol/led-output.md](../protocol/led-output.md) and
`modules/ui-text-demo`'s color encoder for the pattern.

**Rule: no raw RGB literals in the design system (2026-08-22).**
`widgets.Default` and `widgets.groupColors` (`core/gfx/widgets/theme.go`,
`softbutton.go`) build every color through `push3.ColorByName`/
`ColorForIndex`, instead of a hand-picked `color.NRGBA{...}`. Each entry
is the closest palette match to the original hand-picked value.

A widget or module that adds a new named color must do the same: pick the
nearest `push3.Palette` entry, rather than invent a new RGB triple. This
keeps every color on screen traceable to a real, named Push color. This
is a convention, not an enforced type. Nothing stops a literal
`color.NRGBA{}`, but do not add one going forward.

**Rule: no raw RGB in process modules either (2026-08-22).** A JS or
Python module cannot import `push3`, because it is a Go package. So
`cmd/genpalette` generates `palette.json` into every `examples/modules/*`
directory that has a `manifest.json`. This file holds all 128 raw
indices, resolved through `push3.ColorForIndex`, plus a `byName` map of
the named entries.

Load `palette.json` and look up a color by name or by 0-127 index,
instead of hand-copying RGB. See
[writing-a-process-module.md](../guides/writing-a-process-module.md#colors)
for the loader snippet in both languages, and `hello-{js,py}` and
`beatcount-{js,py}` (in `examples/modules/`) or `knobs-js` (published
separately — see [catalog/schema.md](../../catalog/schema.md), installable
via `-catalog-install knobs-js`) for working examples.

Regenerate `palette.json` with `go run ./cmd/genpalette` after any change
to `push3.Palette` itself. This is rare. `palette.json` is a checked-in
generated file, not a build step.

**Rule: every widget must accept a color, not hardcode one (2026-08-22).**
This is the invariant called out at the top of this page. The full
statement lives in `core/gfx/widgets`' package doc (`theme.go`). In
short: a widget's color must be a `color.NRGBA` parameter, directly or
through a field like `Knob.Color`/`SoftButton.Color`. Any `push3.Palette`
entry must be usable. An unset color must still render something
sensible, not nothing: white for widgets with no natural per-instance
default (see "Color defaulting" below), or the widget's own existing
default otherwise — `Theme.Select` for `Knob`/`KnobFull`/`KnobArc`/
`Fader`, and the `State`-picked color (`White`/`OnColor`/`OffColor`) for
`SoftButton`.

An audit against this rule found two gaps: `DrawFader` and `SoftButton`.
`DrawFader` hardcoded `t.Select`/`t.White`, even though it already took a
`Knob` param with an unused `Color` field. `SoftButton` had no per-button
color override at all, only the closed `State` enum. Both gaps were
fixed on 2026-08-22.

`groupColors` (the `Group` underline cue, a fixed 4-color cycle keyed by
group number) is deliberately exempt from this rule. See
`ableton-push-hack/DESIGN.md`'s "Soft-button groups" section.

### Anti-aliasing (2026-08-22)

`DrawArc` and the package-private `drawLine` (`core/gfx/widgets/
primitives.go`) are anti-aliased by default. They use coverage-based
alpha blending against a signed distance to the arc's radius or the line
segment, not the original step-along-the-shape-and-round-to-a-pixel
approach.

This is a default for every caller, not a knob-only mode. `DrawEnvelope`
gets it for free, because it already calls `drawLine` per segment. Any
hack that calls `widgets.DrawArc` directly picks it up too.

`DrawKnob`, `DrawKnobFull`, and `DrawKnobArc` additionally draw at
`knobStroke = 2` px, instead of the shared 1px default. They use the same
width-parameterized `drawArcWidth`/`drawLineWidth` helpers. For
`DrawKnobArc`'s arbitrary start angle, they use the more general
`drawArcSpanWidth`, which both of those helpers wrap. See
`ableton-push-hack/DESIGN.md`'s "Anti-aliased primitives" section for the
full rationale.

## Previewing without hardware

`cmd/screensim` renders named test scenes to PNG. It needs no display
claim and no Push hardware:

```bash
go run ./cmd/screensim -list-scenes
go run ./cmd/screensim -scene <name> -out out.png
go run ./cmd/screensim -scene <name> -grid -out out.png   # overlay the 8-column guides
go run ./cmd/screensim -scene <name> -raw -out out.png    # skip the BGR565 round-trip
```

Two kinds of scene, registered in `cmd/screensim/scenes.go`:

- **Frame-mode** (`frameScenes`): builds a `*module.Frame` the way a real
  module's `Draw` would. It renders through the exact same
  `internal/renderframe.Render` path that the host uses. This proves that
  an op renders the way it would on a real run.
- **Direct-draw** (`drawScenes`): calls `core/gfx`/`core/gfx/widgets`
  straight against an `*image.NRGBA`. Use this to try a widget before it
  has a `Frame` op at all.

A third registry, `cmd/screensim/modules.go`, renders an actual
compiled-in module's current `Draw` output (`-scene mod:<id>`). It does
this by calling `Init` on the module against `moduletest`'s fake host.
Use this to check a real module's screen without touching hardware or
writing a throwaway scene by hand.

## Writing a module against this

Use the typed `Frame` methods, for example `f.Header(...)` and
`f.List(...)`. Never use `AppendRaw`; it exists only for the
process-loader and for tests that need to simulate an op the host does
not know about. See
[writing-a-go-module.md](../guides/writing-a-go-module.md) for the module
contract itself. This page covers what to draw with, once you are inside
`Draw`.

For a process-loaded module, in any language, the wire format is the
same `{"kind": "...", "params": {...}}` shape that `internal/module`'s
Go types serialize to. `examples/modules/hello-py` and `hello-js` show a
minimal example. `examples/modules/beatcount-py` and `beatcount-js` show
a slightly larger one, using the `header` op for its title bar, the same
way every compiled-in Go module does.

## What's deliberately not here yet

`ableton-push-hack`'s `keyboard-visualizer` hack does not use this
system. It draws its own graphics, and the catalog above has no
equivalent "keyboard" widget. The team judged that extracting one was
not worth doing speculatively. This follows the same precedent in
reverse as this package's own "add the tool, don't invent a use yet"
rule: do not invent a widget for one caller either, until a second
caller appears.
</content>
</invoke>
<invoke name="Read">
<parameter name="file_path">/Users/fpp/Developer/push-tethered-app/docs/architecture/module-host.md