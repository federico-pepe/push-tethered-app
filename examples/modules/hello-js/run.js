#!/usr/bin/env node
// hello-js — the same module as hello-py, in Node, proving the process-loader
// protocol (plans/2026-08-18-process-loader.md) is not Python-specific.
// Deliberately not a port of anything richer: press a pad, the screen shows
// which one and a pad lights up to match, nothing more.
//
// Protocol: one JSON object per line, both directions.
//   request:  {"id": N, "method": "...", "params": {...}}   (no id = notification)
//   response: {"id": N, "result": {...}}  or  {"id": N, "error": "..."}
//
// Node's stdout to a pipe is written synchronously on POSIX, so there is no
// Python-style "flush or it hangs" trap here — but the discipline (one write
// per line, immediately, no batching) is still the contract to follow in any
// language.

const readline = require("node:readline");

// 11, matching core/push3/colors.go's NamedColors["green"] (#34C216). That
// file itself was wrong until 2026-08-18 — it claimed "green" = 22 under an
// assumption (inherited from Push 2's colors.pyc) that only even velocities
// carry a real colour. A live SysEx query of Push 3's own palette
// (ableton-push-hack/docs/push3-led-colors.md) shows every one of the 128
// raw velocities is a distinct, real colour with no gaps; colors.go has
// since been corrected to match. This module is what found the original
// bug: it lit pads pink instead of green on real hardware.
const PAD_LIT_COLOUR = 11; // green palette index — see core/push3/colors.go

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

function respond(id, result) {
  if (id === undefined || id === null) return; // a notification wants no reply
  send({ id, result });
}

function respondError(id, message) {
  send({ id, error: message });
}

function notify(method, params) {
  send({ method, params });
}

const state = {
  lastNote: null,
  lastCoord: null,
  litNote: null,
};

function handlePad(data) {
  const { note, pressed, col, row } = data;

  if (state.litNote !== null && state.litNote !== note) {
    // Release the previous pad before lighting a new one — one lit pad at a
    // time, so a fast run across pads never leaves a trail behind.
    notify("set_pad", { note: state.litNote, colour: 0 });
    state.litNote = null;
  }

  if (pressed) {
    state.lastNote = note;
    state.lastCoord = [col, row];
    state.litNote = note;
    notify("set_pad", { note, colour: PAD_LIT_COLOUR });
  } else if (state.litNote === note) {
    notify("set_pad", { note, colour: 0 });
    state.litNote = null;
  }
}

// Op shapes mirror internal/module's Go types exactly (the ABI, not a JS
// convention): colour is {"R":.,"G":.,"B":.,"A":.}, matching Go's
// image/color.NRGBA JSON encoding (capitalised — that stdlib type has no json
// tags of its own).
function draw() {
  const white = { R: 255, G: 255, B: 255, A: 255 };
  const gray = { R: 120, G: 120, B: 120, A: 255 };
  const black = { R: 0, G: 0, B: 0, A: 255 };

  const ops = [
    { kind: "rect", params: { x: 0, y: 0, w: 960, h: 160, c: black } },
    {
      kind: "text",
      params: { x: 8, baseline: 20, s: "pushapp - hello-js (out of process)", c: gray },
    },
  ];

  if (state.lastNote === null) {
    ops.push({ kind: "text", params: { x: 8, baseline: 80, s: "press a pad", c: white } });
  } else {
    const [col, row] = state.lastCoord;
    const text = `pad ${state.lastNote}  col ${col + 1} row ${row + 1}`;
    ops.push({ kind: "text", params: { x: 8, baseline: 80, s: text, c: white } });
  }

  return { ops, failed: 0 };
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });

rl.on("line", (line) => {
  line = line.trim();
  if (!line) return;

  let env;
  try {
    env = JSON.parse(line);
  } catch {
    return; // malformed line: skip it, matching the host's own tolerance
  }

  const { method, id } = env;
  const params = env.params || {};

  switch (method) {
    case "init":
      respond(id, {});
      break;
    case "handle":
      if (params.kind === "pad") {
        handlePad(params.data || {});
      }
      // buttons/encoders/touch/expression: nothing to do in this example
      break;
    case "draw":
      respond(id, draw());
      break;
    case "close":
      if (state.litNote !== null) {
        notify("set_pad", { note: state.litNote, colour: 0 });
      }
      respond(id, {});
      rl.close();
      process.exit(0);
      break;
    default:
      if (id !== undefined && id !== null) {
        respondError(id, `unknown method ${JSON.stringify(method)}`);
      }
  }
});
