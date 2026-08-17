// Command midiouttest proves the MIDI-out half of the module host: that a note
// sent from Go reaches other software on this machine.
//
// This is phase 0 of plans/2026-08-17-module-host.md — the last unknown in the
// chain before host work starts. Modules need to reach a synth or a DAW, and
// virtual-port creation is the one part of the stack that is not portable
// (see internal/midiout's package doc for the measured per-OS behaviour).
//
// It does not touch Push at all: no USB, no display, no LEDs. Run it alongside
// a MIDI monitor or a soft synth and listen.
//
//	go run ./cmd/midiouttest              # play a scale on the created port
//	go run ./cmd/midiouttest -list        # just list ports and exit
//	go run ./cmd/midiouttest -port loopMIDI -ch 2
//
// -listen makes it the receiver instead of the sender, so the two halves can
// prove each other without a human listening to a synth:
//
//	go run ./cmd/midiouttest -listen "Push Tethered App"   # terminal 1
//	go run ./cmd/midiouttest                              # terminal 2
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/midiout"
	gm "gitlab.com/gomidi/midi/v2"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

func main() {
	list := flag.Bool("list", false, "list MIDI ports and exit")
	listen := flag.String("listen", "", "receive from this input port instead of sending")
	name := flag.String("port", "", "port name to create, or to attach to on Windows")
	ch := flag.Uint("ch", 1, "MIDI channel, 1-16")
	bpm := flag.Float64("bpm", 120, "tempo for the test scale")
	flag.Parse()

	if *list {
		listPorts()
		return
	}
	if *listen != "" {
		receive(*listen)
		return
	}
	if *ch < 1 || *ch > 16 {
		log.Fatalf("-ch %d out of range (want 1-16)", *ch)
	}

	out, err := midiout.Open(*name)
	if err != nil {
		listPorts()
		log.Fatalf("opening MIDI out: %v", err)
	}
	defer out.Close()

	fmt.Printf("port %q opened, mode=%s\n", out.Name(), out.Mode())
	switch out.Mode() {
	case midiout.ModeVirtual:
		fmt.Println("we created this port; other apps should now list it as a MIDI input")
	case midiout.ModeAttached:
		fmt.Println("we attached to an existing port; whatever created it owns its lifetime")
	}

	// A C major scale is enough to show that notes, channel and velocity all
	// arrive intact, and wrong-octave or stuck-note mistakes are audible.
	scale := []byte{60, 62, 64, 65, 67, 69, 71, 72}
	step := time.Duration(float64(time.Minute) / *bpm / 2)
	fmt.Printf("playing 8 notes on channel %d at %.0f BPM\n", *ch, *bpm)

	for _, n := range scale {
		if err := out.SendNote(byte(*ch), n, 100); err != nil {
			log.Fatalf("note on %d: %v", n, err)
		}
		time.Sleep(step)
		// Explicit note off, not velocity 0 — a stuck note here would be the
		// most confusing possible outcome of a test whose whole job is clarity.
		if err := out.NoteOff(byte(*ch), n); err != nil {
			log.Fatalf("note off %d: %v", n, err)
		}
	}

	// One CC sweep: modules remap encoders to CCs, so prove that path too.
	fmt.Printf("sweeping CC 1 on channel %d\n", *ch)
	for v := byte(0); v < 128; v += 8 {
		if err := out.SendCC(byte(*ch), 1, v); err != nil {
			log.Fatalf("cc: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = out.SendCC(byte(*ch), 1, 0)

	fmt.Println("done — if a receiving app saw those, phase 0 is proven")
}

// receive opens an input port by name and prints what arrives, until SIGINT.
// This is the other half of the proof: run it against the port the sender
// creates and the whole path is verified with no synth and no ears involved.
func receive(name string) {
	in, err := gm.FindInPort(name)
	if err != nil {
		listPorts()
		log.Fatalf("opening MIDI in %q: %v", name, err)
	}
	stop, err := gm.ListenTo(in, func(msg gm.Message, _ int32) {
		// Push isn't involved here, but any MIDI source can emit Active
		// Sensing (0xFE) and it would drown the interesting output.
		if len(msg) > 0 && msg[0] >= 0xF8 {
			return
		}
		fmt.Printf("recv % X  %s\n", []byte(msg), msg.String())
	})
	if err != nil {
		log.Fatalf("listening on %q: %v", name, err)
	}
	defer stop()

	fmt.Printf("listening on %q — Ctrl-C to stop\n", name)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	<-ctx.Done()
	fmt.Println("\nstopped")
}

func listPorts() {
	fmt.Println("MIDI input ports:")
	printPorts(gm.GetInPorts())
	fmt.Println("MIDI output ports:")
	printPorts(gm.GetOutPorts())
}

// portLike is the small part of drivers.In / drivers.Out that listing needs.
type portLike interface {
	Number() int
	String() string
}

func printPorts[P portLike](ports []P) {
	if len(ports) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, p := range ports {
		fmt.Printf("  %d: %s\n", p.Number(), p.String())
	}
}
