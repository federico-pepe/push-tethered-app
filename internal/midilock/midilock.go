// Package midilock serialises access to the shared rtmididrv driver.
//
// rtmididrv.Driver keeps its list of opened ports in an unsynchronised slice —
// the mutex that should guard it is commented out in the vendored driver
// (drivers/rtmididrv/driver.go). A single pushapp-ui process with several
// sessions each opening, closing and polling MIDI ports concurrently would
// otherwise race on that slice. internal/midi and internal/midiout both talk
// to the driver through gitlab.com/gomidi/midi/v2 and must go through this
// package's lock around every call that reaches it — enumeration, open, and
// send/receive setup alike.
//
// This does not slow down MIDI data itself: once a port is open, sending and
// receiving messages does not re-enter the driver's port list, so the lock is
// only needed around the driver calls in internal/midi and internal/midiout,
// not on the hot path of decoding events or writing LEDs.
package midilock

import "sync"

var mu sync.Mutex

// Lock serialises one call into gitlab.com/gomidi/midi/v2 or its drivers
// package. Call Unlock when done, typically via defer immediately after Lock.
func Lock() { mu.Lock() }

// Unlock releases the lock taken by Lock.
func Unlock() { mu.Unlock() }
