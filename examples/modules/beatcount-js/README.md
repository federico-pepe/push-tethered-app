# beatcount-js

The same module as `beatcount-py`, in Node — process-loaded port of the Go
`modules/beatcount`, counting an external MIDI clock and drawing the
current beat (1-4) across the pad grid as a digit.

```bash
go run ./cmd/pushapp -install examples/modules/beatcount-js
go run ./cmd/pushapp -module beatcount-js -ext-midi-in "<your clock source>"
```
