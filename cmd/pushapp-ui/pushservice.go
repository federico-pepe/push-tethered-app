package main

import (
	"fmt"

	"github.com/federico-pepe/push-tethered-app/internal/host"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ModuleInfo is the JSON shape sent to the frontend.
//
// A dedicated type rather than binding module.Meta directly: it keeps the
// frontend's contract explicit and stable even if module.Meta's own fields
// change, and it carries Active/Installed, which are properties of "this
// module right now" rather than of the module itself.
type ModuleInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Active       bool   `json:"active"`
	NeedsMIDIOut bool   `json:"needsMidiOut"`

	// Installed distinguishes a process-loaded module from a compiled-in one
	// — only the former can be uninstalled, so the frontend uses this to
	// decide whether to offer that control at all rather than show it and
	// surface a refusal error after the fact.
	Installed bool `json:"installed"`
}

// PushService is the bound Go object the frontend calls into. It is a thin
// wrapper over host.Runtime's own control API (List/Active/Activate/Install/
// Uninstall) — no new behaviour lives here, only the JSON-shaping and, for
// InstallModulePrompt, the native folder picker a webview cannot provide
// itself.
type PushService struct {
	rt *host.Runtime
}

// NewPushService wraps an already-running Runtime.
func NewPushService(rt *host.Runtime) *PushService {
	return &PushService{rt: rt}
}

// ListModules returns every available module — compiled-in and installed —
// marking which one is active.
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
			Installed:    s.rt.IsInstalled(m.ID),
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

// InstallModulePrompt opens a native "choose a folder" dialog and installs
// whatever the user picks. The dialog has to happen on the Go side: a webview
// has no access to real filesystem paths, only to files the user drags in or
// picks through its own sandboxed file input — neither gives us a directory
// path to hand to Runtime.Install. Returns the zero value with no error if the
// user cancels, so the frontend can tell "cancelled" apart from "failed".
func (s *PushService) InstallModulePrompt() (ModuleInfo, error) {
	dir, err := application.Get().Dialog.OpenFile().
		SetTitle("Select a module folder (containing manifest.json)").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		return ModuleInfo{}, fmt.Errorf("choosing a folder: %w", err)
	}
	if dir == "" {
		return ModuleInfo{}, nil // user cancelled
	}

	meta, err := s.rt.Install(dir)
	if err != nil {
		return ModuleInfo{}, err
	}
	return ModuleInfo{ID: meta.ID, Name: meta.Name, NeedsMIDIOut: meta.NeedsMIDIOut, Installed: true}, nil
}

// UninstallModule removes a process-loaded module. Refuses (via the error
// Runtime.Uninstall already returns) for the active module or a compiled-in
// one — the frontend only offers this for modules where ModuleInfo.Installed
// is true and Active is false, but the refusal is still real defence, not
// just a UI nicety, since nothing stops a second client of this same Runtime
// from acting concurrently.
func (s *PushService) UninstallModule(id string) error {
	return s.rt.Uninstall(id)
}
