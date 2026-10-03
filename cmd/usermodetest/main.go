// Command usermodetest sends the mode switch SysEx (F0 00 21 1D 01 01 0A <mode>
// F7) to one Push MIDI output cable, to find out whether Push switches into
// User mode when the HOST asks, not only when the user presses User.
//
// This is a measurement tool for plans/2026-10-03-user-mode-switch.md. It can
// send exactly two messages, User mode and Live mode, and nothing else. It
// refuses a port that is not a Push. It does not open the display, does not
// touch USB and sets no LEDs.
//
//	go run ./cmd/usermodetest -list
//	go run ./cmd/usermodetest -out 3 -mode user              # ask for User mode
//	go run ./cmd/usermodetest -out 3 -mode user -hold 10 -revert
//	go run ./cmd/usermodetest -out 3 -mode live              # go back
//	go run ./cmd/usermodetest -out 0 -follow                 # answer User presses
//	go run ./cmd/usermodetest -watch                         # only log
//
// Pad presses are logged with the cable they arrive on. While Push is in Live
// mode they come on the Live Port, in User mode on the User Port. With no Live
// and no PTA nothing lights the Push, so this is how you see the mode.
//
// -follow does what Live's helper does: when the User button (CC 59) arrives on
// the cable that -out names, it toggles the mode and sends the switch back. It
// logs every SysEx and every User press on every Push input cable, so it also
// shows whether Push announces a mode change by itself (plan test D).
//
// What to watch: the User button LED, and whether the pads stop lighting from
// Live. Pressing User on the Push always undoes it.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/midi"
	gm "gitlab.com/gomidi/midi/v2"
)

func main() {
	list := flag.Bool("list", false, "list the Push MIDI output ports and exit")
	out := flag.Int("out", -1, "MIDI output port number to send to (from -list)")
	modeName := flag.String("mode", "user", "mode to ask for: user or live")
	hold := flag.Int("hold", 0, "seconds to wait after sending before exiting")
	revert := flag.Bool("revert", false, "after -hold, ask for Live mode again (also on Ctrl+C)")
	follow := flag.Bool("follow", false, "answer each User button press on the -out cable with a mode switch; log all input")
	watch := flag.Bool("watch", false, "only log SysEx and User presses on every Push input cable")
	flag.Parse()
	log.SetFlags(log.Lmicroseconds)

	if *watch {
		runWatch(nil, "")
		return
	}

	ports := midi.ListOutPortNames()
	if *list {
		fmt.Println("Push MIDI output ports. Cable order on Push 3: Live, User, External.")
		for _, p := range ports {
			fmt.Println(" ", p)
		}
		return
	}
	if *out < 0 {
		log.Fatal("pass -out <number> (see -list)")
	}
	name, ok := pushPort(ports, *out)
	if !ok {
		log.Fatalf("out port %d is not a Push MIDI output. Run -list. Refusing to send.", *out)
	}
	var mode midi.Mode
	switch strings.ToLower(*modeName) {
	case "user":
		mode = midi.UserMode
	case "live":
		mode = midi.LiveMode
	default:
		log.Fatalf("-mode must be user or live, got %q", *modeName)
	}

	cable, err := midi.OpenOutCable(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer cable.Release()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	send := func(m midi.Mode) {
		log.Printf("sending % X to %q: ask for %s mode", midi.ModeSwitchSysEx(m), name, m)
		if err := cable.SendModeSwitch(m); err != nil {
			log.Printf("send failed: %v", err)
		}
	}

	if *follow {
		runWatch(&responder{cable: cable, portName: name, send: send}, name)
		return
	}

	send(mode)
	if *hold > 0 {
		log.Printf("holding %d s. Watch the User LED and the pads. Ctrl+C ends early.", *hold)
		select {
		case <-time.After(time.Duration(*hold) * time.Second):
		case <-ctx.Done():
		}
	}
	if *revert {
		send(midi.LiveMode)
		time.Sleep(200 * time.Millisecond) // let the message leave before the port closes
	}
}

// pushPort finds "#<n> <name>" in the list and returns the name.
func pushPort(ports []string, n int) (string, bool) {
	prefix := "#" + strconv.Itoa(n) + " "
	for _, p := range ports {
		if strings.HasPrefix(p, prefix) {
			return strings.TrimPrefix(p, prefix), true
		}
	}
	return "", false
}

// responder answers User presses by toggling the mode.
type responder struct {
	cable    *midi.OutCable
	portName string // the input cable whose presses we answer
	send     func(midi.Mode)

	mu    sync.Mutex
	cur   midi.Mode // what we believe Push is in. Starts as Live.
	lastT time.Time
}

// press handles one User press. Returns the mode it asked for.
func (r *responder) press() midi.Mode {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastT) < 300*time.Millisecond {
		return r.cur // the same press, seen twice
	}
	r.lastT = time.Now()
	if r.cur == midi.UserMode {
		r.cur = midi.LiveMode
	} else {
		r.cur = midi.UserMode
	}
	r.send(r.cur)
	return r.cur
}

// announced records a mode switch that Push sent by itself. We never answer it.
func (r *responder) announced(m midi.Mode) {
	r.mu.Lock()
	r.cur = m
	r.mu.Unlock()
}

const ccUser = 59

// runWatch logs what arrives on every Push input cable. With r set, a User
// press on r.portName is answered. Runs until Ctrl+C.
func runWatch(r *responder, answerPort string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var stops []func()
	for _, p := range gm.GetInPorts() {
		name := p.String()
		if !strings.Contains(strings.ToLower(name), "push") {
			continue
		}
		in, err := gm.InPort(p.Number())
		if err != nil {
			log.Printf("cannot open input %q: %v", name, err)
			continue
		}
		stop, err := gm.ListenTo(in, func(msg gm.Message, _ int32) {
			handleIn(name, msg, r, answerPort)
		}, gm.UseSysEx())
		if err != nil {
			log.Printf("cannot listen on %q: %v", name, err)
			continue
		}
		stops = append(stops, stop)
		log.Printf("listening on %q", name)
	}
	if len(stops) == 0 {
		log.Fatal("no Push input ports to listen on")
	}
	if r != nil {
		log.Printf("press User on the Push. Each press on %q is answered. Ctrl+C ends.", answerPort)
	} else {
		log.Print("press User on the Push. Ctrl+C ends.")
	}
	<-ctx.Done()
	for _, stop := range stops {
		stop()
	}
	if r != nil {
		r.mu.Lock()
		cur := r.cur
		r.mu.Unlock()
		if cur == midi.UserMode {
			r.send(midi.LiveMode) // leave the Push the way we found it
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func handleIn(port string, msg gm.Message, r *responder, answerPort string) {
	b := []byte(msg)
	if len(b) > 0 && b[0] == 0xF0 {
		if m, ok := midi.ParseModeSwitch(b); ok {
			log.Printf("%-28s ANNOUNCE mode %s  % X", port, m, b)
			if r != nil {
				r.announced(m)
			}
			return
		}
		// Live's recurring 3A/38 SysEx is noise here. Show anything else.
		if len(b) > 6 && (b[6] == 0x3A || b[6] == 0x38) {
			return
		}
		log.Printf("%-28s sysex % X", port, b)
		return
	}
	// Pad presses show which cable Push uses right now: Live Port while it is in
	// Live mode, User Port while it is in User mode. That is the visible proof
	// of the mode when nothing lights the LEDs.
	if len(b) == 3 && b[0]&0xF0 == 0x90 && b[2] > 0 {
		log.Printf("%-28s pad note %d pressed", port, b[1])
		return
	}
	if len(b) == 3 && b[0]&0xF0 == 0xB0 && b[1] == ccUser {
		pressed := b[2] > 0
		log.Printf("%-28s User button %s", port, map[bool]string{true: "pressed", false: "released"}[pressed])
		if pressed && r != nil && port == answerPort {
			r.press()
		}
	}
}
