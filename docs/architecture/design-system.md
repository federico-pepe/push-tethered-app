# Design system

**Status:** implemented (basics; visual polish is a deliberately later pass)
**Last verified:** 2026-08-21
**Authoritative code:** [`core/gfx/`](https://github.com/federico-pepe/ableton-push-hack/tree/main/core/gfx)
(ableton-push-hack), [`internal/module/frame.go`](../../internal/module/frame.go),
[`internal/renderframe/`](../../internal/renderframe/)

A shared, reusable set of drawing components for Push's 960x160 screen,
used by both this repo's modules and `ableton-push-hack`'s hacks. Decision
history and the "why" behind each choice lives in `ableton-push-hack`'s
`DESIGN.md`, not here — this page is the map of what exists and how a
module or hack actually calls it. Roadmap and status:
[`plans/2026-08-21-design-system-screensim.md`](../../plans/2026-08-21-design-system-screensim.md).

## Layering

```
core/gfx            rect fill, icon compositing — no font, no widgets
core/gfx/text        Draw/Width (Face7x13 bitmap, default)
                     DrawScaled/WidthScaled (integer upscale of Face7x13)
                     NewFace/DrawWith/WidthWith (opt-in antialiased outline fonts)
core/gfx/layout      8-column grid, top/bottom bar content rect
core/gfx/widgets     composite components built on the three packages above
internal/module      the ABI: Frame's typed methods build an Op display list
internal/renderframe the registry that turns an Op list back into pixels
```

A module never draws pixels itself. `Draw(f *Frame)` calls typed methods
on `Frame` (`f.Header(...)`, `f.List(...)`, `f.Knob(...)`, …), each of
which appends one `Op{Kind, Params}` to a display list. The host — or, for
a process-loaded module, the same JSON schema over stdio — hands that list
to `internal/renderframe.Render`, which looks up a registered handler per
`Kind` and calls the matching `core/gfx/widgets` function. This is why the
op set is described as **open**: adding a widget is one `RegisterOp` call
plus one typed `Frame` method, never a change to the `Module` interface,
so an old module keeps working against a newer host and a module built
against a newer widget set degrades gracefully (unknown op = skipped and
counted, never fatal) on an older one.

Everything in `core/gfx/widgets` operates on a plain `*image.NRGBA` and
knows nothing about USB, BGR565, or the module ABI — a hack in
`ableton-push-hack` can call `widgets.DrawKnob` directly with no
dependency on `push-tethered-app` at all.

## Screen model

960x160px, 8 columns (`core/gfx/layout.Cols`) — matching the 8 soft-buttons
and 8 encoders either side of the screen, so a column-aligned control lines
up with the physical control under it. `layout.ColSpan(w, startCol, span)`
gives the pixel `(x, width)` for any span of columns (a 4+4 split is
`ColSpan(w,0,4)` + `ColSpan(w,4,4)`; 6+2, 5+3, etc. the same way).
`layout.Content(w, h, layout.Bars{TopH, BottomH})` carves off an optional
top and/or bottom bar and returns the rect everything else composes
against.

## Widget catalog

Each row is a `core/gfx/widgets` function, its `Frame` method, and its op
`Kind` string (the third column is what a process-loaded module in Python/
JS/anything else puts in `{"kind": "...", "params": {...}}` — see
[writing-a-process-module.md](../guides/writing-a-process-module.md)).

| Widget | `Frame` method | op kind | What it's for |
|---|---|---|---|
| `DrawHeader` | `Header` | `header` | Filled title bar, left-aligned text |
| `DrawStatusBar` | `StatusBar` | `statusbar` | Bottom status/error line — `StatusBg`/`OffColor` |
| `DrawBreadcrumbBar` | `Breadcrumb` | `breadcrumb` | Top bar with a path, or a status override |
| `RenderList` | `List` | `list` | Vertical scrolling list, cursor, scrollbar |
| `RenderListH` | `HList` | `hlist` | Horizontal scrolling list (columns, not rows) |
| `DrawKVRows` | `KVRows` | `kvrows` | Label:value rows |
| `DrawBotStrip` | `BotStrip` | `botstrip` | The 8 under-screen soft-buttons + a hint |
| `DrawMeter` / `DrawMeterV` | `Meter` / `MeterV` | `meter` / `meterv` | Horizontal / vertical level bar |
| `DrawArc` | `Arc` | `arc` | Raw circular arc primitive |
| `DrawKnob` | `Knob` | `knob` | Radial-progress knob (arc sweep + value + label) |
| `DrawKnobFull` | `KnobFull` | `knobfull` | Rotary-pointer knob (full circle + angle pointer) |
| `DrawFader` | `Fader` | `fader` | Vertical linear control, handle + value |
| `DrawEnvelope` | `Envelope` | `envelope` | Polyline through normalized points |
| `DrawPadGrid` | `PadGrid` | `padgrid` | `cols x rows` cell grid, row 0 at the bottom |
| `DrawBorder`/`HLine`/`VLine` | `Border`/`HLine`/`VLine` | `border`/`hline`/`vline` | 1px outline / lines |
| — (escape hatch) | `Image` | `image` | Blit an arbitrary `*image.NRGBA` |
| `text.Draw`/`DrawScaled` | `Text`/`TextScaled` | `text` | Face7x13, optionally integer-upscaled (`Scale`) |
| `text.DrawWith` | `StyledText` | `styledtext` | Antialiased outline font, `Weight` + point `Size` |

Two things every one of these gets from the host for free, so a module
never has to think about them:

- **ASCII enforcement.** `internal/renderframe` sanitizes every string field
  before it reaches `core/gfx/text` — Face7x13 has no glyph past ASCII and
  draws a missing-glyph box instead, so a non-ASCII byte becomes `?` (or
  `.` for a truncation ellipsis) rather than a silent rendering bug. Write
  ASCII; don't rely on the substitution to look good.
- **Theme.** `Header`, `KVRows`, `List`, `HList`, `BotStrip`, `Breadcrumb`,
  `StatusBar` all take colors from `Host.Theme()` (`widgets.Theme`,
  starting point `widgets.Default`) rather than literal colors, so a
  module's UI matches whatever palette the host is running.

## Grouping and pagination — conventions, not widgets

Two things IDEAS.md asked for turned out not to need new drawing code:

- **Soft-button groups**: `SoftButton.Group` (int, 0 = none) draws a thin
  colored underline clustering an arbitrary subset of the 8 slots — purely
  visual, since soft-buttons have no physical per-button LED (their state
  feedback *is* the on-screen label color). Selection logic — which
  button(s) in a group are "on" — lives in the calling module via
  `module.ButtonGroup` (`Toggle`/`IsSelected`), a plain state tracker, not
  part of the ABI. It supports both semantics a module might want, picked
  per group: `Exclusive: true` for radio behavior, `false` for independent
  toggles (mute/solo, multi-select filters).
- **Whole-frame pagination** (more content than fits in one pass over the
  8 encoders/8 soft-buttons): no widget or op exists for this on purpose.
  A module already redraws its full `Draw` every frame and owns whatever
  state it wants, so "page 2 of 2" is just an int field the module
  branches on, advanced from `Handle` on an ordinary `Button` event
  (`"D-Pad left"`/`"D-Pad right"` arrive like any other button, no new
  Host API needed).

## Fonts and sizing

`Face7x13` (fixed 7x13 bitmap) is the default everywhere and stays that
way — cheap, deterministic, and the reason ASCII-only is enforceable at
all (the font has no other glyphs). Two opt-in extensions, both additive:

- **`TextScaled`** — integer nearest-neighbor upscaling of Face7x13 itself
  (each source pixel becomes an NxN block, no blur). Cheap, no new
  dependency, but only integer multiples of the one bitmap size. Used by
  `modules/remap`'s editor to make the value being dialed in read as the
  important number, not just a different color from its label.
- **`StyledText`** — antialiased outline fonts at an arbitrary point size,
  via `golang.org/x/image`'s already-vendored gofont TTFs
  (`Regular`/`Bold`/`Italic`/`BoldItalic`) through `font/opentype`. No new
  dependency, no font file to ship. Since an outline face *can* render
  more than Face7x13 can, `core/gfx/text.DrawWith`/`WidthWith` sanitize to
  ASCII themselves rather than relying on font coverage for the guarantee
  Face7x13 gave for free.

## Previewing without hardware

`cmd/screensim` renders named test scenes to PNG — no display claim, no
Push required:

```bash
go run ./cmd/screensim -list-scenes
go run ./cmd/screensim -scene <name> -out out.png
go run ./cmd/screensim -scene <name> -grid -out out.png   # overlay the 8-column guides
go run ./cmd/screensim -scene <name> -raw -out out.png    # skip the BGR565 round-trip
```

Two kinds of scene, registered in `cmd/screensim/scenes.go`:

- **Frame-mode** (`frameScenes`): builds a `*module.Frame` the way a real
  module's `Draw` would, rendered through the exact same
  `internal/renderframe.Render` path the host uses. This is what proves an
  op renders the way it would on a real run.
- **Direct-draw** (`drawScenes`): calls `core/gfx`/`core/gfx/widgets`
  straight against an `*image.NRGBA`, for trying out a widget before it
  has a `Frame` op at all.

A third registry, `cmd/screensim/modules.go`, renders an actual
compiled-in module's current `Draw` output (`-scene mod:<id>`) by `Init`ing
it against `moduletest`'s fake host — the way to check a real module's
screen without touching hardware or writing a throwaway scene by hand.

## Writing a module against this

Use the typed `Frame` methods (`f.Header(...)`, `f.List(...)`, …), never
`AppendRaw` — that exists only for the process-loader and for tests that
need to simulate an op a host doesn't know about. See
[writing-a-go-module.md](../guides/writing-a-go-module.md) for the module
contract itself; this page is specifically about what to draw with once
you're inside `Draw`.

For a process-loaded module (any language), the wire format is the same
`{"kind": "...", "params": {...}}` shape `internal/module`'s Go types
serialize to — `examples/modules/hello-py` and `hello-js` show a minimal
one, `examples/modules/beatcount-py`/`beatcount-js` a slightly larger one
using the `header` op for its title bar the same way every compiled-in Go
module does.

## What's deliberately not here yet

`ableton-push-hack`'s `keyboard-visualizer` hack does not use this system
— it draws its own thing and has no equivalent "keyboard" widget in the
catalog above. Extracting one wasn't judged worth doing speculatively (see
this package's own "add the tool, don't invent a use yet" precedent cutting
the other way: don't invent a widget for one caller either, until a second
one shows up).
