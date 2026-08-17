package main

import "github.com/federico-pepe/push-tethered-app/internal/host"

// ModuleInfo is the JSON shape sent to the frontend.
//
// A dedicated type rather than binding module.Meta directly: it keeps the
// frontend's contract explicit and stable even if module.Meta's own fields
// change, and it carries Active, which is a property of "this module right
// now" rather than of the module itself.
type ModuleInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Active       bool   `json:"active"`
	NeedsMIDIOut bool   `json:"needsMidiOut"`
}

// PushService is the bound Go object the frontend calls into. It is a thin
// wrapper over host.Runtime's own control API (List/Active/Activate) — no
// new behaviour lives here, only the JSON-shaping.
type PushService struct {
	rt *host.Runtime
}

// NewPushService wraps an already-running Runtime.
func NewPushService(rt *host.Runtime) *PushService {
	return &PushService{rt: rt}
}

// ListModules returns every compiled-in module, marking which one is active.
func (s *PushService) ListModules() []ModuleInfo {
	active := s.rt.Active().ID
	metas := s.rt.List()
	out := make([]ModuleInfo, 0, len(metas))
	for _, m := range metas {
		out = append(out, ModuleInfo{
			ID:           m.ID,
			Name:         m.Name,
			Active:       m.ID == active,
			NeedsMIDIOut: m.NeedsMIDIOut,
		})
	}
	return out
}

// ActivateModule switches the host to the named module. The frontend re-reads
// ListModules afterward rather than relying on a return value here, so one
// call site handles both success and the "which one is active now" question.
func (s *PushService) ActivateModule(id string) error {
	return s.rt.Activate(id)
}
