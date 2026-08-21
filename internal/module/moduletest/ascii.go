package moduletest

import (
	"encoding/json"

	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// NonASCIIStrings scans every text-bearing field in a Frame's display list and
// returns any string that contains a byte outside printable ASCII.
//
// This exists because the host's own asciiOnly sanitiser silently replaces
// anything it catches — which is the right behaviour for defence in depth, but
// it also means a module author's own non-ASCII string (a stray em-dash pasted
// into a UI label, say) never fails loudly. "Draw emitted only known op kinds"
// tests were passing while remap.go was shipping a "?" where an em-dash used
// to be; this is the check that would have caught it — assert
// len(moduletest.NonASCIIStrings(f)) == 0 in a module's Draw test.
func NonASCIIStrings(f *module.Frame) []string {
	var bad []string
	check := func(s string) {
		if s == "" {
			return
		}
		for i := 0; i < len(s); i++ {
			if s[i] > 0x7E || s[i] < 0x20 {
				bad = append(bad, s)
				return
			}
		}
	}

	for _, op := range f.Ops() {
		switch op.Kind {
		case "text":
			var v module.TextParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.S)
			}
		case "header":
			var v module.HeaderParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.S)
			}
		case "kvrows":
			var v module.KVRowsParams
			if json.Unmarshal(op.Params, &v) == nil {
				for _, r := range v.Rows {
					check(r.Label)
					check(r.Value)
				}
			}
		case "list":
			var v module.ListParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.View.Breadcrumb)
				check(v.View.Status)
				check(v.View.EmptyText)
				for _, r := range v.View.Rows {
					check(r.Text)
				}
			}
		case "botstrip":
			var v module.BotStripParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.Hint)
				for _, b := range v.Buttons {
					check(b.Label)
				}
			}
		case "breadcrumb":
			var v module.BreadcrumbParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.Breadcrumb)
				check(v.Status)
			}
		case "statusbar":
			var v module.StatusBarParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.S)
			}
		case "hlist":
			var v module.HListParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.View.Breadcrumb)
				check(v.View.Status)
				check(v.View.EmptyText)
				for _, c := range v.View.Cols {
					check(c.Text)
				}
			}
		case "styledtext":
			var v module.StyledTextParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.S)
			}
		case "knob", "knobfull":
			var v module.KnobParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.K.Label)
			}
		case "fader":
			var v module.FaderParams
			if json.Unmarshal(op.Params, &v) == nil {
				check(v.K.Label)
			}
		}
	}
	return bad
}
