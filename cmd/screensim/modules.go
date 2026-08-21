package main

// modules.go registers a "mod:<id>" scene per compiled-in module: Init a
// fresh instance against moduletest's fake host, call Draw once, render the
// result. This is what actually verifies a module's own drawing code (as
// opposed to frameScenes/drawScenes, which exercise the widget set with
// hand-built frames) — no hardware, no MIDI port, no display claim.

import (
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/module/moduletest"
	"github.com/federico-pepe/push-tethered-app/internal/renderframe"
	"github.com/federico-pepe/push-tethered-app/modules/beatcount"
	"github.com/federico-pepe/push-tethered-app/modules/monitor"
	"github.com/federico-pepe/push-tethered-app/modules/remap"
	"github.com/federico-pepe/push-tethered-app/modules/seq"
	"github.com/federico-pepe/push-tethered-app/modules/thru"
)

// compiledModules mirrors cmd/pushapp's registration list. New() rather
// than a shared instance: each scene render should start from a fresh,
// unconfigured module, the same as a real activation would.
var compiledModules = map[string]func() module.Module{
	"monitor":   func() module.Module { return monitor.New() },
	"seq":       func() module.Module { return seq.New() },
	"thru":      func() module.Module { return thru.New() },
	"beatcount": func() module.Module { return beatcount.New() },
	"remap":     func() module.Module { return remap.New() },
}

func init() {
	for id, ctor := range compiledModules {
		frameScenes["mod:"+id] = moduleScene(ctor)
	}
}

func moduleScene(ctor func() module.Module) func(*module.Frame) {
	return func(f *module.Frame) {
		m := ctor()
		h := &moduletest.Host{Ops: renderframe.SupportedOps()}
		if err := m.Init(h); err != nil {
			// A scene that can't init is more useful failing loudly at
			// render time than silently drawing a blank frame.
			panic(err)
		}
		defer m.Close()
		m.Draw(f)
	}
}
