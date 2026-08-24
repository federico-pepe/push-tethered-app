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
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/federico-pepe/push-tethered-app/internal/bootstrap"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/modules/beatcount"
	"github.com/federico-pepe/push-tethered-app/modules/monitor"
	"github.com/federico-pepe/push-tethered-app/modules/remap"
	"github.com/federico-pepe/push-tethered-app/modules/seq"
	"github.com/federico-pepe/push-tethered-app/modules/thru"
)

//go:embed all:frontend/dist
var assets embed.FS

// mirrorAddr is where every session's live screen stream is served, one
// path per session key (http://localhost:7071/screen/<key>) — see
// hostManager.mirrorHub and the /screen/ handler below. Hardcoded rather
// than a flag: this is a local dev/monitoring convenience, not a
// user-facing setting, and the frontend needs the same address to build its
// <img> src (see main.ts). Not :7000 or :5000: both are squatted by
// default by macOS's AirPlay Receiver (confirmed live — a request to :7000
// silently landed on ControlCenter's AirTunes server instead of ours).
const mirrorAddr = "localhost:7071"

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
		beatcount.New(),
	}
}

func main() {
	log.SetFlags(0)

	// Every log.Printf in this binary and in internal/bootstrap goes through
	// the standard logger, so redirecting it here covers both — see
	// logfile.go's doc comment for why this exists. A failure to open the
	// log file is not fatal to the app; it just means diagnosing this run
	// falls back to whatever terminal happened to launch it, same as before.
	if path, f, err := openLogFile(); err != nil {
		log.Printf("log file: %v (continuing without one)", err)
	} else {
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		log.Printf("logging to %s", path)
	}

	// A context the host loop runs under, cancelled once the window closes or
	// the app is asked to quit (see the end of main), plus SIGINT/SIGTERM for
	// `wails3 dev` and any headless-ish invocation.
	ctx, cancel := context.WithCancel(context.Background())
	sigCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	defer cancel()

	// availableModules is passed as a factory, not called once here — each
	// session needs its own fresh module instances (see hostManager's
	// newModules doc). bootstrap.Options.Modules is filled in per-connect.
	mgr := newHostManager(sigCtx, bootstrap.Options{FPS: 30}, availableModules)

	// Auto-detect attempt. Most single-Push setups find the Live port by name
	// and are running before the window even paints. When it fails — because
	// more than one Push is attached and there is no right guess (see
	// internal/midi's Open doc), or on Windows where WinMM doesn't expose the
	// port name auto-detect relies on (OpenNamed's doc) — the window still
	// opens and the frontend falls back to the pairing view so the user can
	// pick explicitly.
	if _, err := mgr.connect(ConnectRequest{}); err != nil {
		log.Printf("MIDI: auto-detect failed, waiting for manual pairing: %v", err)
	}

	// One shared server, routed by session key, rather than one listener per
	// session — session count changes over a run's lifetime and a listener
	// per session would mean picking and tracking a port for each. A 404 for
	// an unknown or disconnected key needs no special-casing: mirrorHub's
	// second return value already covers that.
	mux := http.NewServeMux()
	mux.HandleFunc("/screen/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/screen/")
		hub, ok := mgr.mirrorHub(key)
		if !ok {
			http.NotFound(w, r)
			return
		}
		hub.ServeHTTP(w, r)
	})
	go func() {
		if err := http.ListenAndServe(mirrorAddr, mux); err != nil {
			log.Printf("mirror: %v — live screen mirror unavailable", err)
		}
	}()

	app := application.New(application.Options{
		Name:        "Push Tethered App",
		Description: "Module host for Ableton Push 2/3 in tethered mode",
		Services: []application.Service{
			application.NewService(NewPushService(mgr)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// With ApplicationShouldTerminateAfterLastWindowClosed set, macOS's
		// own termination sequence tears the process down without app.Run()
		// ever returning — the mgr.shutdownAll() call after app.Run() below
		// never ran, confirmed live 2026-08-19 (no LEDs went dark on quit,
		// and the log never reached "host: all sessions shut down"). Wails
		// guarantees OnShutdown runs, synchronously, as part of the real
		// shutdown sequence regardless of platform or how termination was
		// triggered, which is why the LED-clearing call belongs here and not
		// only after Run().
		OnShutdown: mgr.shutdownAll,
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Push Tethered App",
		// Wide and tall enough for the two-column pairing view (screens next
		// to MIDI ports) plus several session cards to be visible without
		// scrolling immediately.
		Width:  900,
		Height: 700,
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

	// Idempotent fallback for whatever path reaches here with sessions still
	// open — normal quit is handled by OnShutdown above, which is what
	// actually runs on the platforms tested so far.
	mgr.shutdownAll()

	if appErr != nil {
		log.Fatalf("ui: %v", appErr)
	}
}
