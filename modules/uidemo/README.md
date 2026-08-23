# ui-demo

Exercises every widget in the design system, each driven by a real
hardware control, one page per widget cluster. It exists to verify the
design system on real Push hardware — `core/gfx/widgets`' own unit tests
and `cmd/screensim`'s scenes only check that a widget renders correctly
given hand-built input; this module proves an encoder turn, a pad press,
or a D-Pad press actually reaches the widget and changes what's on screen
the way a person expects.

Controls:

- D-Pad left/right: change page
- Encoders 1-8: live values feeding whichever widgets the current page
  shows (a knob, a meter, a fader, an envelope point, list cursors)
- The 8 pads: toggle cells on the pad-grid page, mirrored to their own
  LEDs via `Host.SetPad`
- The 8 under-screen soft-buttons: toggle an exclusive (radio) group and
  an independent-toggle group on the buttons page; the 8th button (PINK)
  shows a `SoftButton.Color` palette override next to the two State-driven
  groups

Not exercised: the jog wheel, the touch strip, MPE, and the top-row
buttons — none of those have a widget in the catalog yet to verify.

```bash
go run ./cmd/pushapp -module ui-demo
```
