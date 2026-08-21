# Frame.Text scale parameter

**Status: done (2026-08-21).** Raised while building the on-device remap
editor ([2026-08-17-module-host.md](2026-08-17-module-host.md)'s follow-on
work, not its own plan file) — the editor wanted the current field's value
drawn larger than its label, and there was no way to do that. Implemented
per the plan below, verified with `cmd/screensim -scene text-scale`
(pixel-doubling proof also lives in `core/gfx/text/text_test.go`), and
`modules/remap`'s editor now uses it. Open questions below settled: 2x for
remap, `Text`/`TextScaled` only (not `Header`/`KVRow`). Left as historical
reasoning — see [2026-08-21-design-system-screensim.md](2026-08-21-design-system-screensim.md)
for the broader design-system work this was folded into.

## Context

`modules/remap`'s editing screen shows TYPE/CHAN/VALUE/MIN/MAX labels with
the current value centered below each. The value is the thing the user is
actively dialing in with an encoder — it should read as the important
number on screen, bigger than the label above it. Today it's only
differentiated by color (white value vs gray label), because every text
primitive in the stack draws at a single fixed size.

## Why this doesn't already exist

Traced the whole text path while working on the remap editor:

- `core/gfx/text.Draw` (`ableton-push-hack/core/gfx/text/text.go`) draws
  with `golang.org/x/image/font/basicfont.Face7x13` — a fixed 7×13 bitmap
  face, no size parameter, no alternate face wired in.
- `core/gfx/text.Width` is `len(s) * 7`, hardcoded to that one face.
- `internal/module.Frame.Text` / `TextParams`
  (`push-tethered-app/internal/module/frame.go:109,193`) and the `"text"`
  op renderer (`internal/host/render.go:142`) both pass straight through to
  `text.Draw` — no scale field anywhere in the op payload.
- Every module that wants emphasis today fakes it with color or spacing
  (`modules/remap`, `modules/monitor`) — there's no precedent for size-based
  emphasis to follow.

## Approach

`Face7x13` is a monochrome bitmap font (no anti-aliasing), so integer
nearest-neighbor upscaling reproduces it faithfully — each source pixel
becomes an N×N block, no blur, and it still reads as "the same font, just
bigger," consistent with Push's low-res LCD look. No new font/rasterizer
(e.g. pulling in freetype for a TTF face) needed.

1. **`core/gfx/text` (ableton-push-hack, shared via `replace`):** add
   `DrawScaled(img *image.NRGBA, x, baseline, scale int, s string, col color.NRGBA)`
   and `WidthScaled(s string, scale int) int` (`= Width(s) * scale`).
   `DrawScaled` renders the string at 1x into a small temporary
   `image.NRGBA` sized to the glyph run's bounding box (use
   `basicfont.Face7x13.Metrics()` for ascent/descent rather than guessing),
   then blits it into `img` at `(x, baseline)` with each source pixel
   expanded to a `scale`×`scale` block. `scale == 1` should be a thin
   wrapper over the existing `Draw`/`Width` (or have `Draw`/`Width` become
   `scale=1` calls into the new functions) — do not duplicate the metrics
   logic.
   Add `TestDrawScaledMatchesPixelDoubledOutput`-style coverage in
   `core/gfx/text/text_test.go`: render at 1x and at 2x, assert the 2x
   output is exactly the 1x output with each pixel expanded 2×2 (not just
   "looks bigger") — this is what proves nearest-neighbor is being used
   faithfully, not some blurring resize.
2. **`internal/module` (push-tethered-app):** add a `Scale int
   json:"scale,omitempty"` field to `TextParams` — additive, so `scale: 0`
   (the zero value, matching every `TextParams` ever serialized before this
   change) must mean "1x," not "invisible." Add
   `Frame.TextScaled(x, baseline int, s string, c color.NRGBA, scale int)`
   alongside the existing `Text` (which keeps calling the plain path, or
   becomes `f.TextScaled(x, baseline, s, c, 1)` internally — either is fine,
   don't change its signature). No new op kind: folding into the existing
   `"text"` op means `SupportedOps()`, `moduletest`'s fake renderer op list,
   and `internal/host/procmod`'s wire schema all need nothing added, only
   the one new optional field threaded through.
3. **`internal/host/render.go`:** the `"text"` op handler
   (`render.go:142`) reads `v.Scale`, treats `0` as `1`, calls
   `text.DrawScaled` instead of `text.Draw`.
4. **`internal/module/moduletest`:** `ascii.go`'s text-op rendering path
   (used by `moduletest.NonASCIIStrings` and friends) needs the same
   `scale-0-means-1` handling so scaled text still round-trips through
   tests without hardware.
5. **`modules/remap`:** once available, switch the editing screen's value
   row to `f.TextScaled(x, y, values[i], t.White, 2)` (or `3` — pick by eye
   on hardware) instead of the current same-size-different-color
   workaround, and use `text.WidthScaled` for centering math instead of
   `text.Width`.

## Open questions, decide when picked up

- **Which scale factors to support.** Only integers make sense for
  nearest-neighbor (no blur) — is 2x enough, or does anything want 3x+?
  Don't build more than remap's actual use needs until a second caller
  shows up.
- **Does this belong on other widgets too** (`Header`, `KVRow` values), or
  is `Text`/`TextScaled` alone sufficient and higher-level widgets stay
  fixed-size? Lean toward "just `Text` for now" — same reasoning CLAUDE.md
  gives elsewhere: don't build for hypothetical future callers.
- **Cross-repo scope.** Same shape as the `BotStrip` widening done in this
  session: `core/gfx/text` lives in `ableton-push-hack`, is additive only
  (no signature changes to `Draw`/`Width`), so `hacks/push-manager` (the
  only other consumer of that package) needs zero changes — lower risk
  than the `BotStrip` change, which did force `Panel` interface edits
  there.

## Verification

- `core/gfx/text` unit tests (pixel-doubling assertion above), `go build
  ./... && go test ./...` in `core/`.
- `push-tethered-app`: `go build ./... && go vet ./... && go test ./...`,
  plus `moduletest.NonASCIIStrings` coverage on a module using
  `TextScaled`.
- Hardware: run `modules/remap`'s editing screen, eyeball that the scaled
  value text is crisp (no blur) and correctly centered under its column,
  same hardware-test-loop discipline as everything else in this project
  (Live closed, eyeball the actual screen — exit 0 isn't proof).
