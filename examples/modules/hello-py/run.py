#!/usr/bin/env python3
"""hello-py — the smallest module that isn't Go.

Proves the process-loader protocol (plans/2026-08-18-process-loader.md) end
to end from a language with no dependency on this repo at all: stdlib only,
no pip install. Press any pad; the screen shows which one and a pad lights up
to match. That's it — a monitor-style module would be a much longer read for
no extra protocol coverage.

The protocol is one JSON object per line on stdin (host -> module) and stdout
(module -> host). Both directions use the same envelope shape:

    request:  {"id": N, "method": "...", "params": {...}}   (id omitted = notification, no reply expected/sent)
    response: {"id": N, "result": {...}}  or  {"id": N, "error": "..."}

This module only ever RECEIVES requests (init, handle, draw, close) and
SENDS one notification (set_pad) to light the pad it's currently
highlighting. It never calls send_cc/send_note — that needs "needs_midi_out":
true in manifest.json, which this example deliberately doesn't set.

Critical detail, easy to get wrong in any language: flush stdout after every
line. The host reads one line at a time and blocks waiting for it; a module
that only flushes on buffer-full (Python's default when stdout isn't a
terminal) looks like it's hanging.
"""

import json
import sys

# Palette index, not RGB — see core/push3/colors.go in the Go repo. This is
# what a LED colour argument means; screen colours (the "theme" the host
# sends on init) are separate and are plain [R,G,B,A].
#
# 11, matching colors.go's NamedColors["green"] (#34C216). That file itself
# was wrong until 2026-08-18 — it claimed "green" = 22 under an assumption
# (inherited from Push 2's colors.pyc) that only even velocities carry a
# real colour. A live SysEx query of Push 3's own palette
# (ableton-push-hack/docs/push3-led-colors.md) shows every one of the 128
# raw velocities is a distinct, real colour with no gaps; colors.go has
# since been corrected to match. This module is what found the original
# bug: it lit pads pink instead of green on real hardware.
PAD_LIT_COLOUR = 11  # green


def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()  # see the module docstring: this is not optional


def respond(id_, result):
    if id_ is None:
        return  # the caller sent a notification; nothing wants a reply
    send({"id": id_, "result": result})


def notify(method, params):
    send({"method": method, "params": params})


class State:
    def __init__(self):
        self.last_note = None
        self.last_coord = None
        self.lit_note = None


def handle_pad(state, data):
    note = data.get("note")
    pressed = data.get("pressed")
    col = data.get("col")
    row = data.get("row")

    if state.lit_note is not None and state.lit_note != note:
        # Only one pad lit at a time — release the previous one so a fast
        # drum-roll across pads doesn't leave a trail of stuck LEDs.
        notify("set_pad", {"note": state.lit_note, "colour": 0})
        state.lit_note = None

    if pressed:
        state.last_note = note
        state.last_coord = (col, row)
        state.lit_note = note
        notify("set_pad", {"note": note, "colour": PAD_LIT_COLOUR})
    elif state.lit_note == note:
        notify("set_pad", {"note": note, "colour": 0})
        state.lit_note = None


def draw(state):
    """Builds one frame's display list. Op shapes mirror internal/module's
    Go types exactly — this is the ABI, not a Python-specific format. Colour
    is {"R":.,"G":.,"B":.,"A":.}, matching Go's image/color.NRGBA JSON
    encoding (capitalised, no json tags on that stdlib type)."""
    white = {"R": 255, "G": 255, "B": 255, "A": 255}
    black = {"R": 0, "G": 0, "B": 0, "A": 255}

    ops = [
        {"kind": "rect", "params": {"x": 0, "y": 0, "w": 960, "h": 160, "c": black}},
        # "header" draws a themed filled title bar — the same op the compiled-in
        # Go modules use (internal/module.Frame.Header), so an out-of-process
        # module in any language reads as belonging to the same app rather than
        # a lesser-looking guest. Op shapes mirror internal/module's Go types
        # exactly; see docs/guides/writing-a-process-module.md.
        {"kind": "header", "params": {"y": 0, "w": 960, "h": 20, "s": "pushapp - hello-py (out of process)"}},
    ]

    if state.last_note is None:
        ops.append({"kind": "text", "params": {"x": 8, "baseline": 80, "s": "press a pad", "c": white}})
    else:
        col, row = state.last_coord
        text = "pad %d  col %d row %d" % (state.last_note, col + 1, row + 1)
        ops.append({"kind": "text", "params": {"x": 8, "baseline": 80, "s": text, "c": white}})

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
            continue  # malformed line: skip it, matching the host's own tolerance

        method = env.get("method")
        id_ = env.get("id")
        params = env.get("params") or {}

        if method == "init":
            respond(id_, {})
        elif method == "handle":
            kind = params.get("kind")
            data = params.get("data") or {}
            if kind == "pad":
                handle_pad(state, data)
            # buttons/encoders/touch/expression: nothing to do in this example
        elif method == "draw":
            respond(id_, draw(state))
        elif method == "close":
            if state.lit_note is not None:
                notify("set_pad", {"note": state.lit_note, "colour": 0})
            respond(id_, {})
            break
        elif id_ is not None:
            respond_error(id_, "unknown method %r" % method)


def respond_error(id_, message):
    send({"id": id_, "error": message})


if __name__ == "__main__":
    main()
