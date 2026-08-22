# hello-py

The smallest module that isn't Go. Proves the process-loader protocol
end to end from a language with no dependency on this repo at all: stdlib
only, no `pip install`. Press any pad; the screen shows which one and a
pad lights up to match.

Only ever receives requests (`init`, `handle`, `draw`, `close`) and sends
one notification (`set_pad`). Doesn't need `"needs_midi_out"` in
`manifest.json`.

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```
