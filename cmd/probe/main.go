// Command probe dumps the full USB configuration descriptor for any connected
// Ableton device (VID 0x2982) — interfaces, alt settings and, crucially,
// endpoint addresses. macOS does not publish endpoint descriptors via ioreg,
// so this is the only way to learn them short of a bus capture.
//
// It deliberately never opens or claims the device: gousb hands the descriptor
// to the matcher callback before opening, and we always return false. Read-only
// by construction — nothing is sent to the hardware.
package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/google/gousb"
)

const vendorAbleton = 0x2982

// known maps PIDs we have identified.
var known = map[gousb.ID]string{
	0x1967: "Push 2",
	0x1969: "Push 3",
}

func main() {
	ctx := gousb.NewContext()
	defer ctx.Close()

	found := 0
	// OpenDevices calls the matcher with each device's descriptor. Returning
	// false means "do not open" — we get the full descriptor for free.
	if _, err := ctx.OpenDevices(func(d *gousb.DeviceDesc) bool {
		if d.Vendor != vendorAbleton {
			return false
		}
		found++
		dumpDevice(d)
		return false
	}); err != nil {
		log.Fatalf("enumerating USB devices: %v", err)
	}

	if found == 0 {
		log.Fatalf("no Ableton device (VID %#04x) found — is Push connected and in controller mode?", vendorAbleton)
	}
}

func dumpDevice(d *gousb.DeviceDesc) {
	name := known[d.Product]
	if name == "" {
		name = "unknown model"
	}
	fmt.Printf("=== Ableton %s — VID %#04x PID %#04x ===\n", name, uint16(d.Vendor), uint16(d.Product))
	fmt.Printf("  bus %d addr %d  USB %s  class %d/%d/%d  maxCtrlPacket %d\n\n",
		d.Bus, d.Address, d.Spec, d.Class, d.SubClass, d.Protocol, d.MaxControlPacketSize)

	for _, cfgNum := range sortedKeys(d.Configs) {
		cfg := d.Configs[cfgNum]
		fmt.Printf("Configuration %d — %d interface(s), maxPower %dmA\n",
			cfg.Number, len(cfg.Interfaces), cfg.MaxPower)

		ifaces := append([]gousb.InterfaceDesc(nil), cfg.Interfaces...)
		sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].Number < ifaces[j].Number })

		for _, iface := range ifaces {
			for _, alt := range iface.AltSettings {
				fmt.Printf("  iface %d alt %d  class %3d/%3d/%3d  %-22s  %d endpoint(s)\n",
					alt.Number, alt.Alternate,
					alt.Class, alt.SubClass, alt.Protocol,
					classLabel(alt.Class, alt.SubClass),
					len(alt.Endpoints))

				for _, epAddr := range sortedEndpoints(alt.Endpoints) {
					ep := alt.Endpoints[epAddr]
					fmt.Printf("        ep %#02x  %-3s  %-11s  maxPacket %5d  interval %d\n",
						uint8(ep.Address), dirLabel(ep.Direction), ep.TransferType,
						ep.MaxPacketSize, ep.PollInterval)
				}
			}
		}
		fmt.Println()
	}
}

func classLabel(class gousb.Class, sub gousb.Class) string {
	switch {
	case class == 0xFF:
		return "VENDOR-SPECIFIC"
	case class == 1 && sub == 1:
		return "audio control"
	case class == 1 && sub == 2:
		return "audio streaming"
	case class == 1 && sub == 3:
		return "MIDI streaming"
	default:
		return class.String()
	}
}

func dirLabel(d gousb.EndpointDirection) string {
	if d == gousb.EndpointDirectionIn {
		return "IN"
	}
	return "OUT"
}

func sortedKeys(m map[int]gousb.ConfigDesc) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func sortedEndpoints(m map[gousb.EndpointAddress]gousb.EndpointDesc) []gousb.EndpointAddress {
	out := make([]gousb.EndpointAddress, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
