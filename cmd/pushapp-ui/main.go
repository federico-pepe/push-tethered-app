// Command pushapp-ui is the desktop UI for the module host: a window that
// lists the compiled-in modules, shows which one is active, and lets the user
// switch. It owns the hardware exactly the same way cmd/pushapp does — see
// internal/bootstrap, which both entry points share — and adds nothing to the
// module contract; it is a client of internal/host's control API, same as any
// other future frontend could be.
//
// Per plans/2026-08-17-module-host.md, there is no hardware switcher: the
// first module is activated immediately on launch, before the window even
// opens, and switching only ever happens from here.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/modules/monitor"
	"github.com/federico-pepe/push-tethered-app/modules/remap"
	"github.com/federico-pepe/push-tethered-app/modules/seq"
	"github.com/federico-pepe/push-tethered-app/modules/thru"
)

//go:embed all:frontend/dist
var assets embed.FS

// availableModules must list the same set cmd/pushapp does. Kept as a
// separate literal, not a shared function, because the two binaries are
// different Go modules — see cmd/pushapp-ui/go.mod's replace directives for
// why moving it to a common package would need a third nested module for no
// real benefit at this size.
func availableModules() []module.Module {
	return []module.Module{
		monitor.New(),
		thru.New(),
		seq.New(),
		remap.New(),
	}
}

func main() {
	log.SetFlags(0)

	rt, cleanup, err := bootstrap.Open(bootstrap.Options{
		FPS:     30,
		Modules: availableModules(),
	})
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer cleanup()

	if err := rt.Activate(rt.List()[0].ID); err != nil {
		log.Fatalf("host: %v", err)
	}

	// A context the host loop runs under, cancelled once the window closes or
	// the app is asked to quit (see the end of main), plus SIGINT/SIGTERM for
	// `wails3 dev` and any headless-ish invocation.
	ctx, cancel := context.WithCancel(context.Background())
	sigCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	runDone := make(chan error, 1)
	go func() { runDone <- rt.Run(sigCtx) }()

	app := application.New(application.Options{
		Name:        "Push Tethered App",
		Description: "Module host for Ableton Push 2/3 in tethered mode",
		Services: []application.Service{
			application.NewService(NewPushService(rt)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Push Tethered App",
		Width:  480,
		Height: 420,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 28,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(16, 16, 20),
		URL:              "/",
	})

	// Blocks until the window closes or the app is asked to quit.
	appErr := app.Run()

	// Stop the host loop and wait for it to actually finish (Run drains events
	// and drives the frame ticker; letting Shutdown race it would let a frame
	// draw against a module the host is mid-switch away from) before releasing
	// the hardware.
	cancel()
	<-runDone
	rt.Shutdown()

	if appErr != nil {
		log.Fatalf("ui: %v", appErr)
	}
}
