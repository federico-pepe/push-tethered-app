# ui-text-demo

A live font-tuning bench: every encoder drives one text-rendering
parameter, so a change can be dialed in and eyeballed on real hardware
instead of guessing constants and rebuilding `cmd/screensim` scenes each
time.

Controls:

- Encoder 1: face — Basic (Tamzen, fixed size/weight) vs Styled
  (Helvetica Neue, honors weight/size below)
- Encoder 2: weight — Regular/Bold/Italic/BoldItalic (Styled face only)
- Encoder 3: size in points (Styled face only) — clamps at its limits
- Encoder 4: color — cycles the 0-127 Push hardware LED palette
  (`core/push3.Palette`/`ColorForIndex`) instead of dialing raw RGB
- Encoder 5: left margin (x) — clamps at its limits
- Encoder 6: vertical offset from screen center (y) — clamps at its limits
- Encoders 7-8: unused, reserved for future parameters
- Bottom soft-buttons 1-4: pick a sample string
- Bottom soft-button 8: reset every parameter to its default

```bash
go run ./cmd/pushapp -module ui-text-demo
```
