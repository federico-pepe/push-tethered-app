# hello-js

The same module as `hello-py`, in Node — proving the process-loader
protocol isn't Python-specific. Press a pad, the screen shows which one
and a pad lights up to match. Nothing more.

Its two screen colors (white, black) come from `palette.json`
(`cmd/genpalette`-generated from `core/push3.Palette`) via `paletteColor`
and `paletteById`, one of each lookup style — see
[writing-a-process-module.md](../../../docs/guides/writing-a-process-module.md#colors).

```bash
go run ./cmd/pushapp -install examples/modules/hello-js
go run ./cmd/pushapp -module hello-js
```
