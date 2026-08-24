// Package applog gives every entry point (cmd/pushapp, cmd/pushapp-ui) the
// same log line shape — a timestamp and a level on every line — computed
// once here rather than duplicated per binary.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// timeLayout matches "2026-04-07T16:35:55.764227" — six fractional digits,
// no timezone offset: every reader of this log is on the machine that wrote
// it, so a zone conversion would only cost clarity.
const timeLayout = "2006-01-02T15:04:05.000000"

// target is the raw (unwrapped) writer Wrap was last given — Errorf/Fatalf
// write straight to it with their own "error" tag, since the standard
// logger (and this package's Wrap) only ever tags a line "info". Guarded
// by mu since it's set once at startup but read from whatever goroutine
// calls Errorf.
var (
	mu     sync.Mutex
	target io.Writer = os.Stderr
)

// wrapWriter prepends "<timestamp>: info: " to every write it receives.
// Safe as log.SetOutput's target: the standard logger issues exactly one
// Write per formatted log line (the message plus its trailing newline),
// and callers of Wrap are expected to have set log.SetFlags(0) so nothing
// of the standard logger's own is already in p — Wrap owns the whole line.
type wrapWriter struct {
	out io.Writer
}

func (w *wrapWriter) Write(p []byte) (int, error) {
	line := fmt.Sprintf("%s: info: %s", time.Now().Format(timeLayout), p)
	if _, err := w.out.Write([]byte(line)); err != nil {
		return 0, err
	}
	// Report the caller's own byte count as written, not the (longer)
	// timestamped line's — log.Logger.Output treats a short count as an
	// error, and every byte of p did make it into out.
	return len(p), nil
}

// Wrap returns an io.Writer for log.SetOutput that timestamps every line
// written to out — e.g. os.Stderr, or io.MultiWriter(os.Stderr, logFile).
// Pair with log.SetFlags(0): this package only owns the output writer, not
// the logger attached to it.
func Wrap(out io.Writer) io.Writer {
	mu.Lock()
	target = out
	mu.Unlock()
	return &wrapWriter{out: out}
}

// Banner logs a boxed three-line startup marker — call once, right after
// log.SetOutput(Wrap(...)). Exists so a log file that accumulates across
// runs (or a long stderr scrollback) has an unmissable "a new run started
// here" line, and so a paste into a bug report starts somewhere obvious.
func Banner() {
	const rule = "#######################################"
	log.Print(rule)
	log.Print("Push Tethered App")
	log.Print(rule)
}

// Errorf logs a formatted message tagged "error" instead of "info" —
// log.Printf (and everything using it, which is most of this codebase)
// always comes out "info" via Wrap, so an actual failure needs this
// instead to read as one in the log. Writes straight to whatever writer
// the last Wrap call was given, bypassing the standard logger entirely.
func Errorf(format string, args ...any) {
	writeLevel("error", fmt.Sprintf(format, args...))
}

// Fatalf is Errorf followed by os.Exit(1) — log.Fatalf's shape, tagged
// "error" instead of "info".
func Fatalf(format string, args ...any) {
	Errorf(format, args...)
	os.Exit(1)
}

func writeLevel(level, msg string) {
	line := fmt.Sprintf("%s: %s: %s\n", time.Now().Format(timeLayout), level, msg)
	mu.Lock()
	out := target
	mu.Unlock()
	_, _ = out.Write([]byte(line))
}
