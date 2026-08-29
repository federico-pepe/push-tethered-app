package main

import (
	"fmt"

	"github.com/federico-pepe/push-tethered-app/internal/catalog"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// apiVersion is bumped whenever a PushService method's signature changes in a
// way an old frontend build cannot safely call — which every change in this
// file is, since single-session methods became session-keyed. The frontend
// checks this once at startup (see main.ts) and asks for a reload on
// mismatch, rather than this file carrying old and new method shims side by
// side.
const apiVersion = 4

// errNoSession is what every session-scoped method returns for an unknown or
// no-longer-connected session key.
var errNoSession = fmt.Errorf("no such session")

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

// SessionInfo is one connected Push, as shown on its session card.
type SessionInfo struct {
	Key        string        `json:"key"`
	Unit       string        `json:"unit"`
	DisplaySel string        `json:"displaySel"`
	MIDIIn     pmidi.PortRef `json:"midiIn"`
	ExtMIDIIn  bool          `json:"extMidiIn"`
	ExtMIDIOut bool          `json:"extMidiOut"`
}

// Overview is everything the pairing view and the session list need, fetched
// in one round trip. With N sessions, polling IsConnected + LastError +
// ListModules per session the way the single-session UI used to would be
// 1+N calls every tick for no reason.
type Overview struct {
	Sessions   []SessionInfo     `json:"sessions"`
	Units      []display.Info    `json:"units"`
	MIDIUnits  []pmidi.Unit      `json:"midiUnits"`
	UnitErrors map[string]string `json:"unitErrors"` // by unit key — see hostManager.lastErrors
}

// PushService is the bound Go object the frontend calls into. It is a thin
// wrapper over hostManager and host.Runtime's own control API — no new
// behaviour lives here beyond JSON-shaping and, for InstallModulePrompt, the
// native folder picker a webview cannot provide itself.
type PushService struct {
	mgr *hostManager
}

// NewPushService wraps a hostManager, which may or may not have any sessions
// connected yet.
func NewPushService(mgr *hostManager) *PushService {
	return &PushService{mgr: mgr}
}

// APIVersion lets the frontend detect a stale cached build against a rebuilt
// backend — see main.ts's startup check. Bump this, not the method
// signatures below it, when adding a genuinely additive method; bump it and
// expect the frontend to reload for anything that changes an existing one.
func (s *PushService) APIVersion() int { return apiVersion }

// Overview reports every connected session plus every USB and MIDI unit
// currently visible, for the pairing view and the session list in one call.
func (s *PushService) Overview() Overview {
	units, err := display.List()
	if err != nil {
		units = nil // the pairing view shows "no units found" either way
	}
	sessions := s.mgr.list()
	sessInfos := make([]SessionInfo, len(sessions))
	for i, si := range sessions {
		sessInfos[i] = SessionInfo{
			Key: si.Key, Unit: si.Unit, DisplaySel: si.DisplaySel, MIDIIn: si.MIDIIn,
			ExtMIDIIn: si.ExtMIDIIn, ExtMIDIOut: si.ExtMIDIOut,
		}
	}
	return Overview{
		Sessions:   sessInfos,
		Units:      units,
		MIDIUnits:  pmidi.ListUnits(),
		UnitErrors: s.mgr.lastErrors(),
	}
}

// ListMIDIPorts lists every MIDI input port name the OS currently sees. Kept
// as a flat fallback alongside Overview's grouped MIDIUnits, for whatever the
// grouping logic cannot make sense of on an untested platform.
func (s *PushService) ListMIDIPorts() []string {
	return s.mgr.ports()
}

// IdentifyUnit flashes sel's display for a few seconds — see
// hostManager.identifyUnit. Only meaningful for a unit not already shown as a
// session card.
func (s *PushService) IdentifyUnit(sel string) error {
	return s.mgr.identifyUnit(sel)
}

// IdentifyMIDIPort flashes every pad on outNum for a few seconds — the
// disambiguation path for a PortRef the pairing logic marked Ambiguous, where
// there is no automatic answer for which physical unit a colliding cable
// belongs to.
func (s *PushService) IdentifyMIDIPort(outNum int) error {
	return s.mgr.identifyMIDIPort(outNum)
}

// Connect pairs and claims the unit and cable named by req, starting a new
// session, and returns its key. An empty req reproduces the historical
// single-device auto-detect, which still works as long as only one Push is
// attached.
func (s *PushService) Connect(req ConnectRequest) (string, error) {
	return s.mgr.connect(req)
}

// Disconnect stops one session and releases its hardware.
func (s *PushService) Disconnect(key string) error {
	return s.mgr.disconnect(key)
}

// ListModules returns every available module for the given session —
// compiled-in and installed, marking which one is active.
func (s *PushService) ListModules(sessionKey string) ([]ModuleInfo, error) {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return nil, errNoSession
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

// ActivateModule switches the given session to the named module.
func (s *PushService) ActivateModule(sessionKey, id string) error {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return errNoSession
	}
	return rt.Activate(id)
}

// OpenMirror opens the given session's live screen mirror in the system's
// default browser — the same stream the in-app "Live screen" overlay shows,
// just outside the app window (handy for screen-sharing just the Push
// display during a demo). Errors (no such session, no default browser
// configured) surface to the caller rather than being swallowed, since
// there's no other feedback path for a failed OS-level open.
func (s *PushService) OpenMirror(sessionKey string) error {
	if _, ok := s.mgr.mirrorHub(sessionKey); !ok {
		return errNoSession
	}
	url := fmt.Sprintf("http://%s/screen/%s", mirrorAddr, sessionKey)
	return application.Get().Browser.OpenURL(url)
}

// InstallModulePrompt opens a native "choose a folder or archive" dialog and
// installs whatever the user picks, through the given session's Runtime.
// Runtime.Install accepts either a directory or a .tar.gz/.tgz archive (see
// internal/host/procmod.InstallFromPath), so this dialog allows choosing
// either. The installed-modules directory is shared process-wide, so the
// module becomes available to every session — but only after each one
// reloads its own installed list; a session other than sessionKey will not
// see it until its own ListModules is next called following a fresh
// LoadInstalled (see plans/2026-08-19-multi-device.md's open question on
// this).
func (s *PushService) InstallModulePrompt(sessionKey string) (ModuleInfo, error) {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return ModuleInfo{}, errNoSession
	}

	path, err := application.Get().Dialog.OpenFile().
		SetTitle("Select a module folder (containing manifest.json) or a .tar.gz/.tgz archive").
		CanChooseDirectories(true).
		CanChooseFiles(true).
		AddFilter("Module archive", "*.tar.gz;*.tgz").
		PromptForSingleSelection()
	if err != nil {
		return ModuleInfo{}, fmt.Errorf("choosing a folder or archive: %w", err)
	}
	if path == "" {
		return ModuleInfo{}, nil // user cancelled
	}

	meta, err := rt.Install(path)
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

// UninstallModule removes a process-loaded module via the given session.
// Refuses (via the error Runtime.Uninstall already returns) for the active
// module or a compiled-in one.
func (s *PushService) UninstallModule(sessionKey, id string) error {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return errNoSession
	}
	return rt.Uninstall(id)
}

// UpdateInfo is one installed module with a newer catalog release available.
type UpdateInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	InstalledVersion string `json:"installedVersion"`
	LatestVersion    string `json:"latestVersion"`
}

// CatalogList fetches the module catalog (internal/catalog.DefaultCatalogURL)
// for the "Browse Catalog" panel. This is a network call with no session or
// hardware dependency, unlike every other PushService method here — a
// sessionKey isn't needed, but every other catalog method below takes one
// since installing/updating still goes through a session's Runtime.
func (s *PushService) CatalogList() ([]catalog.Entry, error) {
	cat, err := catalog.Fetch(catalog.DefaultCatalogURL)
	if err != nil {
		return nil, err
	}
	return cat.Entries, nil
}

// CatalogInstall resolves id's catalog entry, downloads its latest release,
// and installs it through the given session's Runtime — the same
// rt.Install used by InstallModulePrompt, just fed a downloaded tarball
// instead of a user-picked path.
func (s *PushService) CatalogInstall(sessionKey, id string) (ModuleInfo, error) {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return ModuleInfo{}, errNoSession
	}

	dir, cleanup, err := catalogDownload(id)
	if err != nil {
		return ModuleInfo{}, err
	}
	defer cleanup()

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

// CatalogUpdate re-downloads id's latest catalog release and replaces the
// already-installed module's files via rt.Update.
func (s *PushService) CatalogUpdate(sessionKey, id string) (ModuleInfo, error) {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return ModuleInfo{}, errNoSession
	}

	dir, cleanup, err := catalogDownload(id)
	if err != nil {
		return ModuleInfo{}, err
	}
	defer cleanup()

	meta, err := rt.Update(id, dir)
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

// CatalogCheckUpdates cross-references the given session's installed
// modules against the catalog by id and reports which have a newer release.
func (s *PushService) CatalogCheckUpdates(sessionKey string) ([]UpdateInfo, error) {
	rt, ok := s.mgr.session(sessionKey)
	if !ok {
		return nil, errNoSession
	}

	cat, err := catalog.Fetch(catalog.DefaultCatalogURL)
	if err != nil {
		return nil, err
	}

	var updates []UpdateInfo
	for _, m := range rt.List() {
		if !rt.IsInstalled(m.ID) {
			continue
		}
		entry, err := cat.Find(m.ID)
		if err != nil {
			continue // not a catalog module, or no longer listed
		}
		available, latest, _, err := catalog.CheckUpdate(entry, m.Version)
		if err != nil || !available {
			continue
		}
		updates = append(updates, UpdateInfo{
			ID: m.ID, Name: m.Name, InstalledVersion: m.Version, LatestVersion: latest,
		})
	}
	return updates, nil
}

// catalogDownload resolves id's catalog entry and downloads its latest
// release, shared by CatalogInstall and CatalogUpdate.
func catalogDownload(id string) (dir string, cleanup func(), err error) {
	cat, err := catalog.Fetch(catalog.DefaultCatalogURL)
	if err != nil {
		return "", nil, err
	}
	entry, err := cat.Find(id)
	if err != nil {
		return "", nil, err
	}
	downloadURL, _, err := catalog.ResolveAsset(entry)
	if err != nil {
		return "", nil, err
	}
	return catalog.DownloadAndExtract(downloadURL)
}
