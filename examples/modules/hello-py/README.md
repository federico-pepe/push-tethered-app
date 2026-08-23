# hello-py

The smallest module that isn't Go. Proves the process-loader protocol
end to end from a language with no dependency on this repo at all: stdlib
only, no `pip install`. Press any pad; the screen shows which one and a
pad lights up to match.

Only ever receives requests (`init`, `handle`, `draw`, `close`) and sends
one notification (`set_pad`). Doesn't need `"needs_midi_out"` in
`manifest.json`.

Its two screen colors (white, black) come from `palette.json`
(`cmd/genpalette`-generated from `core/push3.Palette`) via
`palette_color` and `palette_by_id`, one of each lookup style — see
[writing-a-process-module.md](../../../docs/guides/writing-a-process-module.md#colors).

```bash
go run ./cmd/pushapp -install examples/modules/hello-py
go run ./cmd/pushapp -module hello-py
```
