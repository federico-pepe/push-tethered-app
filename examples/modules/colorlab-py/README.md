# colorlab-py

A tool module. It shows one Push palette color three ways, so you can tune
colors by eye on a real device. The pad LEDs and the screen draw the same
palette index differently. See the addendum in
[push3-led-colors.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/push3-led-colors.md) in `ableton-push-hack`.

## Use

| Control | Action |
|---------|--------|
| Encoder 1 | Palette index 0-127 (the full pad color) |
| Encoder 2 | Dim index: the pad shade for an empty step |
| Encoders 3-5 | Screen R, G and B. They start from the palette RGB |
| Encoder 6 | Reset the screen RGB of this index |
| Save | Write `colors.json` next to `run.py` |

The pads show the full color on the left and the dim color on the right in the
bottom half. The top-left is a checkerboard of both, like a step grid. The
top-right shows white, full, dim and green from top to bottom. The screen
shows three swatches: the palette RGB, your screen RGB and the dim RGB.
Change the middle swatch until it looks like the pads.

`colors.json` looks like this. A module can read it at start up:

```json
{"version": 1, "colors": {"7": {"dim": 78, "rgb": [247, 227, 62]}}}
```

The `dim` key is only there when you changed it, and `rgb` only when you
changed the screen color.

The file loads again when the module starts, so you can continue later.

## Files

- `hardware-palette.json`: all 128 SysEx-verified LED entries. It is not
  `palette.json`. `cmd/genpalette` writes only the 90 named entries, and
  dim shades are in the unnamed indices. Build this file again with
  `scripts/gen_palette.py` from `pta-module-bcaseq`. This module does not use
  a `palette.json`.
- `colors.json`: your saved values. Created by Save.

```bash
go run ./cmd/pushapp -install examples/modules/colorlab-py
go run ./cmd/pushapp -module colorlab-py
```
