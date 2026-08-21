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
// Rules can be created and edited entirely on-device — see the "Editing
// rules" section below — or by hand-editing the module's config file (the
// host logs its path on activation) as a JSON object under "overrides",
// keyed by srcKey(). Example: to send pad note 40 out as note 45 with
// velocity rescaled into 20-100:
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
//
// # Editing rules
//
// Screen Bot 1 toggles edit mode. Once armed, the next pad press, button
// press, or encoder turn selects that control as the target and opens its
// rule for editing — new if none exists yet, otherwise the existing one.
// While editing, Screen Top 1-5 (paired with the top-row encoders) adjust
// Out Type, Out Channel, Out Value, Out Min, and Out Max; Screen Bot 8 saves,
// Screen Bot 7 clears the rule back to passthrough, and Screen Bot 1 cancels
// back to armed. Saving or clearing returns to armed so the next control can
// be picked without re-toggling.
package remap

import (
	"fmt"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
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

// Bottom-strip buttons dedicated to the on-device editor. Kept apart from
// the physical passthrough/override path: these three CCs never reach
// handleButton, regardless of ui state.
const (
	editToggleCC = push3.CCScreenBot1
	editClearCC  = push3.CCScreenBot7
	editSaveCC   = push3.CCScreenBot8
)

// uiState is the on-device editor's screen mode.
type uiState int

const (
	stateOff     uiState = iota // normal passthrough + overrides list
	stateArmed                  // edit mode on, waiting for a target
	stateEditing                // target selected, draft rule shown/editable
)

// target identifies the physical control currently being edited. kind
// matches srcKey's ("note" for pads, "cc" for buttons/encoders).
type target struct {
	kind string
	num  byte
}

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

	ui          uiState
	sel         target
	draft       MidiMapping
	hasOverride bool // whether sel already had a rule when selected

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
		Name:         "MIDI Remap (Test)",
		Author:       "Federico Pepe",
		Version:      "1.0.0",
		Description:  "User-editable overrides on top of thru's default passthrough",
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
	case module.Button:
		if m.handleUIButton(e) {
			return
		}
		m.handleButton(e)
	case module.Pad:
		switch m.ui {
		case stateArmed:
			if e.Pressed {
				m.armTarget("note", e.Note, e.Channel)
			}
		case stateEditing:
			// Swallowed: a pad press mid-edit is not a new selection.
		default:
			m.handlePad(e)
		}
	case module.Encoder:
		switch m.ui {
		case stateEditing:
			m.editField(e)
		case stateArmed:
			if e.Delta != 0 {
				m.armTarget("cc", e.CC, 0)
			}
		default:
			m.handleEncoder(e)
		}
	case module.Touch, module.Expression:
		// Out of scope, same reasoning as thru: not a control value, and MPE
		// forwarding would flood the port.
	}
}

// handleUIButton intercepts the editor's own bottom-strip buttons and, while
// armed, any other button press as a target selection. It reports whether
// the event was consumed by the editor — if not, the caller falls through to
// the normal passthrough/override path.
func (m *Module) handleUIButton(e module.Button) bool {
	switch e.CC {
	case editToggleCC:
		if e.Pressed {
			m.toggleEdit()
		}
		return true
	case editSaveCC:
		if e.Pressed && m.ui == stateEditing {
			m.saveDraft()
		}
		return true
	case editClearCC:
		if e.Pressed && m.ui == stateEditing && m.hasOverride {
			m.clearRule()
		}
		return true
	}

	switch m.ui {
	case stateArmed:
		if e.Pressed {
			m.armTarget("cc", e.CC, 0)
		}
		return true
	case stateEditing:
		return true // swallowed: not a new selection mid-edit
	default:
		return false
	}
}

// toggleEdit cycles: off -> armed (enter edit mode); armed -> off (exit);
// editing -> armed (cancel the current draft, stay in edit mode). The
// button's own LED mirrors edit mode: lit for armed and editing, off only
// when back to plain passthrough.
func (m *Module) toggleEdit() {
	switch m.ui {
	case stateOff:
		m.ui = stateArmed
		m.host.SetButton(editToggleCC, 127)
	case stateEditing:
		m.ui = stateArmed
	default: // stateArmed
		m.ui = stateOff
		m.host.SetButton(editToggleCC, 0)
	}
}

// armTarget selects a control for editing: ch is the pad's channel (ignored
// for "cc" targets, which are always channel 1 on the screen). Loads the
// existing rule into the draft if one exists, else a passthrough-identical
// default.
func (m *Module) armTarget(kind string, num byte, ch int) {
	m.sel = target{kind: kind, num: num}
	key := srcKey(kind, num)
	if rule, ok := m.overrides[key]; ok {
		m.draft = rule
		m.hasOverride = true
	} else {
		m.draft = defaultDraft(kind, num, ch)
		m.hasOverride = false
	}
	m.ui = stateEditing
}

// defaultDraft mirrors handlePad/handleButton/handleEncoder's no-rule
// (thru-identical) behaviour, as the starting point for a new rule.
func defaultDraft(kind string, num byte, ch int) MidiMapping {
	if kind == "note" {
		return MidiMapping{OutType: "note", OutCh: byte(ch), OutNum: num, OutMin: 0, OutMax: 127}
	}
	return MidiMapping{OutType: "cc", OutCh: outChannel, OutNum: num, OutMin: 0, OutMax: 127}
}

// editField adjusts one field of the in-progress draft per the top-row
// encoder that fired: 0 Out Type, 1 Out Channel, 2 Out Value, 3 Out Min,
// 4 Out Max. Encoders 6-8 (index 5-7) are unused.
func (m *Module) editField(e module.Encoder) {
	if e.Delta == 0 {
		return
	}
	switch e.Index {
	case 0:
		if e.Delta > 0 {
			m.draft.OutType = "note"
		} else {
			m.draft.OutType = "cc"
		}
	case 1:
		m.draft.OutCh = byte(push3.ClampInt(int(m.draft.OutCh)+e.Delta, 1, 16))
	case 2:
		m.draft.OutNum = byte(push3.ClampInt(int(m.draft.OutNum)+e.Delta, 0, 127))
	case 3:
		m.draft.OutMin = byte(push3.ClampInt(int(m.draft.OutMin)+e.Delta, 0, 127))
	case 4:
		m.draft.OutMax = byte(push3.ClampInt(int(m.draft.OutMax)+e.Delta, 0, 127))
	}
}

// saveDraft commits the in-progress draft as the override for the selected
// target and returns to armed so another control can be picked.
func (m *Module) saveDraft() {
	m.overrides[srcKey(m.sel.kind, m.sel.num)] = m.draft
	m.persist()
	m.ui = stateArmed
}

// clearRule deletes the override for the selected target, reverting it to
// passthrough, and returns to armed.
func (m *Module) clearRule() {
	delete(m.overrides, srcKey(m.sel.kind, m.sel.num))
	m.persist()
	m.ui = stateArmed
}

// persist writes the overrides table to the module's Store. remap has never
// called Set before this — every prior rule was hand-edited into the file.
func (m *Module) persist() {
	if err := m.host.Store().Set(&doc{Overrides: m.overrides}); err != nil {
		m.host.Log("saving overrides: %v", err)
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
	title := fmt.Sprintf("pushapp - remap  (%d override(s))", len(m.overrides))
	switch m.ui {
	case stateArmed:
		title = "pushapp - remap  [ARMED - pick a control]"
	case stateEditing:
		kind := "CC"
		if m.sel.kind == "note" {
			kind = "NOTE"
		}
		title = fmt.Sprintf("pushapp - remap  EDITING %s %d", kind, m.sel.num)
	}
	f.Text(8, 14, title, t.CrumbCol)
	el := time.Since(m.start).Seconds()
	if el < 0.001 {
		el = 0.001
	}
	stats := fmt.Sprintf("%d sent   %.0f fps", m.sent, float64(m.frames)/el)
	f.Text(w-8-text.Width(stats), 14, stats, t.Gray)

	if m.ui == stateEditing {
		m.drawEditing(f, w, h, t)
		return
	}
	m.drawOverview(f, w, h, t)
}

// drawOverview covers stateOff and stateArmed: the overrides list plus the
// recent-activity log, unchanged either way. Only the bottom strip differs:
// the Remap button is always shown there so it's clear which physical button
// toggles edit mode, and its on-screen state (State: SoftOn) plus its own
// lit LED (toggleEdit) together show whether it's currently armed — no
// separate banner text needed.
func (m *Module) drawOverview(f *module.Frame, w, h int, t module.Theme) {
	// Overrides in effect, so the on-screen state matches whatever was
	// hand-edited into the config file or saved via the editor. KVRows
	// rather than a bespoke list: this is exactly the label:value shape it
	// exists for.
	rows := make([]module.KVRow, 0, len(m.overrides))
	for key, rule := range m.overrides {
		val := fmt.Sprintf("-> %s %d [%d-%d]", rule.OutType, rule.OutNum, rule.OutMin, rule.OutMax)
		rows = append(rows, module.KVRow{Label: key, Value: val, ValueCol: t.Accent})
	}
	f.Text(10, 34, "OVERRIDES", t.Gray)
	if len(rows) == 0 {
		f.Text(10, 50, "(none - passthrough, same as thru)", t.Gray)
	} else {
		f.KVRows(40, 400, 14, 100, h-38, rows)
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

	// An error takes over the whole strip (it needs the OffColor background
	// to stay noticeable); otherwise the Remap button + a hint carry the
	// same information the old status bar did.
	if m.ui == stateOff && m.lastErr != "" {
		f.Rect(0, h-18, w, 18, t.OffColor)
		f.Text(8, h-5, "send error: "+text.Truncate(m.lastErr, 90), t.White)
		return
	}

	var buttons [8]module.SoftButton
	buttons[0] = module.SoftButton{Label: "REMAP"}
	hint := m.lastSent
	if hint == "" {
		hint = "press a pad, turn an encoder, or press a button"
	}
	if m.ui == stateArmed {
		buttons[0].State = widgets.SoftOn
		hint = "pick a pad, button, or encoder to map"
	}
	f.BotStrip(h-18, w, w/8, 18, buttons, text.Truncate(hint, 60))
}

// drawEditing shows the in-progress draft for the selected target: one
// field per top-row encoder, its label and current value both centered
// under that encoder's own column — the same centering BotStrip uses for
// its buttons — plus Cancel/Clear/Save in the bottom strip.
func (m *Module) drawEditing(f *module.Frame, w, h int, t module.Theme) {
	colW := w / 8
	typeVal := "CC"
	if m.draft.OutType == "note" {
		typeVal = "NOTE"
	}
	labels := [5]string{"TYPE", "CHAN", "VALUE", "MIN", "MAX"}
	values := [5]string{
		typeVal,
		fmt.Sprintf("%d", m.draft.OutCh),
		fmt.Sprintf("%d", m.draft.OutNum),
		fmt.Sprintf("%d", m.draft.OutMin),
		fmt.Sprintf("%d", m.draft.OutMax),
	}
	const valueScale = 2 // the field being dialed in should read as the important number, not just a different color
	for i, label := range labels {
		x := i * colW
		f.Text(x+(colW-text.Width(label))/2, 44, label, t.Gray)
		f.TextScaled(x+(colW-text.WidthScaled(values[i], valueScale))/2, 76, values[i], t.White, valueScale)
	}

	var buttons [8]module.SoftButton
	buttons[0] = module.SoftButton{Label: "CANCEL"}
	clearState := widgets.SoftOff
	if m.hasOverride {
		clearState = widgets.SoftNeutral
	}
	buttons[6] = module.SoftButton{Label: "CLEAR", State: clearState}
	buttons[7] = module.SoftButton{Label: "SAVE", State: widgets.SoftOn}
	f.BotStrip(h-18, w, colW, 18, buttons, "")
}
