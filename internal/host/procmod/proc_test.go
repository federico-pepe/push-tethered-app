package procmod

import (
	"bufio"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
)

// fakeChild drives the OTHER end of the protocol from a test, without
// spawning a real process. It exists so the supervisor's behaviour —
// timeouts, crash detection, malformed input — can be tested deterministically
// and fast, rather than via real subprocess timing.
type fakeChild struct {
	t      *testing.T
	toHost io.WriteCloser // this end is the child's stdout, as the host reads it
	r      *bufio.Scanner // reads what the host wrote to the child's stdin
}

// newTestProc wires a Proc to a fakeChild over two io.Pipes and starts the
// protocol engine (the init handshake), without exec.Cmd anywhere in the
// picture. The fake child must answer init before this returns, same as a
// real one must.
func newTestProc(t *testing.T, h module.Host, answerInit func(*fakeChild)) (*Proc, *fakeChild) {
	t.Helper()

	hostToChildR, hostToChildW := io.Pipe() // p.stdin writes here; fake child reads
	childToHostR, childToHostW := io.Pipe() // fake child writes here; p reads as stdout

	fc := &fakeChild{t: t, toHost: childToHostW, r: bufio.NewScanner(hostToChildR)}
	t.Cleanup(func() {
		hostToChildR.Close()
		hostToChildW.Close()
		childToHostR.Close()
		childToHostW.Close()
	})

	p := &Proc{meta: module.Meta{ID: "test", Name: "Test"}}

	done := make(chan error, 1)
	go func() { done <- p.start(h, hostToChildW, childToHostR, nil) }()

	if answerInit != nil {
		answerInit(fc)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("start() did not return — init handshake never completed")
	}
	return p, fc
}

// recv reads and decodes the next line the host sent, per the protocol's
// framing (one JSON object per line).
func (fc *fakeChild) recv() Envelope {
	fc.t.Helper()
	if !fc.r.Scan() {
		fc.t.Fatalf("recv: pipe closed: %v", fc.r.Err())
	}
	var env Envelope
	if err := json.Unmarshal(fc.r.Bytes(), &env); err != nil {
		fc.t.Fatalf("recv: %v: %q", err, fc.r.Text())
	}
	return env
}

func (fc *fakeChild) send(env Envelope) {
	fc.t.Helper()
	b, err := json.Marshal(env)
	if err != nil {
		fc.t.Fatal(err)
	}
	b = append(b, '\n')
	if _, err := fc.toHost.Write(b); err != nil {
		fc.t.Fatalf("send: %v", err)
	}
}

func (fc *fakeChild) sendRaw(line string) {
	fc.t.Helper()
	if _, err := fc.toHost.Write([]byte(line + "\n")); err != nil {
		fc.t.Fatalf("sendRaw: %v", err)
	}
}

// ackInit is the standard "reply ok" used by every test whose scenario starts
// after the handshake — it reads the init request and acks it.
func ackInit(fc *fakeChild) {
	env := fc.recv()
	if env.Method != methodInit {
		fc.t.Fatalf("first message = %q, want %q", env.Method, methodInit)
	}
	fc.send(Envelope{ID: env.ID, Result: json.RawMessage(`{}`)})
}

func TestInitHandshakeCarriesHostState(t *testing.T) {
	h := &moduletest.Host{}
	var gotParams initParams
	newTestProc(t, h, func(fc *fakeChild) {
		env := fc.recv()
		if env.Method != methodInit {
			t.Fatalf("method = %q, want %q", env.Method, methodInit)
		}
		if err := json.Unmarshal(env.Params, &gotParams); err != nil {
			t.Fatal(err)
		}
		fc.send(Envelope{ID: env.ID, Result: json.RawMessage(`{}`)})
	})

	if gotParams.Device == "" {
		t.Error("init params carried no device")
	}
	if len(gotParams.SupportedOps) == 0 {
		t.Error("init params carried no supported_ops")
	}
	if len(gotParams.Theme) == 0 {
		t.Error("init params carried no theme")
	}
}

func TestInitHandshakeFailureIsReturned(t *testing.T) {
	h := &moduletest.Host{}
	p := &Proc{meta: module.Meta{ID: "test"}}
	hostToChildR, hostToChildW := io.Pipe()
	childToHostR, childToHostW := io.Pipe()
	t.Cleanup(func() {
		hostToChildR.Close()
		hostToChildW.Close()
		childToHostR.Close()
		childToHostW.Close()
	})

	go func() {
		r := bufio.NewScanner(hostToChildR)
		r.Scan()
		var env Envelope
		json.Unmarshal(r.Bytes(), &env)
		b, _ := json.Marshal(Envelope{ID: env.ID, Error: "boom"})
		childToHostW.Write(append(b, '\n'))
	}()

	err := p.start(h, hostToChildW, childToHostR, nil)
	if err == nil {
		t.Fatal("start() returned nil despite the child refusing init")
	}
}

func TestHandleIsFireAndForget(t *testing.T) {
	h := &moduletest.Host{}
	p, fc := newTestProc(t, h, ackInit)

	done := make(chan struct{})
	go func() {
		p.Handle(module.Pad{Note: 60, Col: 0, Row: 3, Velocity: 100, Pressed: true})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Handle blocked despite the child never responding")
	}

	env := fc.recv()
	if env.Method != methodHandle {
		t.Fatalf("method = %q, want %q", env.Method, methodHandle)
	}
	if env.ID != 0 {
		t.Errorf("handle carried an ID (%d); it must be a notification", env.ID)
	}
	var params handleParams
	if err := json.Unmarshal(env.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.Kind != "pad" {
		t.Errorf("kind = %q, want %q", params.Kind, "pad")
	}
	var pad module.Pad
	if err := json.Unmarshal(params.Data, &pad); err != nil {
		t.Fatal(err)
	}
	if pad.Note != 60 || !pad.Pressed {
		t.Errorf("decoded pad = %+v, want note 60 pressed", pad)
	}
}

func TestDrawRoundTrip(t *testing.T) {
	h := &moduletest.Host{}
	p, fc := newTestProc(t, h, ackInit)

	go func() {
		env := fc.recv()
		if env.Method != methodDraw {
			t.Errorf("method = %q, want %q", env.Method, methodDraw)
		}
		result := drawResult{
			Ops: []opWire{{Kind: "rect", Params: json.RawMessage(`{"x":1,"y":2,"w":3,"h":4,"c":{"R":1,"G":2,"B":3,"A":255}}`)}},
		}
		b, _ := json.Marshal(result)
		fc.send(Envelope{ID: env.ID, Result: b})
	}()

	f := module.NewFrame(960, 160)
	p.Draw(f)

	if len(f.Ops()) != 1 || f.Ops()[0].Kind != "rect" {
		t.Fatalf("frame ops = %+v, want one rect op", f.Ops())
	}
}

func TestDrawFailedCountIsLoggedNotFatal(t *testing.T) {
	h := &moduletest.Host{}
	p, fc := newTestProc(t, h, ackInit)

	go func() {
		env := fc.recv()
		result := drawResult{Failed: 3}
		b, _ := json.Marshal(result)
		fc.send(Envelope{ID: env.ID, Result: b})
	}()

	f := module.NewFrame(960, 160)
	p.Draw(f) // must not panic despite Failed > 0 and no ops
	if len(f.Ops()) != 0 {
		t.Errorf("expected no ops, got %d", len(f.Ops()))
	}
	found := false
	for _, l := range h.Logs {
		if l != "" {
			found = true
		}
	}
	if !found {
		t.Error("a nonzero Failed count produced no host log line")
	}
}

// TestDrawTimeout is the supervisor's central guarantee: a module that never
// answers draw must not stall the host past drawTimeout.
func TestDrawTimeout(t *testing.T) {
	h := &moduletest.Host{}
	p, _ := newTestProc(t, h, ackInit)

	start := time.Now()
	f := module.NewFrame(960, 160)
	p.Draw(f) // the fake child never answers this one
	elapsed := time.Since(start)

	if elapsed > drawTimeout+100*time.Millisecond {
		t.Errorf("Draw took %s, want ~%s (drawTimeout)", elapsed, drawTimeout)
	}
	if len(f.Ops()) != 0 {
		t.Error("a timed-out draw should not have produced ops")
	}
}

// TestCrashDuringDrawUnblocksImmediately checks the OTHER escape from a
// blocked call: the child process actually exiting, not just being slow.
// This must return well before drawTimeout, via the done channel.
func TestCrashDuringDrawUnblocksImmediately(t *testing.T) {
	h := &moduletest.Host{}
	p, fc := newTestProc(t, h, ackInit)

	env := struct{}{}
	_ = env
	// Simulate the child dying: close its write end without ever responding.
	fc.toHost.Close()

	start := time.Now()
	f := module.NewFrame(960, 160)
	p.Draw(f)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Draw took %s after the child pipe closed, want near-instant (well under drawTimeout %s)",
			elapsed, drawTimeout)
	}
}

func TestChildSendCC(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	go fc.send(Envelope{ID: 99, Method: methodSendCC, Params: json.RawMessage(`{"ch":1,"cc":10,"val":64}`)})

	resp := fc.recv()
	if resp.ID != 99 || resp.Error != "" {
		t.Fatalf("response = %+v, want id 99, no error", resp)
	}
	if len(h.MIDI) != 1 {
		t.Fatalf("host recorded %d MIDI writes, want 1", len(h.MIDI))
	}
	if got := h.MIDI[0]; got.Kind != "cc" || got.Ch != 1 || got.Num != 10 || got.Val != 64 {
		t.Errorf("recorded write = %+v", got)
	}
}

func TestChildSendCCFailurePropagatesAsError(t *testing.T) {
	h := &moduletest.Host{NoMIDIOut: true}
	_, fc := newTestProc(t, h, ackInit)

	go fc.send(Envelope{ID: 1, Method: methodSendCC, Params: json.RawMessage(`{"ch":1,"cc":10,"val":64}`)})

	resp := fc.recv()
	if resp.Error == "" {
		t.Fatal("expected an error response when the host has no MIDI-out port")
	}
}

func TestChildSendNoteAndNoteOff(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	go fc.send(Envelope{ID: 1, Method: methodSendNote, Params: json.RawMessage(`{"ch":1,"note":60,"vel":100}`)})
	if resp := fc.recv(); resp.Error != "" {
		t.Fatalf("send_note error: %s", resp.Error)
	}

	go fc.send(Envelope{ID: 2, Method: methodNoteOff, Params: json.RawMessage(`{"ch":1,"note":60}`)})
	if resp := fc.recv(); resp.Error != "" {
		t.Fatalf("note_off error: %s", resp.Error)
	}

	if len(h.MIDI) != 2 || h.MIDI[0].Kind != "note" || h.MIDI[1].Kind != "noteoff" {
		t.Fatalf("recorded writes = %+v", h.MIDI)
	}
}

func TestChildSetPadAndSetButtonAreNotifications(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	fc.send(Envelope{Method: methodSetPad, Params: json.RawMessage(`{"note":36,"colour":21}`)})
	fc.send(Envelope{Method: methodSetButton, Params: json.RawMessage(`{"cc":20,"brightness":127}`)})

	// No response should arrive for either — prove it by sending a real
	// request afterward and checking ITS response is the very next line, i.e.
	// nothing was queued ahead of it.
	go fc.send(Envelope{ID: 7, Method: methodSendCC, Params: json.RawMessage(`{"ch":1,"cc":1,"val":1}`)})
	resp := fc.recv()
	if resp.ID != 7 {
		t.Fatalf("first response seen has id %d, want 7 (a notification produced an unexpected reply)", resp.ID)
	}

	if h.LitPads()[36] != 21 {
		t.Error("set_pad notification did not reach the host")
	}
	found := false
	for _, b := range h.Buttons {
		if b.CC == 20 && b.Brightness == 127 {
			found = true
		}
	}
	if !found {
		t.Error("set_button notification did not reach the host")
	}
}

func TestChildLogNotification(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	fc.send(Envelope{Method: methodLog, Params: json.RawMessage(`{"message":"hello from the child"}`)})

	go fc.send(Envelope{ID: 1, Method: methodSendCC, Params: json.RawMessage(`{"ch":1,"cc":1,"val":1}`)})
	fc.recv() // barrier: guarantees the log notification above was processed first

	found := false
	for _, l := range h.Logs {
		if l == "hello from the child" {
			found = true
		}
	}
	if !found {
		t.Errorf("log message did not reach the host, got %v", h.Logs)
	}
}

func TestChildStoreRoundTrip(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	go fc.send(Envelope{ID: 1, Method: methodStoreSet, Params: json.RawMessage(`{"doc":{"n":42}}`)})
	if resp := fc.recv(); resp.Error != "" {
		t.Fatalf("store_set: %s", resp.Error)
	}

	go fc.send(Envelope{ID: 2, Method: methodStoreGet, Params: json.RawMessage(`{}`)})
	resp := fc.recv()
	if resp.Error != "" {
		t.Fatalf("store_get: %s", resp.Error)
	}
	var result storeGetResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(result.Doc, &doc); err != nil {
		t.Fatalf("stored doc = %s: %v", result.Doc, err)
	}
	if doc.N != 42 {
		t.Errorf("round-tripped doc.n = %d, want 42", doc.N)
	}
}

func TestUnknownMethodFromChildReturnsError(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	go fc.send(Envelope{ID: 1, Method: "not_a_real_method"})
	resp := fc.recv()
	if resp.Error == "" {
		t.Fatal("unknown method produced no error response")
	}
}

// TestMalformedLineIsSkippedNotFatal proves one garbled line does not wedge
// the whole read loop — a module built against a slightly different protocol
// version should degrade, not kill the connection.
func TestMalformedLineIsSkippedNotFatal(t *testing.T) {
	h := &moduletest.Host{}
	_, fc := newTestProc(t, h, ackInit)

	fc.sendRaw(`{"this is not valid json`)

	go fc.send(Envelope{ID: 1, Method: methodSendCC, Params: json.RawMessage(`{"ch":1,"cc":1,"val":1}`)})
	resp := fc.recv()
	if resp.ID != 1 || resp.Error != "" {
		t.Fatalf("request after a malformed line got %+v, want a clean response", resp)
	}
}

func TestCloseHandshake(t *testing.T) {
	h := &moduletest.Host{}
	p, fc := newTestProc(t, h, ackInit)

	closeErr := make(chan error, 1)
	go func() { closeErr <- p.Close() }()

	env := fc.recv()
	if env.Method != methodClose {
		t.Fatalf("method = %q, want %q", env.Method, methodClose)
	}
	fc.send(Envelope{ID: env.ID, Result: json.RawMessage(`{}`)})

	select {
	case err := <-closeErr:
		if err != nil {
			t.Errorf("Close() = %v, want nil (no real process attached)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() did not return after the child acked")
	}
}

func TestCloseOnNeverStartedProcIsNoop(t *testing.T) {
	p := &Proc{meta: module.Meta{ID: "never-started"}}
	if err := p.Close(); err != nil {
		t.Errorf("Close() on a never-started Proc = %v, want nil", err)
	}
}
