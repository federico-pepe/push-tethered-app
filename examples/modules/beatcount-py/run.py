#!/usr/bin/env python3
"""beatcount-py — process-loaded port of modules/beatcount (the Go original,
push-tethered-app/modules/beatcount/beatcount.go). Counts an external MIDI
clock and draws the current beat (1-4) across the pad grid as a digit.

Same protocol as hello-py (see that module's docstring for the full
envelope shape); the one new thing this example demonstrates is
NeedsMIDIIn: a "handle" notification with "kind": "external_midi" arrives
whenever a byte comes in on the app's external MIDI input port, separate
from anything Push itself sends.

Critical wire detail: ExternalMIDI.Raw is a Go []byte, and Go's
encoding/json encodes that as a base64 STRING, not a JSON array of numbers.
"data": {"raw": "+A=="} is a single 0xF8 clock tick. Forgetting to
base64-decode is the easiest way to get this wrong.
"""

import base64
import json
import sys

BEATS = 4  # one bar, 4/4
TICKS_PER_QUARTER_NOTE = 24  # the MIDI clock standard, independent of tempo

LIT_PAD = 120  # "white" pad, core/push3/colors.go

# digitBitmaps[i] is the glyph for beat i+1 (1-4), one row per byte, top of
# the digit first. Bit 7 (0x80) is column 0 (leftmost). Ported byte-for-byte
# from modules/beatcount/beatcount.go's digitBitmaps.
DIGIT_BITMAPS = [
    [  # 1
        0b00011000,
        0b00111000,
        0b00011000,
        0b00011000,
        0b00011000,
        0b00011000,
        0b00011000,
        0b01111110,
    ],
    [  # 2
        0b00111100,
        0b01100110,
        0b00000110,
        0b00001100,
        0b00011000,
        0b00110000,
        0b01100000,
        0b01111110,
    ],
    [  # 3
        0b01111100,
        0b00000110,
        0b00000110,
        0b00111100,
        0b00000110,
        0b00000110,
        0b00000110,
        0b01111100,
    ],
    [  # 4
        0b00001100,
        0b00011100,
        0b00110100,
        0b01100100,
        0b01111110,
        0b00000100,
        0b00000100,
        0b00001100,
    ],
]


def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()  # not optional — see hello-py's docstring


def respond(id_, result):
    if id_ is None:
        return
    send({"id": id_, "result": result})


def respond_error(id_, message):
    send({"id": id_, "error": message})


def notify(method, params):
    send({"method": method, "params": params})


def pad_note(col, row):
    """Mirrors core/push3.PadNote: note 36 is bottom-left, ascending
    left-to-right then bottom-to-top."""
    return 36 + row * 8 + col


class State:
    def __init__(self):
        self.beat = 0  # 0-3
        self.tick = 0  # 0-(TICKS_PER_QUARTER_NOTE-1) within the current beat
        self.have_clock = False


def draw_digit(state):
    """Lights every pad to match DIGIT_BITMAPS[state.beat], clearing
    everything else — a full redraw, not a diff, so 64 SetPad notifications
    every beat and no risk of ever drifting from what should be showing."""
    bitmap = DIGIT_BITMAPS[state.beat]
    for written_row in range(8):
        physical_row = 7 - written_row  # written row 0 is the top of the glyph
        row_bits = bitmap[written_row]
        for col in range(8):
            lit = (row_bits & (1 << (7 - col))) != 0
            colour = LIT_PAD if lit else 0
            notify("set_pad", {"note": pad_note(col, physical_row), "colour": colour})


def clear_grid():
    for col in range(8):
        for row in range(8):
            notify("set_pad", {"note": pad_note(col, row), "colour": 0})


def handle_external_midi(state, data):
    raw = base64.b64decode(data.get("raw", ""))
    if len(raw) == 0:
        return
    first = raw[0]
    if first == 0xF8:  # Timing Clock
        on_clock(state)
    elif first == 0xFA:  # Start
        reset(state)


def on_clock(state):
    if not state.have_clock:
        reset(state)
        return
    state.tick += 1
    if state.tick < TICKS_PER_QUARTER_NOTE:
        return
    state.tick = 0
    state.beat = (state.beat + 1) % BEATS
    draw_digit(state)


def reset(state):
    state.tick = 0
    state.beat = 0
    state.have_clock = True
    draw_digit(state)


def draw(state):
    black = {"R": 0, "G": 0, "B": 0, "A": 255}
    gray = {"R": 120, "G": 120, "B": 120, "A": 255}
    white = {"R": 255, "G": 255, "B": 255, "A": 255}

    ops = [
        {"kind": "rect", "params": {"x": 0, "y": 0, "w": 960, "h": 160, "c": black}},
        # Same "header" op modules/beatcount (the Go original) draws its title
        # with — see hello-py's draw() for why this matters for a process module.
        {"kind": "header", "params": {"y": 0, "w": 960, "h": 20, "s": "pushapp - beat counter (out of process)"}},
    ]
    if not state.have_clock:
        ops.append({"kind": "text", "params": {"x": 8, "baseline": 60, "s": "waiting for an external MIDI clock...", "c": gray}})
    else:
        text = "beat %d / %d" % (state.beat + 1, BEATS)
        ops.append({"kind": "text", "params": {"x": 8, "baseline": 60, "s": text, "c": white}})

    return {"ops": ops, "failed": 0}


def main():
    state = State()
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            env = json.loads(line)
        except json.JSONDecodeError:
            continue

        method = env.get("method")
        id_ = env.get("id")
        params = env.get("params") or {}

        if method == "init":
            respond(id_, {})
        elif method == "handle":
            kind = params.get("kind")
            data = params.get("data") or {}
            if kind == "external_midi":
                handle_external_midi(state, data)
        elif method == "draw":
            respond(id_, draw(state))
        elif method == "close":
            if state.have_clock:
                clear_grid()
            respond(id_, {})
            break
        elif id_ is not None:
            respond_error(id_, "unknown method %r" % method)


if __name__ == "__main__":
    main()
