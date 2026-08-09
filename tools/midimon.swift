// midimon — macOS-only probe that dumps incoming MIDI from Push.
//
// NOT part of the app build. This is a capture tool, kept in the repo so the
// button-map measurements are reproducible. Swift because CoreMIDI is right
// there with no dependencies; the app itself is Go (see CLAUDE.md).
//
// It deliberately uses CoreMIDI rather than libusb: that is the co-existence
// mode input path (docs/feasibility.md §6.1a). Reading interface 5 over libusb
// would take Push's MIDI ports away from the DAW.
//
// Build & run:
//   swiftc -O tools/midimon.swift -o midimon && ./midimon [seconds]

import CoreMIDI
import Foundation

let duration = CommandLine.arguments.count > 1 ? Double(CommandLine.arguments[1]) ?? 20 : 20
let start = Date()

func sprop(_ obj: MIDIObjectRef, _ prop: CFString) -> String {
    var out: Unmanaged<CFString>?
    if MIDIObjectGetStringProperty(obj, prop, &out) == noErr, let o = out {
        return o.takeRetainedValue() as String
    }
    return "?"
}

/// padLabel maps Push's 8x8 grid notes (36-99) to a column/row label.
/// Note 36 is the bottom-left pad.
func padLabel(_ note: UInt8) -> String {
    guard note >= 36 && note <= 99 else { return "" }
    let i = Int(note) - 36
    return "  [pad col \(i % 8 + 1) row \(i / 8 + 1) from bottom-left]"
}

/// decode turns one MIDI message into something readable, annotated for Push.
func decode(_ b: [UInt8]) -> String {
    guard let status = b.first else { return "empty" }
    let ch = Int(status & 0x0F) + 1

    // System realtime (0xF8-0xFF) must be tested before the 0xF0 mask, or
    // Active Sensing (0xFE) gets misreported as SysEx. Push emits FE roughly
    // 37x/second as a keepalive — any consumer has to filter it.
    if status >= 0xF8 {
        switch status {
        case 0xF8: return "Clock"
        case 0xFA: return "Start"
        case 0xFB: return "Continue"
        case 0xFC: return "Stop"
        case 0xFE: return "ActiveSensing"
        case 0xFF: return "Reset"
        default:   return String(format: "Realtime %02X", status)
        }
    }

    switch status & 0xF0 {
    case 0x90 where b.count >= 3 && b[2] > 0:
        return "NoteOn   ch\(ch) note \(b[1]) vel \(b[2])\(padLabel(b[1]))"
    case 0x90 where b.count >= 3:
        return "NoteOff  ch\(ch) note \(b[1]) (vel 0)\(padLabel(b[1]))"
    case 0x80 where b.count >= 3:
        return "NoteOff  ch\(ch) note \(b[1]) vel \(b[2])\(padLabel(b[1]))"
    case 0xA0 where b.count >= 3:
        return "PolyAT   ch\(ch) note \(b[1]) pressure \(b[2])\(padLabel(b[1]))"
    case 0xB0 where b.count >= 3:
        // Push encoders send relative deltas: 1..63 = clockwise, 127..64 = ccw.
        var hint = ""
        if b[2] >= 1 && b[2] <= 10 { hint = "  [maybe encoder +\(b[2])]" }
        if b[2] >= 118 && b[2] <= 127 { hint = "  [maybe encoder -\(128 - Int(b[2]))]" }
        if b[2] == 127 { hint = "  [button press, or encoder -1]" }
        if b[2] == 0 { hint = "  [button release]" }
        return "CC       ch\(ch) cc \(b[1]) val \(b[2])\(hint)"
    case 0xD0 where b.count >= 2:
        return "ChanAT   ch\(ch) pressure \(b[1])"
    case 0xE0 where b.count >= 3:
        let bend = Int(b[1]) | (Int(b[2]) << 7)
        return "PitchBend ch\(ch) value \(bend)  [touch strip?]"
    case 0xF0:
        return "SysEx    \(b.map { String(format: "%02X", $0) }.joined(separator: " "))"
    default:
        return "raw      \(b.map { String(format: "%02X", $0) }.joined(separator: " "))"
    }
}

// Collect Push sources only, so DAW/network traffic does not pollute the dump.
var sources: [(ref: MIDIEndpointRef, name: String)] = []
for i in 0..<MIDIGetNumberOfSources() {
    let s = MIDIGetSource(i)
    let name = sprop(s, kMIDIPropertyDisplayName)
    if name.lowercased().contains("push") {
        sources.append((s, name))
    }
}

if sources.isEmpty {
    print("No Push MIDI sources found. Connected and in controller mode?")
    exit(1)
}

var client = MIDIClientRef()
guard MIDIClientCreate("push-tethered-probe" as CFString, nil, nil, &client) == noErr else {
    print("MIDIClientCreate failed"); exit(1)
}

let names = sources.map { $0.name }
var counts = [String: Int]()

var port = MIDIPortRef()
let status = MIDIInputPortCreateWithBlock(client, "in" as CFString, &port) { pktList, refCon in
    let idx = Int(bitPattern: refCon) - 1
    let portName = (idx >= 0 && idx < names.count) ? names[idx] : "?"

    for packet in pktList.unsafeSequence() {
        let len = Int(packet.pointee.length)
        guard len > 0 else { continue }
        let bytes: [UInt8] = withUnsafeBytes(of: packet.pointee.data) { raw in
            Array(raw.prefix(len))
        }
        let t = Date().timeIntervalSince(start)
        let decoded = decode(bytes)
        let hex = bytes.map { String(format: "%02X", $0) }.joined(separator: " ")
        print(String(format: "%6.2fs  %-28s  %-52s  %@",
                     t, (portName as NSString).utf8String!, (decoded as NSString).utf8String!, hex))
        counts[String(decoded.prefix(8)).trimmingCharacters(in: .whitespaces), default: 0] += 1
    }
}
guard status == noErr else { print("MIDIInputPortCreateWithBlock failed"); exit(1) }

for (i, s) in sources.enumerated() {
    MIDIPortConnectSource(port, s.ref, UnsafeMutableRawPointer(bitPattern: i + 1))
    print("listening on: \(s.name)")
}

print("\nPress pads, buttons, turn encoders, touch the strip. \(Int(duration))s...\n")

Timer.scheduledTimer(withTimeInterval: duration, repeats: false) { _ in
    print("\n=== summary ===")
    if counts.isEmpty {
        print("NOTHING RECEIVED — Push may not send MIDI in controller mode without a host handshake.")
    } else {
        for (k, v) in counts.sorted(by: { $0.value > $1.value }) {
            print("  \(k): \(v)")
        }
    }
    exit(0)
}
RunLoop.main.run()
