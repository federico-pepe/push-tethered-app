// Package procmod runs a module as a separate process, any language, speaking
// a small JSON protocol over its own stdin/stdout.
//
// This is the payoff of a decision made in phase 1: internal/module's types
// (Event, Op, Meta) already carry JSON tags specifically so a module could one
// day live outside this binary. Proc is that day — it implements
// module.Module by translating every call into a line of JSON sent to a child
// process, so from Runtime's point of view a process-loaded module is
// indistinguishable from an in-tree Go one.
//
// See plans/2026-08-17-process-loader.md for the design rationale — stdio
// over sockets (no per-OS transport code), one manifest.json per module
// directory (not core/hackcfg's hack.json, which is shaped for on-device
// services this app doesn't have), and the Image display-list op dropped for
// v1 (an *image.NRGBA does not serialise; a module needing raw pixels stays
// in-tree Go for now).
package procmod

import "encoding/json"

// Envelope is one line of the wire protocol, either direction.
//
// A request has Method set, and ID set only if a response is wanted — Handle
// is sent as a notification (Method set, ID omitted) specifically because the
// module contract promises Handle never blocks, so the host must not wait for
// an acknowledgement. A response has no Method; it carries the same ID as the
// request it answers, and either Result or Error, never both.
//
// This is deliberately not a full JSON-RPC implementation (no batching, no
// version field) — the peer set is fixed at two and the method set is small,
// so adopting a general framework would be more machinery than the problem
// needs.
type Envelope struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// IsRequest reports whether this envelope is a call rather than a response.
func (e Envelope) IsRequest() bool { return e.Method != "" }

// WantsResponse reports whether a request expects a reply. Handle is the one
// host→child call that does not.
func (e Envelope) WantsResponse() bool { return e.ID != 0 }

// Method names, host → child.
const (
	methodInit   = "init"
	methodHandle = "handle" // notification: no response
	methodDraw   = "draw"
	methodClose  = "close"
)

// Method names, child → host. Mirror module.Host's own methods 1:1 — no new
// behaviour here, only JSON-shaping, the same principle as cmd/pushapp-ui's
// PushService.
const (
	methodSetPad       = "set_pad"    // notification
	methodSetButton    = "set_button" // notification
	methodSendCC       = "send_cc"
	methodSendNote     = "send_note"
	methodNoteOff      = "note_off"
	methodSendClock    = "send_clock"
	methodSendStart    = "send_start"
	methodSendContinue = "send_continue"
	methodSendStop     = "send_stop"
	methodLog          = "log" // notification
	methodStoreGet     = "store_get"
	methodStoreSet     = "store_set"
)

// ── host → child params/results ────────────────────────────────────────────

type initParams struct {
	Device       string    `json:"device"`
	Theme        wireTheme `json:"theme"`
	SupportedOps []string  `json:"supported_ops"`
}

// wireTheme carries the theme as plain hex-ish RGBA rather than depending on
// the module package's Theme alias resolving the same way in another
// language's JSON decoder — a []int{r,g,b,a} per colour needs no colour
// library on the other end to consume.
type wireTheme map[string][4]uint8

// handleParams wraps a decoded event with an explicit discriminator, since
// module.Event is a Go interface and JSON has no such concept. Kind matches
// module.Event.EventKind(); Data is that concrete event's own JSON encoding.
type handleParams struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// drawResult is what a child returns for a draw request: its display list for
// this frame, plus how many ops it could not build (mirrors module.Frame's own
// Failed() counter) so a child-side bug is visible in the host's log.
type drawResult struct {
	Ops    []opWire `json:"ops"`
	Failed int      `json:"failed"`
}

// opWire mirrors module.Op's wire shape exactly (kind + raw params) — the
// display list was already designed in phase 1 to be an open, JSON-native
// format for exactly this reason.
type opWire struct {
	Kind   string          `json:"kind"`
	Params json.RawMessage `json:"params"`
}

// ── child → host params ────────────────────────────────────────────────────

type setPadParams struct {
	Note   byte `json:"note"`
	Colour byte `json:"colour"`
}

type setButtonParams struct {
	CC         byte `json:"cc"`
	Brightness byte `json:"brightness"`
}

type sendCCParams struct {
	Ch  byte `json:"ch"`
	CC  byte `json:"cc"`
	Val byte `json:"val"`
}

type sendNoteParams struct {
	Ch   byte `json:"ch"`
	Note byte `json:"note"`
	Vel  byte `json:"vel"`
}

type noteOffParams struct {
	Ch   byte `json:"ch"`
	Note byte `json:"note"`
}

type logParams struct {
	Message string `json:"message"`
}

type storeGetResult struct {
	// Doc is the raw persisted document, or absent/null if nothing has been
	// stored yet — mirrors module.Store.Get's own "no file yet is not an
	// error" contract.
	Doc json.RawMessage `json:"doc,omitempty"`
}

type storeSetParams struct {
	Doc json.RawMessage `json:"doc"`
}
