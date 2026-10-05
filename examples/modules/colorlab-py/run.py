#!/usr/bin/env python3
"""colorlab-py - pick pad colors, dim shades and screen RGB on a real Push.

The pad LEDs and the screen draw the same palette index differently. This
module shows both side by side so you can tune a pair by eye.

  Encoder 1   palette index 0-127 (the "full" pad color)
  Encoder 2   dim index (the pad color to use for an empty step)
  Encoder 3-5 screen R, G, B (starts from the palette RGB)
  Encoder 6   reset the screen RGB of this index
  Save        write colors.json next to this file

Pads: bottom half = full (left) and dim (right). Top-left = checkerboard of
full and dim, like a step grid. Top-right rows, top to bottom = white,
full, dim, green. Same protocol as hello-py; stdlib only.

palette.json (from cmd/genpalette) holds the exact color of all 128 entries.
Dim shades live in the unnamed indices, so this needs core v0.2.1 or later.
Source: ableton-push-hack docs/push3-led-colors.md.
"""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
COLORS_PATH = os.path.join(HERE, "colors.json")
SAVE_CC = 82  # Save button
FRICTION = 2  # raw encoder delta per index step for encoders 1 and 2
RGB_STEP = 3

with open(os.path.join(HERE, "palette.json")) as _f:
    PALETTE = json.load(_f)["byIndex"]


def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()  # not optional: stdout is a pipe


def notify(method, params):
    send({"method": method, "params": params})


def rgba(rgb):
    return {"R": rgb[0], "G": rgb[1], "B": rgb[2], "A": 255}


def palette_rgb(i):
    e = PALETTE[i]
    return (e["r"], e["g"], e["b"])


class Lab:
    def __init__(self):
        self.index = 1
        self.dim = {}      # index -> dim palette index
        self.rgb = {}      # index -> (r, g, b) for the screen
        self.accum = {}
        self.popup = None
        self.load()

    def load(self):
        try:
            with open(COLORS_PATH) as f:
                doc = json.load(f)
            for key, entry in (doc.get("colors") or {}).items():
                i = int(key)
                if isinstance(entry.get("dim"), int) and 0 <= entry["dim"] <= 127:
                    self.dim[i] = entry["dim"]
                if isinstance(entry.get("rgb"), list) and len(entry["rgb"]) == 3:
                    self.rgb[i] = tuple(max(0, min(255, int(v))) for v in entry["rgb"])
        except (OSError, ValueError, AttributeError, TypeError):
            pass

    def save(self):
        colors = {}
        for i in sorted(set(self.dim) | set(self.rgb)):
            entry = {}
            if i in self.dim:
                entry["dim"] = self.dim[i]
            if i in self.rgb:
                entry["rgb"] = list(self.rgb[i])
            colors[str(i)] = entry
        tmp = COLORS_PATH + ".tmp"
        with open(tmp, "w") as f:
            json.dump({"version": 1, "colors": colors}, f, indent=1)
        os.replace(tmp, COLORS_PATH)
        self.popup = "Saved %d colors" % len(colors)

    def cur_dim(self):
        return self.dim.get(self.index, self.index)

    def cur_rgb(self):
        return self.rgb.get(self.index, palette_rgb(self.index))

    def steps(self, key, delta):
        """Encoder friction: whole steps from raw deltas, remainder kept."""
        acc = self.accum.get(key, 0) + delta
        n = int(acc / FRICTION)
        self.accum[key] = acc - n * FRICTION
        return n

    def encoder(self, idx, delta):
        if idx == 0:
            self.index = max(0, min(127, self.index + self.steps("i", delta)))
        elif idx == 1:
            self.dim[self.index] = max(0, min(127, self.cur_dim() + self.steps("d", delta)))
        elif idx in (2, 3, 4):
            rgb = list(self.cur_rgb())
            rgb[idx - 2] = max(0, min(255, rgb[idx - 2] + RGB_STEP * delta))
            self.rgb[self.index] = tuple(rgb)
        elif idx == 5:
            self.rgb.pop(self.index, None)

    def pads(self):
        """8x8 palette indices, row 0 = bottom."""
        full, dim = self.index, self.cur_dim()
        grid = [[0] * 8 for _ in range(8)]
        for row in range(8):
            for col in range(8):
                left = col < 4
                if row < 4:
                    grid[row][col] = full if left else dim
                elif left:
                    grid[row][col] = full if (row + col) % 2 == 0 else dim
                else:
                    grid[row][col] = (120, full, dim, 126)[7 - row]
        return grid


def rect(x, y, w, h, c):
    return {"kind": "rect", "params": {"x": x, "y": y, "w": w, "h": h, "c": c}}


def text(x, baseline, s, c):
    return {"kind": "text", "params": {"x": x, "baseline": baseline, "s": s, "c": c}}


def draw(lab):
    white, gray = rgba((255, 255, 255)), rgba((120, 120, 120))
    full, dim, rgb = lab.index, lab.cur_dim(), lab.cur_rgb()
    ops = [
        rect(0, 0, 960, 160, rgba((0, 0, 0))),
        text(8, 14, "COLOR LAB  index %d  %s" % (full, PALETTE[full]["name"]), white),
        text(8, 152, "Enc1 index  Enc2 dim  Enc3-5 screen R G B  Enc6 reset  Save = colors.json", gray),
        rect(8, 36, 150, 56, rgba(palette_rgb(full))),
        rect(176, 36, 150, 56, rgba(rgb)),
        rect(344, 36, 150, 56, rgba(palette_rgb(dim))),
        text(8, 108, "PALETTE %d %d %d" % palette_rgb(full), gray),
        text(176, 108, "SCREEN %d %d %d" % rgb, white),
        text(344, 108, "DIM PAD %d %s" % (dim, PALETTE[dim]["name"]), gray),
        text(8, 130, "Match the middle swatch to how the pads look.", gray),
    ]
    if lab.popup:
        ops.append(text(560, 14, lab.popup, white))
    return {"ops": ops, "failed": 0}


def relight(last, grid):
    if grid == last:
        return last
    for row in range(8):
        for col in range(8):
            notify("set_pad", {"note": 36 + row * 8 + col, "colour": grid[row][col]})
    return grid


def main():
    lab, last = Lab(), None
    for line in sys.stdin:
        try:
            env = json.loads(line)
        except ValueError:
            continue
        method, id_ = env.get("method"), env.get("id")
        params = env.get("params") or {}
        if method == "init":
            send({"id": id_, "result": {}})
            notify("set_button", {"cc": SAVE_CC, "brightness": 122})
        elif method == "handle":
            kind, data = params.get("kind"), params.get("data") or {}
            if kind == "encoder" and data.get("delta") and data.get("index") is not None:
                lab.popup = None
                lab.encoder(data["index"], data["delta"])
            elif kind == "button" and data.get("name") == "Save" and data.get("pressed"):
                lab.save()
        elif method == "draw":
            last = relight(last, lab.pads())
            send({"id": id_, "result": draw(lab)})
        elif method == "close":
            for row in range(8):
                for col in range(8):
                    notify("set_pad", {"note": 36 + row * 8 + col, "colour": 0})
            notify("set_button", {"cc": SAVE_CC, "brightness": 0})
            send({"id": id_, "result": {}})
            break
        elif id_ is not None and method is not None:
            send({"id": id_, "error": "unknown method %r" % method})


if __name__ == "__main__":
    main()
