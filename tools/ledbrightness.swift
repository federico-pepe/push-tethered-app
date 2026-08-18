// ledbrightness — macOS-only probe for button-LED brightness fidelity.
//
// NOT part of the app build. Companion to tools/ledtest.swift, but focused:
// ledtest's button step is a 3-value ramp (1, 63, 127) meant to confirm the
// CC-brightness protocol works at all. This probe exists to answer the
// specific open question in docs/protocol/led-output.md — whether the 0-127
// brightness scale is perceptually linear, and whether there's a dead zone
// at the low end — by holding on each of several values long enough to
// eyeball, on ONE button at a time so differences aren't confused with
// per-button hardware/silkscreen differences.
//
// Uses CoreMIDI, same reasoning as ledtest.swift: that is the co-existence
// output path, and writing to interface 5 over libusb would take Push's MIDI
// ports away from the DAW.
//
// Every exit path leaves the device dark, including SIGINT.
//
// Build & run:
//   swiftc -O tools/ledbrightness.swift -o ledbrightness
//   ./ledbrightness            # sweeps the first screen-top button (CC 102)
//   ./ledbrightness 108        # sweeps a specific CC

import CoreMIDI
import Foundation

let screenTopCCs: [UInt8] = Array(102...109)   // row above the display
let screenBotCCs: [UInt8] = Array(20...27)     // row below the display
let allButtonCCs = screenTopCCs + screenBotCCs

func sprop(_ obj: MIDIObjectRef, _ prop: CFString) -> String {
    var out: Unmanaged<CFString>?
    if MIDIObjectGetStringProperty(obj, prop, &out) == noErr, let o = out {
        return o.takeRetainedValue() as String
    }
    return "?"
}

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
guard MIDIClientCreate("push-tethered-led-brightness" as CFString, nil, nil, &client) == noErr else {
    print("MIDIClientCreate failed"); exit(1)
}
var outPort = MIDIPortRef()
guard MIDIOutputPortCreate(client, "out" as CFString, &outPort) == noErr else {
    print("MIDIOutputPortCreate failed"); exit(1)
}

func send(_ bytes: [UInt8]) {
    var packet = MIDIPacket()
    packet.timeStamp = 0
    packet.length = UInt16(bytes.count)
    withUnsafeMutableBytes(of: &packet.data) { raw in
        for (i, b) in bytes.enumerated() { raw[i] = b }
    }
    var list = MIDIPacketList(numPackets: 1, packet: packet)
    MIDISend(outPort, dest, &list)
}

func button(_ cc: UInt8, _ brightness: UInt8) { send([0xB0, cc, brightness]) }

func allOff() {
    for cc in allButtonCCs { button(cc, 0) }
}

signal(SIGINT) { _ in allOff(); print("\ninterrupted — LEDs cleared"); exit(0) }
atexit { allOff() }

func pause(_ s: Double) { Thread.sleep(forTimeInterval: s) }

// Default to the first screen-top button unless a CC is given on argv.
var targetCC: UInt8 = 102
if CommandLine.arguments.count > 1, let v = UInt8(CommandLine.arguments[1]) {
    targetCC = v
}

print("output: \(destName)")
print("sweeping CC \(targetCC) only — every other button LED stays off\n")

allOff()
pause(0.5)

let steps: [UInt8] = [0, 4, 8, 16, 32, 48, 64, 80, 96, 112, 127]
for v in steps {
    print(String(format: "brightness %3d — watching for ~2.5s", v))
    button(targetCC, v)
    pause(2.5)
}

allOff()
print("\ndone — LED cleared")
