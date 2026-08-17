// Package remap forwards Push's controls to MIDI, same as thru, but every
// control can be individually overridden by a user-edited rule.
//
// This is the option-B remapper from the original stated goal, reduced to
// what it actually is once the app is a module host: a module, not the
// product. Its default behaviour, with no rules defined, is identical to
// thru — thru is this module's identity case. What remap adds is a
// persisted table of MidiMapping rules, ported from
// hacks/push-manager/src/remap.go, which already modelled src->out CC/Note
// with scaling and relative-encoder accumulation.
//
// There is no on-device editor yet (that is the app UI, phase 3). Until then,
// a rule is added by hand-editing the module's config file — the host logs
// its path on activation — as a JSON object under "overrides", keyed by
// srcKey(). Example: to send pad note 40 out as note 45 with velocity
// rescaled into 20-100:
//
//	{
//	  "overrides": {
//	    "note:40": {"out_type":"note","out_ch":1,"out_num":45,"out_min":20,"out_max":100}
//	  }
//	}
//
// One deliberate deviation from the ported model: push-manager's srcKey
// includes the source channel, because its intercepted control surface can
// target different channels. Here it does not. Screen controls are always on
// channel 1, so channel is never ambiguous for them, and pad note-ons are the
// one thing that IS multi-channel — MPE rotates a pad's channel across
// sessions with no user action (see internal/midi's package doc) — so keying
// on channel would make a saved override silently stop matching. Rules are
// per-note, not per-note-per-channel.
package remap

import (
	"fmt"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// outChannel is the default channel used when a rule (or the passthrough
// default) does not specify one for its input side. Overrides set their own
// OutCh explicitly.
const outChannel = 1

const (
	encoderCCBase = 1  // mirrors thru: encoder i -> CC encoderCCBase+i, not Push's own CC
	encoderStart  = 64 // absolute encoder value starts centred
	logLines      = 5
)

// MidiMapping is one override rule, ported from push-manager's remap.go. JSON
// tags match that file's, so the concept and the file shape carry over even
// though nothing here reads push-manager's config directly.
type MidiMapping struct {
	OutType string `json:"out_type"` // "cc" | "note"
	OutCh   byte   `json:"out_ch"`
	OutNum  byte   `json:"out_num"`
	OutMin  byte   `json:"out_min"`
	OutMax  byte   `json:"out_max"`
	Name    string `json:"name,omitempty"`
}

// doc is what gets persisted.
type doc struct {
	Overrides map[string]MidiMapping `json:"overrides"`
}

// srcKey identifies a physical control. See the package doc for why channel is
// deliberately not part of it.
func srcKey(kind string, num byte) string { return fmt.Sprintf("%s:%d", kind, num) }

// Module forwards controls to MIDI, applying any matching override.
type Module struct {
	host module.Host

	overrides map[string]MidiMapping
	accum     map[string]int // relative-encoder accumulators, keyed like overrides

	encoders [8]int // absolute values for encoders with no override

	held map[byte]bool // sounding notes, for Close — same purpose as thru's

	log      []string
	lastSent string
	sent     int
	lastErr  string

	start  time.Time
	frames int
}

// New returns the remap module.
func New() *Module {
	return &Module{
		overrides: map[string]MidiMapping{},
		accum:     map[string]int{},
		held:      map[byte]bool{},
	}
}

func (m *Module) Meta() module.Meta {
	return module.Meta{
		ID:           "remap",
		Name:         "MIDI Remap",
		Author:       "push-tethered-app",
		Version:      "1.0.0",
		NeedsMIDIOut: true,
	}
}

func (m *Module) Init(h module.Host) error {
	m.host = h
	m.start = time.Now()
	m.frames = 0

	var d doc
	if err := h.Store().Get(&d); err != nil {
		h.Log("loading overrides: %v (starting with none)", err)
		d = doc{}
	}
	m.overrides = d.Overrides
	if m.overrides == nil {
		m.overrides = map[string]MidiMapping{}
	}
	m.accum = map[string]int{}
	for i := range m.encoders {
		m.encoders[i] = encoderStart
	}

	h.Log("%d override(s) loaded; edit the config file to add more", len(m.overrides))
	return nil
}

// Close silences anything still sounding, same reasoning as thru: the host
// clears LEDs but knows nothing about notes in flight.
func (m *Module) Close() error {
	for note := range m.held {
		if err := m.host.NoteOff(outChannel, note); err != nil {
			return fmt.Errorf("remap: releasing note %d: %w", note, err)
		}
		delete(m.held, note)
	}
	return nil
}

func (m *Module) push(line string) {
	m.log = append(m.log, line)
	if len(m.log) > logLines {
		m.log = m.log[len(m.log)-logLines:]
	}
}

func (m *Module) record(desc string, err error) {
	if err != nil {
		m.lastErr = err.Error()
		m.push("ERR  " + desc)
		return
	}
	m.sent++
	m.lastSent = desc
	m.push(desc)
}

func (m *Module) Handle(ev module.Event) {
	switch e := ev.(type) {
	case module.Pad:
		m.handlePad(e)
	case module.Encoder:
		m.handleEncoder(e)
	case module.Button:
		m.handleButton(e)
	case module.Touch, module.Expression:
		// Out of scope, same reasoning as thru: not a control value, and MPE
		// forwarding would flood the port.
	}
}

// handlePad applies a note override if one exists for this pad, else forwards
// the pad's own note unchanged — the thru-identical default.
func (m *Module) handlePad(e module.Pad) {
	key := srcKey("note", e.Note)
	rule, ok := m.overrides[key]

	if !ok {
		// Default: identical to thru.
		if e.Pressed {
			err := m.host.SendNote(outChannel, e.Note, e.Velocity)
			if err == nil {
				m.held[e.Note] = true
			}
			m.record(fmt.Sprintf("note %d on  vel %d", e.Note, e.Velocity), err)
			return
		}
		err := m.host.NoteOff(outChannel, e.Note)
		delete(m.held, e.Note)
		m.record(fmt.Sprintf("note %d off", e.Note), err)
		return
	}

	m.applyRule(key, rule, e.Note, boolToVal(e.Pressed, e.Velocity))
}

// handleButton applies a CC override if one exists, else forwards the
// button's own CC as a 0/127 switch — the thru-identical default.
func (m *Module) handleButton(e module.Button) {
	key := srcKey("cc", e.CC)
	rule, ok := m.overrides[key]

	if !ok {
		val := boolToVal(e.Pressed, 127)
		err := m.host.SendCC(outChannel, e.CC, val)
		m.record(fmt.Sprintf("cc   %d = %d  (%s)", e.CC, val, buttonLabel(e)), err)
		return
	}

	m.applyRule(key, rule, e.CC, boolToVal(e.Pressed, 127))
}

// handleEncoder applies an override if one exists, accumulating the signed
// delta into the rule's own OutMin..OutMax range (mirroring push-manager's
// relative-source handling); else forwards to CC 1-8, the thru-identical
// default.
func (m *Module) handleEncoder(e module.Encoder) {
	if e.Index < 0 || e.Index >= len(m.encoders) {
		return // volume/tempo/jog: no default assignment, same as thru
	}
	key := srcKey("cc", e.CC)
	rule, ok := m.overrides[key]

	if !ok {
		m.encoders[e.Index] = push3.ClampInt(m.encoders[e.Index]+e.Delta, 0, 127)
		cc := byte(encoderCCBase + e.Index)
		val := byte(m.encoders[e.Index])
		err := m.host.SendCC(outChannel, cc, val)
		m.record(fmt.Sprintf("cc   %d = %d  (enc %d)", cc, val, e.Index+1), err)
		return
	}

	acc := push3.ClampInt(m.accum[key]+e.Delta, int(rule.OutMin), int(rule.OutMax))
	m.accum[key] = acc
	m.sendMapped(rule, key, byte(acc), false)
}

// applyRule scales an absolute 0-127 source value into the rule's output
// range, mirroring push-manager's non-relative transform: val 0 means release
// for a note-type source.
func (m *Module) applyRule(key string, rule MidiMapping, srcNum byte, val byte) {
	release := val == 0
	out := push3.ScaleVal(val, rule.OutMin, rule.OutMax)
	m.sendMapped(rule, key, out, release)
}

func (m *Module) sendMapped(rule MidiMapping, key string, val byte, release bool) {
	desc := fmt.Sprintf("%s -> %s %d = %d", key, rule.OutType, rule.OutNum, val)
	var err error
	switch rule.OutType {
	case "note":
		if release {
			err = m.host.NoteOff(rule.OutCh, rule.OutNum)
			delete(m.held, rule.OutNum)
			desc = fmt.Sprintf("%s -> note %d off", key, rule.OutNum)
		} else {
			err = m.host.SendNote(rule.OutCh, rule.OutNum, val)
			if err == nil {
				m.held[rule.OutNum] = true
			}
		}
	default: // "cc"
		err = m.host.SendCC(rule.OutCh, rule.OutNum, val)
	}
	m.record(desc, err)
}

func boolToVal(pressed bool, onVal byte) byte {
	if pressed {
		return onVal
	}
	return 0
}

func buttonLabel(e module.Button) string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("CC %d unmapped", e.CC)
}

func (m *Module) Draw(f *module.Frame) {
	m.frames++
	w, h := f.Size()
	t := m.host.Theme()

	f.Rect(0, 0, w, h, t.Black)
	f.Rect(0, 0, w, 20, t.CrumbBg)
	f.Text(8, 14, fmt.Sprintf("pushapp - remap  (%d override(s))", len(m.overrides)), t.CrumbCol)
	el := time.Since(m.start).Seconds()
	if el < 0.001 {
		el = 0.001
	}
	stats := fmt.Sprintf("%d sent   %.0f fps", m.sent, float64(m.frames)/el)
	f.Text(w-8-text.Width(stats), 14, stats, t.Gray)

	// Overrides in effect, so the on-screen state matches whatever was hand-
	// edited into the config file. KVRows rather than a bespoke list: this is
	// exactly the label:value shape it exists for.
	rows := make([]module.KVRow, 0, len(m.overrides))
	for key, rule := range m.overrides {
		val := fmt.Sprintf("-> %s %d [%d-%d]", rule.OutType, rule.OutNum, rule.OutMin, rule.OutMax)
		rows = append(rows, module.KVRow{Label: key, Value: val, ValueCol: t.Accent})
	}
	f.Text(10, 34, "OVERRIDES", t.Gray)
	if len(rows) == 0 {
		f.Text(10, 50, "(none - passthrough, same as thru)", t.Gray)
	} else {
		f.KVRows(40, 400, 14, 100, h-20, rows)
	}

	// Right: recent activity, same shape as thru.
	lx := 420
	f.Text(lx, 34, "SENT", t.Gray)
	for i, line := range m.log {
		c := t.White
		if i < len(m.log)-1 {
			c = t.Gray
		}
		f.Text(lx, 50+i*15, text.Truncate(line, 60), c)
	}

	if m.lastErr != "" {
		f.Rect(0, h-18, w, 18, t.OffColor)
		f.Text(8, h-5, "send error: "+text.Truncate(m.lastErr, 90), t.White)
		return
	}
	last := m.lastSent
	if last == "" {
		last = "press a pad, turn an encoder, or press a button"
	}
	f.Rect(0, h-18, w, 18, t.StatusBg)
	f.Text(8, h-5, "last: "+last, t.StatusCol)
}
