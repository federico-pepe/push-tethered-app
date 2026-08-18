package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/host"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
)

// hostManager owns the *host.Runtime across its whole disconnected/connected
// lifecycle. It exists because bootstrap.Open can fail at the MIDI step —
// most commonly on Windows, where the Live port can't be found by name (see
// internal/midi's OpenNamed doc) — and the window must still open so the user
// has somewhere to pick a port from. Everything that used to run inline in
// main() before the window existed now runs in connect, callable once at
// startup (auto-detect) and again from the frontend (manual pick) with the
// same code path either time.
type hostManager struct {
	rootCtx  context.Context // cancelled on app quit/SIGINT/SIGTERM; parent of every run's context
	baseOpts bootstrap.Options

	mu         sync.Mutex
	rt         *host.Runtime
	cleanup    func()
	cancel     context.CancelFunc
	stopped    chan struct{} // closed by watch once Run has returned and teardown is done
	deliberate bool          // set by shutdown before cancelling, so watch knows this exit was asked for

	lastErr error // most recent unexpected disconnect, cleared on the next successful connect
}

func newHostManager(rootCtx context.Context, opts bootstrap.Options) *hostManager {
	return &hostManager{rootCtx: rootCtx, baseOpts: opts}
}

// connected reports whether a Runtime is currently active, and returns it.
func (m *hostManager) connected() (*host.Runtime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rt, m.rt != nil
}

// ports lists every MIDI input port name the OS sees, for a manual picker.
func (m *hostManager) ports() []string {
	return pmidi.ListInPorts()
}

// connect claims the hardware and starts the host loop. name is the exact
// MIDI input port to use, or "" to auto-detect the Live port. Safe to call
// again after a prior failure; refuses if already connected.
func (m *hostManager) connect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rt != nil {
		return fmt.Errorf("already connected")
	}

	opts := m.baseOpts
	opts.MIDIInName = name
	rt, cleanup, err := bootstrap.Open(opts)
	if err != nil {
		return err
	}

	if err := rt.Activate(rt.List()[0].ID); err != nil {
		cleanup()
		return fmt.Errorf("host: %w", err)
	}

	ctx, cancel := context.WithCancel(m.rootCtx)
	runDone := make(chan error, 1)
	go func() { runDone <- rt.Run(ctx) }()

	stopped := make(chan struct{})
	m.rt, m.cleanup, m.cancel, m.stopped, m.deliberate, m.lastErr = rt, cleanup, cancel, stopped, false, nil

	// watch is the sole reader of runDone. It fires whether Run stopped on its
	// own (e.g. the device was unplugged) or because shutdown() cancelled its
	// context; either way it tears the runtime down and clears m.rt so
	// connected() reflects reality instead of the UI polling a dead runtime.
	go m.watch(rt, cleanup, runDone, stopped)
	return nil
}

// watch waits for one Run call to finish, tears the runtime down, records why
// for lastError if the exit was not shutdown's doing, then closes stopped so
// a concurrent shutdown() call can return.
func (m *hostManager) watch(rt *host.Runtime, cleanup func(), runDone chan error, stopped chan struct{}) {
	err := <-runDone
	defer close(stopped)

	m.mu.Lock()
	if m.rt != rt {
		m.mu.Unlock()
		return // a newer session has already replaced this one
	}
	m.mu.Unlock()

	rt.Shutdown()
	cleanup()

	m.mu.Lock()
	defer m.mu.Unlock()
	deliberate := m.deliberate
	m.rt, m.cleanup, m.cancel, m.stopped = nil, nil, nil, nil
	if err != nil && !deliberate {
		m.lastErr = err
		log.Printf("host: disconnected: %v", err)
	}
}

// lastError returns the most recent unexpected disconnect reason, if any.
func (m *hostManager) lastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

// shutdown stops the host loop and releases the hardware, if connected. Safe
// to call when never connected. Teardown itself happens in watch, which is
// the sole reader of runDone; shutdown only signals and waits.
func (m *hostManager) shutdown() {
	m.mu.Lock()
	cancel, stopped := m.cancel, m.stopped
	m.deliberate = true
	m.mu.Unlock()
	if cancel == nil {
		return
	}

	cancel()
	<-stopped
	log.Print("host: shut down")
}
