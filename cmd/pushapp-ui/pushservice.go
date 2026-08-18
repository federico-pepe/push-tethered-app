package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// errNotConnected is what every module-list/control method returns while no
// MIDI port has been picked yet — see hostManager and ListMIDIPorts/Connect.
var errNotConnected = fmt.Errorf("not connected to Push yet")

// ModuleInfo is the JSON shape sent to the frontend.
//
// A dedicated type rather than binding module.Meta directly: it keeps the
// frontend's contract explicit and stable even if module.Meta's own fields
// change, and it carries Active/Installed, which are properties of "this
// module right now" rather than of the module itself.
type ModuleInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Description  string `json:"description"`
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
	mgr *hostManager
}

// NewPushService wraps a hostManager, which may or may not be connected yet.
func NewPushService(mgr *hostManager) *PushService {
	return &PushService{mgr: mgr}
}

// IsConnected reports whether a MIDI port has been claimed and the host is
// running. The frontend polls this to decide between the port-picker view and
// the module-list view.
func (s *PushService) IsConnected() bool {
	_, ok := s.mgr.connected()
	return ok
}

// LastError reports why the host stopped on its own since the last successful
// connect — e.g. the device was unplugged. Empty if there is nothing to
// report. The frontend reads this once IsConnected turns false unexpectedly,
// to show why rather than silently reverting to the port-picker.
func (s *PushService) LastError() string {
	if err := s.mgr.lastError(); err != nil {
		return err.Error()
	}
	return ""
}

// ListMIDIPorts lists every MIDI input port the OS currently sees, for the
// port-picker view shown when auto-detect fails (see hostManager.connect).
func (s *PushService) ListMIDIPorts() []string {
	return s.mgr.ports()
}

// ConnectMIDIPort claims the named MIDI port and starts the host. name must be
// one of ListMIDIPorts' results — Push's Live port on most setups is the one
// whose name mentions Push and either ends in "Live Port" or, on Windows,
// carries no "MIDIINn (...)" prefix at all.
func (s *PushService) ConnectMIDIPort(name string) error {
	return s.mgr.connect(name)
}

// ListModules returns every available module — compiled-in and installed —
// marking which one is active.
func (s *PushService) ListModules() ([]ModuleInfo, error) {
	rt, ok := s.mgr.connected()
	if !ok {
		return nil, errNotConnected
	}
	active := rt.Active().ID
	metas := rt.List()
	out := make([]ModuleInfo, 0, len(metas))
	for _, m := range metas {
		out = append(out, ModuleInfo{
			ID:           m.ID,
			Name:         m.Name,
			Version:      m.Version,
			Description:  m.Description,
			Active:       m.ID == active,
			NeedsMIDIOut: m.NeedsMIDIOut,
			Installed:    rt.IsInstalled(m.ID),
		})
	}
	return out, nil
}

// ActivateModule switches the host to the named module. The frontend re-reads
// ListModules afterward rather than relying on a return value here, so one
// call site handles both success and the "which one is active now" question.
func (s *PushService) ActivateModule(id string) error {
	rt, ok := s.mgr.connected()
	if !ok {
		return errNotConnected
	}
	return rt.Activate(id)
}

// InstallModulePrompt opens a native "choose a folder" dialog and installs
// whatever the user picks. The dialog has to happen on the Go side: a webview
// has no access to real filesystem paths, only to files the user drags in or
// picks through its own sandboxed file input — neither gives us a directory
// path to hand to Runtime.Install. Returns the zero value with no error if the
// user cancels, so the frontend can tell "cancelled" apart from "failed".
func (s *PushService) InstallModulePrompt() (ModuleInfo, error) {
	rt, ok := s.mgr.connected()
	if !ok {
		return ModuleInfo{}, errNotConnected
	}

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

	meta, err := rt.Install(dir)
	if err != nil {
		return ModuleInfo{}, err
	}
	return ModuleInfo{
		ID:           meta.ID,
		Name:         meta.Name,
		Version:      meta.Version,
		Description:  meta.Description,
		NeedsMIDIOut: meta.NeedsMIDIOut,
		Installed:    true,
	}, nil
}

// UninstallModule removes a process-loaded module. Refuses (via the error
// Runtime.Uninstall already returns) for the active module or a compiled-in
// one — the frontend only offers this for modules where ModuleInfo.Installed
// is true and Active is false, but the refusal is still real defence, not
// just a UI nicety, since nothing stops a second client of this same Runtime
// from acting concurrently.
func (s *PushService) UninstallModule(id string) error {
	rt, ok := s.mgr.connected()
	if !ok {
		return errNotConnected
	}
	return rt.Uninstall(id)
}
