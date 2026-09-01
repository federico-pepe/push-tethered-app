#!/usr/bin/env node
// border-chase-js — lights the outer border of the pad grid in a clockwise
// chase, one pad at a time, each with the next palette colour index (1, 2,
// 3, ...). Fixed sequence, not derived from the grid at runtime, so the
// exact pad order is easy to read and check against the spec:
//
//   38 -> 37 -> 36 -> 44 -> 52 -> 60 -> 68 -> 76 -> 84 -> 92
//      -> 93 -> 94 -> 95 -> 96 -> 97 -> 98 -> 99
//      -> 91 -> 83 -> 75 -> 67 -> 59 -> 51 -> 43
//      -> 41 -> 42
//
// Starts at note 38 (row 0, col 2), goes left along the bottom row to col 0,
// up the left column, right along the top row, down the right column, then
// left along the bottom row again toward 41 (col 5) and 42 (col 6) — one
// full lap minus 39, 40, 38 needed to close the loop back to the start. The
// last two, 41 and 42, light out of grid order so 41 gets colour 26 and 42
// gets colour 25 (swapped from strict step order, per spec).
//
// Protocol: examples/modules/hello-js/run.js documents the JSON-over-stdio
// contract this follows (init/handle/draw/close, notify("set_pad", ...)).

const readline = require("node:readline");

const BORDER_NOTES = [
  38, 37, 36,
  44, 60, 52, 68, 76, 84, 92,
  93, 94, 95, 96, 97, 98, 99,
  91, 83, 75, 67, 59, 51, 43,
  41, 42,
];

const STEP_MS = 120; // time between pads lighting up

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

function respond(id, result) {
  if (id === undefined || id === null) return;
  send({ id, result });
}

function notify(method, params) {
  send({ method, params });
}

const state = {
  litNotes: new Set(),
  step: 0,
  timer: null,
};

function clearAllPads() {
  for (const note of state.litNotes) {
    notify("set_pad", { note, colour: 0 });
  }
  state.litNotes.clear();
}

function startChase() {
  clearAllPads();
  state.step = 0;
  if (state.timer) clearInterval(state.timer);
  state.timer = setInterval(() => {
    if (state.step >= BORDER_NOTES.length) {
      clearInterval(state.timer);
      state.timer = null;
      return;
    }
    const note = BORDER_NOTES[state.step];
    const colour = state.step + 1; // palette index, starts at 1
    notify("set_pad", { note, colour });
    state.litNotes.add(note);
    state.step++;
  }, STEP_MS);
}

function handlePad(data) {
  const { pressed } = data;
  if (pressed) startChase(); // press any pad to replay the chase
}

function draw() {
  const white = { R: 255, G: 255, B: 255, A: 255 };
  const black = { R: 0, G: 0, B: 0, A: 255 };

  const done = state.timer === null && state.step >= BORDER_NOTES.length;
  const text = done
    ? "chase done - press a pad to replay"
    : `lighting pad ${state.step}/${BORDER_NOTES.length}`;

  return {
    ops: [
      { kind: "rect", params: { x: 0, y: 0, w: 960, h: 160, c: black } },
      {
        kind: "header",
        params: { y: 0, w: 960, h: 20, s: "pushapp - border-chase-js" },
      },
      { kind: "text", params: { x: 8, baseline: 80, s: text, c: white } },
    ],
    failed: 0,
  };
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });

rl.on("line", (line) => {
  line = line.trim();
  if (!line) return;

  let env;
  try {
    env = JSON.parse(line);
  } catch {
    return;
  }

  const { method, id } = env;
  const params = env.params || {};

  switch (method) {
    case "init":
      respond(id, {});
      startChase();
      break;
    case "handle":
      if (params.kind === "pad") {
        handlePad(params.data || {});
      }
      break;
    case "draw":
      respond(id, draw());
      break;
    case "close":
      if (state.timer) clearInterval(state.timer);
      clearAllPads();
      respond(id, {});
      rl.close();
      process.exit(0);
      break;
    default:
      if (id !== undefined && id !== null) {
        send({ id, error: `unknown method ${JSON.stringify(method)}` });
      }
  }
});
