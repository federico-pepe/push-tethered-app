module github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui

go 1.26.3

require (
	github.com/federico-pepe/push-tethered-app v0.0.0-00010101000000-000000000000
	github.com/wailsapp/wails/v3 v3.0.0-beta.9
)

// Same reason the root module has this replace for ableton-push-hack/core:
// this is a nested module in the same repo, not a published dependency. A
// replace in push-tethered-app's own go.mod is NOT honoured here — replace
// directives only apply to the main module being built — so both replaces
// below are needed, not just the second one.
replace github.com/federico-pepe/push-tethered-app => ../..

replace github.com/federico-pepe/ableton-push-hack/core => ../../../../Documents/GitHub/ableton-push-hack/core

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/federico-pepe/ableton-push-hack/core v0.0.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/gousb v1.1.3 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	gitlab.com/gomidi/midi/v2 v2.3.24 // indirect
	golang.org/x/image v0.41.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)
