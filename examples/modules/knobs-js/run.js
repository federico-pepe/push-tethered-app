#!/usr/bin/env node
// knobs-js — 8 knobs across the screen, one per encoder, each 0-100.
// Built as a quick test bench for the design system's knob rendering
// (docs/architecture/design-system.md's Knob/KnobFull rows — anti-aliased
// arc, knobStroke=2 by default as of 2026-08-22). Not a port of anything;
// exists purely to turn every encoder and eyeball the result.
//
// Same protocol as hello-js (see that module's comment for the full
// envelope shape). Track and sweep colors come from the host's Theme
// automatically — the "knob" op takes no color params of its own
// (internal/module.KnobParams), so this module never touches color.
//
// Endless-encoder convention: clamp the accumulator itself to [0,100] on
// every delta, same as modules/uidemo and modules/ui-text-demo — turning
// past a knob's limit stops there and reverses immediately, it doesn't
// wrap back to 0. See CHANGELOG.md's 2026-08-22 entry.

const readline = require("node:readline");

const NUM_KNOBS = 8;

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

function respond(id, result) {
  if (id === undefined || id === null) return;
  send({ id, result });
}

function respondError(id, message) {
  send({ id, error: message });
}

const state = {
  // value[i] is encoder i's accumulated value, clamped to [0,100] at write
  // time so a reversal past the limit responds immediately.
  value: new Array(NUM_KNOBS).fill(0),
};

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}

function handleEncoder(data) {
  const { index, delta } = data;
  if (index >= 0 && index < NUM_KNOBS) {
    state.value[index] = clamp(state.value[index] + delta, 0, 100);
  }
}

// Op shapes mirror internal/module's Go types exactly — KnobParams is
// {cx,cy,r,k:{...}} (internal/module/frame.go), and widgets.Knob (the "k"
// object's type, in ableton-push-hack) has no json tags of its own, so its
// wire keys are its literal Go field names — Label/Value/Min/Max,
// capitalized — same reasoning as color.NRGBA's R/G/B/A in the other
// example modules.
function draw() {
  const black = { R: 0, G: 0, B: 0, A: 255 };
  const gray = { R: 120, G: 120, B: 120, A: 255 };

  const ops = [
    { kind: "rect", params: { x: 0, y: 0, w: 960, h: 160, c: black } },
    { kind: "header", params: { y: 0, w: 960, h: 20, s: "pushapp - knobs-js (out of process)" } },
  ];

  const r = 30;
  const cy = 80; // vertical center of the content band between the header (0-20) and status bar (140-158)
  const spacing = 960 / NUM_KNOBS;
  for (let i = 0; i < NUM_KNOBS; i++) {
    const cx = Math.round(spacing * i + spacing / 2);
    ops.push({
      kind: "knob",
      params: {
        cx,
        cy,
        r,
        k: { Label: `ENC ${i + 1}`, Value: state.value[i], Min: 0, Max: 100 },
      },
    });
  }

  ops.push({
    kind: "statusbar",
    params: { y: 140, w: 960, h: 18, s: "turn encoders 1-8", is_error: false },
  });

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
    return;
  }

  const { method, id } = env;
  const params = env.params || {};

  switch (method) {
    case "init":
      respond(id, {});
      break;
    case "handle":
      if (params.kind === "encoder") {
        handleEncoder(params.data || {});
      }
      break;
    case "draw":
      respond(id, draw());
      break;
    case "close":
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
