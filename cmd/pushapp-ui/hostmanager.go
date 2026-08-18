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

	mu      sync.Mutex
	rt      *host.Runtime
	cleanup func()
	cancel  context.CancelFunc
	runDone chan error
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

	m.rt, m.cleanup, m.cancel, m.runDone = rt, cleanup, cancel, runDone
	return nil
}

// shutdown stops the host loop and releases the hardware, if connected. Safe
// to call when never connected.
func (m *hostManager) shutdown() {
	m.mu.Lock()
	rt, cleanup, cancel, runDone := m.rt, m.cleanup, m.cancel, m.runDone
	m.mu.Unlock()
	if rt == nil {
		return
	}

	// Mirrors main's old ordering: stop the loop and wait for it before
	// releasing the hardware, so no frame draws against a module mid-switch.
	cancel()
	<-runDone
	rt.Shutdown()
	cleanup()
	log.Print("host: shut down")
}
