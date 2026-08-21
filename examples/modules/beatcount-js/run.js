#!/usr/bin/env node
// beatcount-js — process-loaded port of modules/beatcount (the Go original,
// push-tethered-app/modules/beatcount/beatcount.go). Counts an external
// MIDI clock and draws the current beat (1-4) across the pad grid as a
// digit.
//
// Same protocol as hello-js (see that module's comment for the full
// envelope shape); the one new thing this example demonstrates is
// NeedsMIDIIn: a "handle" notification with kind "external_midi" arrives
// whenever a byte comes in on the app's external MIDI input port, separate
// from anything Push itself sends.
//
// Critical wire detail: ExternalMIDI.Raw is a Go []byte, and Go's
// encoding/json encodes that as a base64 STRING, not a JSON array of
// numbers. {"raw": "+A=="} is a single 0xF8 clock tick. Forgetting to
// base64-decode is the easiest way to get this wrong.

const readline = require("node:readline");

const BEATS = 4; // one bar, 4/4
const TICKS_PER_QUARTER_NOTE = 24; // the MIDI clock standard, independent of tempo

const LIT_PAD = 120; // "white" pad, core/push3/colors.go

// digitBitmaps[i] is the glyph for beat i+1 (1-4), one row per byte, top of
// the digit first. Bit 7 (0x80) is column 0 (leftmost). Ported byte-for-byte
// from modules/beatcount/beatcount.go's digitBitmaps.
const DIGIT_BITMAPS = [
  [0b00011000, 0b00111000, 0b00011000, 0b00011000, 0b00011000, 0b00011000, 0b00011000, 0b01111110], // 1
  [0b00111100, 0b01100110, 0b00000110, 0b00001100, 0b00011000, 0b00110000, 0b01100000, 0b01111110], // 2
  [0b01111100, 0b00000110, 0b00000110, 0b00111100, 0b00000110, 0b00000110, 0b00000110, 0b01111100], // 3
  [0b00001100, 0b00011100, 0b00110100, 0b01100100, 0b01111110, 0b00000100, 0b00000100, 0b00001100], // 4
];

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

function notify(method, params) {
  send({ method, params });
}

// Mirrors core/push3.PadNote: note 36 is bottom-left, ascending left-to-right
// then bottom-to-top.
function padNote(col, row) {
  return 36 + row * 8 + col;
}

const state = {
  beat: 0, // 0-3
  tick: 0, // 0-(TICKS_PER_QUARTER_NOTE-1) within the current beat
  haveClock: false,
};

// Lights every pad to match DIGIT_BITMAPS[state.beat], clearing everything
// else — a full redraw, not a diff, so 64 set_pad notifications every beat
// and no risk of ever drifting from what should be showing.
function drawDigit() {
  const bitmap = DIGIT_BITMAPS[state.beat];
  for (let writtenRow = 0; writtenRow < 8; writtenRow++) {
    const physicalRow = 7 - writtenRow; // written row 0 is the top of the glyph
    const rowBits = bitmap[writtenRow];
    for (let col = 0; col < 8; col++) {
      const lit = (rowBits & (1 << (7 - col))) !== 0;
      notify("set_pad", { note: padNote(col, physicalRow), colour: lit ? LIT_PAD : 0 });
    }
  }
}

function clearGrid() {
  for (let col = 0; col < 8; col++) {
    for (let row = 0; row < 8; row++) {
      notify("set_pad", { note: padNote(col, row), colour: 0 });
    }
  }
}

function handleExternalMIDI(data) {
  const raw = Buffer.from(data.raw || "", "base64");
  if (raw.length === 0) return;
  switch (raw[0]) {
    case 0xf8: // Timing Clock
      onClock();
      break;
    case 0xfa: // Start
      reset();
      break;
  }
}

function onClock() {
  if (!state.haveClock) {
    reset();
    return;
  }
  state.tick++;
  if (state.tick < TICKS_PER_QUARTER_NOTE) return;
  state.tick = 0;
  state.beat = (state.beat + 1) % BEATS;
  drawDigit();
}

function reset() {
  state.tick = 0;
  state.beat = 0;
  state.haveClock = true;
  drawDigit();
}

function draw() {
  const black = { R: 0, G: 0, B: 0, A: 255 };
  const gray = { R: 120, G: 120, B: 120, A: 255 };
  const white = { R: 255, G: 255, B: 255, A: 255 };

  const ops = [
    { kind: "rect", params: { x: 0, y: 0, w: 960, h: 160, c: black } },
    // Same "header" op modules/beatcount (the Go original) draws its title with.
    { kind: "header", params: { y: 0, w: 960, h: 20, s: "pushapp - beat counter (out of process)" } },
  ];
  if (!state.haveClock) {
    ops.push({ kind: "text", params: { x: 8, baseline: 60, s: "waiting for an external MIDI clock...", c: gray } });
  } else {
    const text = `beat ${state.beat + 1} / ${BEATS}`;
    ops.push({ kind: "text", params: { x: 8, baseline: 60, s: text, c: white } });
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
    return;
  }

  const { method, id } = env;
  const params = env.params || {};

  switch (method) {
    case "init":
      respond(id, {});
      break;
    case "handle":
      if (params.kind === "external_midi") {
        handleExternalMIDI(params.data || {});
      }
      break;
    case "draw":
      respond(id, draw());
      break;
    case "close":
      if (state.haveClock) {
        clearGrid();
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
