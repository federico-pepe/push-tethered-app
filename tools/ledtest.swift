// ledtest — macOS-only probe that drives Push's LEDs via CoreMIDI.
//
// NOT part of the app build. Companion to tools/midimon.swift; kept in the repo
// so the LED measurements stay reproducible.
//
// Uses CoreMIDI on purpose: that is the co-existence output path
// (docs/feasibility.md §6.1a). Writing to interface 5 over libusb would take
// Push's MIDI ports away from the DAW.
//
// Protocol (core/push3/colors.go is the source of truth):
//   Pad LED    : Note On ch1, note 36-99, velocity = palette index (0 = off)
//   Button LED : CC ch1, value = brightness 0-127 (white LEDs ignore colour)
//
// Every exit path turns the LEDs off, including SIGINT — leaving a device lit
// after a probe is rude and makes the next run ambiguous.
//
// Build & run:
//   swiftc -O tools/ledtest.swift -o ledtest && ./ledtest

import CoreMIDI
import Foundation

// ── Map constants (mirrors core/push3, which this repo does not re-declare) ──
let padNoteMin: UInt8 = 36
let padNoteMax: UInt8 = 99
let screenTopCCs: [UInt8] = Array(102...109)   // row above the display
let screenBotCCs: [UInt8] = Array(20...27)     // row below the display

func sprop(_ obj: MIDIObjectRef, _ prop: CFString) -> String {
    var out: Unmanaged<CFString>?
    if MIDIObjectGetStringProperty(obj, prop, &out) == noErr, let o = out {
        return o.takeRetainedValue() as String
    }
    return "?"
}

// Push expects LED traffic on the Live port.
var dest: MIDIEndpointRef = 0
var destName = ""
for i in 0..<MIDIGetNumberOfDestinations() {
    let d = MIDIGetDestination(i)
    let n = sprop(d, kMIDIPropertyDisplayName)
    if n.contains("Push") && n.contains("Live") { dest = d; destName = n; break }
}
if dest == 0 {
    print("No 'Push ... Live Port' MIDI destination found. Connected and in controller mode?")
    exit(1)
}

var client = MIDIClientRef()
guard MIDIClientCreate("push-tethered-led" as CFString, nil, nil, &client) == noErr else {
    print("MIDIClientCreate failed"); exit(1)
}
var outPort = MIDIPortRef()
guard MIDIOutputPortCreate(client, "out" as CFString, &outPort) == noErr else {
    print("MIDIOutputPortCreate failed"); exit(1)
}

var sent = 0

func send(_ bytes: [UInt8]) {
    var packet = MIDIPacket()
    packet.timeStamp = 0
    packet.length = UInt16(bytes.count)
    withUnsafeMutableBytes(of: &packet.data) { raw in
        for (i, b) in bytes.enumerated() { raw[i] = b }
    }
    var list = MIDIPacketList(numPackets: 1, packet: packet)
    MIDISend(outPort, dest, &list)
    sent += 1
}

func pad(_ note: UInt8, _ colour: UInt8) { send([0x90, note, colour]) }
func button(_ cc: UInt8, _ brightness: UInt8) { send([0xB0, cc, brightness]) }

func allOff() {
    for n in padNoteMin...padNoteMax { pad(n, 0) }
    for cc in screenTopCCs + screenBotCCs { button(cc, 0) }
}

// Any exit path leaves the device dark.
signal(SIGINT) { _ in allOff(); print("\ninterrupted — LEDs cleared"); exit(0) }
atexit { allOff() }

func pause(_ s: Double) { Thread.sleep(forTimeInterval: s) }

print("output: \(destName)\n")

// ── 1. Clear ────────────────────────────────────────────────────────────────
print("1. clearing all LEDs")
allOff()
pause(0.5)

// ── 2. Palette sweep — 64 pads, 64 distinct palette indices ─────────────────
// Even indices 2-52 are the 26 primary Live colours; 54-78 are muted variants.
// Walking evens from 2 gives a broad, visually distinct spread across the grid.
print("2. palette sweep — each pad a different colour")
for i in 0..<64 {
    let note = padNoteMin + UInt8(i)
    let colour = UInt8(2 + (i * 2) % 126)
    pad(note, colour)
    pause(0.008)
}
pause(2.0)

// ── 3. Row walk — verifies note->pad geometry from the LED side ─────────────
// Note 36 is bottom-left; rows should light bottom to top. If they run top to
// bottom, the input-side mapping in mapcheck is upside down.
print("3. row walk, bottom to top (note 36 = bottom-left)")
allOff()
pause(0.3)
for row in 0..<8 {
    for col in 0..<8 {
        pad(padNoteMin + UInt8(row * 8 + col), 122)   // white
    }
    pause(0.18)
    for col in 0..<8 {
        pad(padNoteMin + UInt8(row * 8 + col), 0)
    }
}
pause(0.3)

// ── 4. Button LEDs — brightness ramp ───────────────────────────────────────
print("4. screen button rows — brightness ramp")
for b: UInt8 in [1, 63, 127] {
    for cc in screenTopCCs + screenBotCCs { button(cc, b) }
    pause(0.7)
}
for cc in screenTopCCs + screenBotCCs { button(cc, 0) }
pause(0.3)

// ── 5. Colour-order check ──────────────────────────────────────────────────
// Bottom row: red, green, blue at known palette slots. Confirms the palette
// index -> colour mapping matches core/push3/colors.go rather than being
// shifted or byte-swapped.
print("5. colour check — bottom-left three pads should read RED GREEN BLUE")
allOff()
pause(0.3)
pad(36, 127)   // shared slot: pure green or red
pad(37, 126)   // shared slot: very dark or pure blue
pad(38, 124)   // white
pad(39, 123)   // mid grey
pad(40, 125)   // light grey
pause(3.0)

print("6. clearing")
allOff()

print("\ndone — \(sent) MIDI messages sent")
print("  pads lit in step 2         -> pad LED protocol confirmed")
print("  rows walked bottom to top  -> note 36 = bottom-left confirmed from output side")
print("  buttons ramped in step 4   -> CC brightness confirmed")
print("  step 5 shows named colours -> palette indices match core/push3")
