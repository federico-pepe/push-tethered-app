package procmod

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/module"
)

const (
	// initTimeout is generous: a child interpreter starting up (importing
	// libraries, etc.) is a one-time cost, not a per-frame one.
	initTimeout = 3 * time.Second

	// drawTimeout must be short: this blocks the host's single render
	// goroutine, and at 30fps the whole frame budget is ~33ms. A child this
	// slow could not have kept up anyway; better to draw nothing for one
	// frame and log it than to visibly stall the display.
	drawTimeout = 200 * time.Millisecond

	// closeTimeout bounds the close handshake itself (the child's chance to
	// release notes etc.); closeGrace is the additional time given for the
	// process to actually exit afterward before it is killed outright.
	closeTimeout = 2 * time.Second
	closeGrace   = 2 * time.Second

	// notifyTimeout bounds notify()'s write, same reasoning as drawTimeout: a
	// child that isn't draining its stdin (wedged, or a real OS pipe buffer
	// that's genuinely full) must not turn Handle's "never blocks" guarantee
	// into a lie.
	notifyTimeout = 200 * time.Millisecond
)

// Proc runs a module as a child process. It implements module.Module, so from
// Runtime's point of view it is indistinguishable from an in-tree Go module —
// see the package doc for why that's the whole point.
type Proc struct {
	meta       module.Meta
	dir        string
	execFields []string

	host module.Host

	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex

	pendingMu sync.Mutex
	nextID    int
	pending   map[int]chan Envelope

	// done is closed when the read loop ends — the child exited or its
	// stdout pipe broke. Any in-flight call() unblocks on it rather than
	// waiting out its full timeout for a process that is already gone.
	done chan struct{}
}

var _ module.Module = (*Proc)(nil)

// New loads a module's manifest from dir and prepares it to run, without
// spawning anything yet — that happens in Init, same as any other module.
func New(dir string) (*Proc, error) {
	man, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	exec, err := man.ResolvedExec()
	if err != nil {
		return nil, fmt.Errorf("module %q: %w", man.ID, err)
	}
	fields, err := resolveExec(dir, exec)
	if err != nil {
		return nil, fmt.Errorf("module %q: %w", man.ID, err)
	}
	return &Proc{meta: man.Meta(), dir: dir, execFields: fields}, nil
}

func (p *Proc) Meta() module.Meta { return p.meta }

// Init spawns the child process and performs the init handshake.
func (p *Proc) Init(h module.Host) error {
	cmd := exec.Command(p.execFields[0], p.execFields[1:]...)
	cmd.Dir = p.dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("module %q: stdin pipe: %w", p.meta.ID, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("module %q: stdout pipe: %w", p.meta.ID, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("module %q: stderr pipe: %w", p.meta.ID, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("module %q: starting %v: %w", p.meta.ID, p.execFields, err)
	}
	p.cmd = cmd

	if err := p.start(h, stdin, stdout, stderr); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

// start wires the protocol engine over the given pipes and performs the init
// handshake. Split out from Init so tests can drive the engine over an
// io.Pipe pair without spawning a real OS process — see proc_test.go.
func (p *Proc) start(h module.Host, stdin io.WriteCloser, stdout, stderr io.Reader) error {
	p.host = h
	p.stdin = stdin
	p.pending = map[int]chan Envelope{}
	p.done = make(chan struct{})

	go p.readLoop(stdout)
	if stderr != nil {
		go p.stderrLoop(stderr)
	}

	params := initParams{
		Device:       h.Device().String(),
		Theme:        themeToWire(h.Theme()),
		SupportedOps: h.SupportedOps(),
	}
	if _, err := p.call(methodInit, params, initTimeout); err != nil {
		return fmt.Errorf("module %q: init handshake: %w", p.meta.ID, err)
	}
	return nil
}

// Handle marshals the event and sends it as a notification — Handle must
// never block, so the host does not wait for the child to finish processing
// it. A write failure (a dead pipe) is logged, not returned: nothing in the
// module.Module contract lets Handle report an error.
func (p *Proc) Handle(ev module.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		p.logf("handle: marshalling event: %v", err)
		return
	}
	if err := p.notify(methodHandle, handleParams{Kind: ev.EventKind(), Data: data}); err != nil {
		p.logf("handle: %v", err)
	}
}

// Draw requests this frame's display list and appends it via AppendRaw — the
// second real caller of that method, exactly as phase 1's package doc
// predicted it would be needed for a process loader.
func (p *Proc) Draw(f *module.Frame) {
	raw, err := p.call(methodDraw, nil, drawTimeout)
	if err != nil {
		p.logf("draw: %v", err)
		return
	}
	var result drawResult
	if err := json.Unmarshal(raw, &result); err != nil {
		p.logf("draw: parsing result: %v", err)
		return
	}
	for _, op := range result.Ops {
		f.AppendRaw(op.Kind, op.Params)
	}
	if result.Failed > 0 {
		p.logf("draw: module reported %d failed op(s)", result.Failed)
	}
}

// Close asks the child to release anything it's holding, then waits for it to
// exit, killing it if it overstays closeGrace. A module that hangs here must
// not hang the host's own shutdown.
func (p *Proc) Close() error {
	if p.stdin == nil {
		return nil // Init never got far enough to start anything
	}

	// Ignore the error: if the process already crashed there is nothing left
	// to close cleanly, and that is not this method's problem to report —
	// the crash itself was already logged by the read loop.
	_, _ = p.call(methodClose, nil, closeTimeout)
	_ = p.stdin.Close()

	if p.cmd == nil {
		return nil // test harness driving the protocol with no real process
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- p.cmd.Wait() }()

	select {
	case err := <-waitDone:
		return err
	case <-time.After(closeGrace):
		_ = p.cmd.Process.Kill()
		<-waitDone
		return fmt.Errorf("module %q did not exit after close; killed", p.meta.ID)
	}
}

// ── protocol engine ─────────────────────────────────────────────────────────

// call sends a request and blocks for its response, up to timeout total —
// covering the write itself as well as the wait, not just the wait. A wedged
// child that isn't reading its stdin can otherwise block the write forever
// (an io.Pipe blocks synchronously with nothing to drain it; even a real OS
// pipe eventually fills its kernel buffer), which would defeat the timeout
// entirely if only the response wait were bounded.
func (p *Proc) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}

	p.pendingMu.Lock()
	p.nextID++
	id := p.nextID
	ch := make(chan Envelope, 1)
	p.pending[id] = ch
	p.pendingMu.Unlock()
	defer func() {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	writeErr := make(chan error, 1)
	go func() { writeErr <- p.writeLine(Envelope{ID: id, Method: method, Params: raw}) }()

	select {
	case err := <-writeErr:
		if err != nil {
			return nil, fmt.Errorf("writing %s: %w", method, err)
		}
	case <-timer.C:
		return nil, fmt.Errorf("%s: timed out after %s (writing request)", method, timeout)
	case <-p.done:
		return nil, fmt.Errorf("%s: module process exited", method)
	}

	select {
	case env := <-ch:
		if env.Error != "" {
			return nil, errors.New(env.Error)
		}
		return env.Result, nil
	case <-timer.C:
		return nil, fmt.Errorf("%s: timed out after %s", method, timeout)
	case <-p.done:
		return nil, fmt.Errorf("%s: module process exited", method)
	}
}

// notify sends a request with no ID — fire and forget, no response expected
// or waited for. Still bounded: see call's doc for why an unbounded write is
// not actually safe just because no response is being awaited.
func (p *Proc) notify(method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}

	writeErr := make(chan error, 1)
	go func() { writeErr <- p.writeLine(Envelope{Method: method, Params: raw}) }()

	select {
	case err := <-writeErr:
		return err
	case <-time.After(notifyTimeout):
		return fmt.Errorf("%s: timed out writing notification after %s", method, notifyTimeout)
	case <-p.done:
		return fmt.Errorf("%s: module process exited", method)
	}
}

func (p *Proc) writeLine(env Envelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.stdin == nil {
		return errors.New("module process not started")
	}
	_, err = p.stdin.Write(b)
	return err
}

// readLoop demultiplexes the child's stdout: a line with Method set is a
// request from the child (handled and, if it wants one, answered); a line
// with no Method is a response to something the host asked, matched to its
// pending call() by ID.
func (p *Proc) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	// Default token limit is 64KB, easily exceeded by a frame with many list
	// rows; 4MB is generous headroom for a protocol that is otherwise tiny.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			p.logf("malformed line from module: %v", err)
			continue
		}
		if env.IsRequest() {
			p.handleChildRequest(env)
			continue
		}
		p.pendingMu.Lock()
		ch, ok := p.pending[env.ID]
		p.pendingMu.Unlock()
		if ok {
			select {
			case ch <- env:
			default:
			}
		}
	}
	close(p.done)
}

func (p *Proc) stderrLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		p.logf("(stderr) %s", scanner.Text())
	}
}

// handleChildRequest dispatches a call the child made to us — the JSON-RPC
// mirror of module.Host's own methods. See protocol.go's table.
func (p *Proc) handleChildRequest(env Envelope) {
	switch env.Method {
	case methodSetPad:
		var params setPadParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.logf("set_pad: %v", err)
			return
		}
		p.host.SetPad(params.Note, params.Colour)

	case methodSetButton:
		var params setButtonParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.logf("set_button: %v", err)
			return
		}
		p.host.SetButton(params.CC, params.Brightness)

	case methodLog:
		var params logParams
		if err := json.Unmarshal(env.Params, &params); err == nil {
			p.host.Log("%s", params.Message)
		}

	case methodSendCC:
		var params sendCCParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.respondErr(env.ID, err)
			return
		}
		p.respond(env.ID, struct{}{}, p.host.SendCC(params.Ch, params.CC, params.Val))

	case methodSendNote:
		var params sendNoteParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.respondErr(env.ID, err)
			return
		}
		p.respond(env.ID, struct{}{}, p.host.SendNote(params.Ch, params.Note, params.Vel))

	case methodNoteOff:
		var params noteOffParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.respondErr(env.ID, err)
			return
		}
		p.respond(env.ID, struct{}{}, p.host.NoteOff(params.Ch, params.Note))

	case methodSendClock:
		p.respond(env.ID, struct{}{}, p.host.SendClock())

	case methodSendStart:
		p.respond(env.ID, struct{}{}, p.host.SendStart())

	case methodSendContinue:
		p.respond(env.ID, struct{}{}, p.host.SendContinue())

	case methodSendStop:
		p.respond(env.ID, struct{}{}, p.host.SendStop())

	case methodStoreGet:
		var doc json.RawMessage
		if err := p.host.Store().Get(&doc); err != nil {
			p.respondErr(env.ID, err)
			return
		}
		p.respond(env.ID, storeGetResult{Doc: doc}, nil)

	case methodStoreSet:
		var params storeSetParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			p.respondErr(env.ID, err)
			return
		}
		// Store.Set marshals whatever it's given; passing the already-raw
		// bytes through avoids a pointless decode/re-encode round trip.
		p.respond(env.ID, struct{}{}, p.host.Store().Set(params.Doc))

	default:
		if env.WantsResponse() {
			p.respondErr(env.ID, fmt.Errorf("unknown method %q", env.Method))
		} else {
			p.logf("unknown notification %q", env.Method)
		}
	}
}

func (p *Proc) respond(id int, result any, err error) {
	if id == 0 {
		return // the child did not ask for a response
	}
	if err != nil {
		p.respondErr(id, err)
		return
	}
	b, merr := json.Marshal(result)
	if merr != nil {
		p.respondErr(id, merr)
		return
	}
	_ = p.writeLine(Envelope{ID: id, Result: b})
}

func (p *Proc) respondErr(id int, err error) {
	if id == 0 {
		return
	}
	_ = p.writeLine(Envelope{ID: id, Error: err.Error()})
}

func (p *Proc) logf(format string, args ...any) {
	if p.host != nil {
		p.host.Log(format, args...)
		return
	}
	log.Printf(format, args...)
}

// themeToWire converts the theme to plain [4]uint8 RGBA per colour, so a
// child in any language can consume it with no colour library of its own —
// no dependence on module.Theme's Go representation surviving the JSON
// round trip in any particular shape.
func themeToWire(t module.Theme) wireTheme {
	c := func(v color.NRGBA) [4]uint8 { return [4]uint8{v.R, v.G, v.B, v.A} }
	return wireTheme{
		"black": c(t.Black), "white": c(t.White), "gray": c(t.Gray), "dark_gray": c(t.DarkGray),
		"select": c(t.Select), "dir_color": c(t.DirColor), "accent": c(t.Accent),
		"on_color": c(t.OnColor), "off_color": c(t.OffColor),
		"crumb_bg": c(t.CrumbBg), "crumb_col": c(t.CrumbCol),
		"status_bg": c(t.StatusBg), "status_col": c(t.StatusCol),
	}
}
