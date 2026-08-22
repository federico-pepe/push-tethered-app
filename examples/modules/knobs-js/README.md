# knobs-js

8 knobs across the screen, one per encoder, each ranging 0-100 and
vertically centered in the content band between the header and status
bar. Built as a quick test bench for the design system's knob rendering
(anti-aliased arc, `knobStroke = 2`) — turn any encoder and watch its
knob track live.

Demonstrates the endless-encoder convention: each value clamps at its
accumulator, so turning past 0 or 100 stops there and reverses
immediately instead of wrapping back around. Track/sweep colors come from
the host's `Theme` automatically — the `"knob"` op takes no color params
of its own.

```bash
go run ./cmd/pushapp -install examples/modules/knobs-js
go run ./cmd/pushapp -module knobs-js
```
