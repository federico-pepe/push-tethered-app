// Package version holds the build-time version string for pushapp binaries.
//
// Version is overridden at build time via:
//
//	go build -ldflags "-X github.com/federico-pepe/push-tethered-app/internal/version.Version=v0.1.0-alpha"
//
// Local `go build`/`go run` without that flag reports "dev".
package version

var Version = "dev"
