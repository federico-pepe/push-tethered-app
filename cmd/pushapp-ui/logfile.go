package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// userConfigDir is a seam over os.UserConfigDir so tests can point it at a
// temp directory instead of the real one — same pattern as
// internal/host/store.go's own seam of the same name.
var userConfigDir = os.UserConfigDir

// openLogFile opens (truncating) this run's log file and returns its path
// alongside the handle. Truncated rather than appended or rotated: this is a
// "what happened on the last run" log for diagnosing a specific bug report,
// not an audit trail, and an ever-growing file serves that worse, not better.
//
// pushapp-ui has no terminal of its own once launched by double-clicking the
// app on any of the three desktop OSes — log.Printf's default stderr output
// goes nowhere the user can find. Every diagnostic exchange in
// plans/2026-08-19-live-coexistence.md and the multi-device work needed a
// screen-share or a manually redirected terminal run instead; this exists so
// "what does the log say" has an answer without either.
func openLogFile() (path string, f *os.File, err error) {
	base, err := userConfigDir()
	if err != nil {
		return "", nil, fmt.Errorf("finding a config directory: %w", err)
	}
	dir := filepath.Join(base, "push-tethered-app", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	path = filepath.Join(dir, "pushapp-ui.log")
	f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return path, f, nil
}
