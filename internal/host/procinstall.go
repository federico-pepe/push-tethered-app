package host

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/federico-pepe/push-tethered-app/internal/host/procmod"
	"github.com/federico-pepe/push-tethered-app/internal/module"
)

// installedModule is a process-loaded module plus where its files live, so
// Uninstall knows what to delete.
type installedModule struct {
	mod module.Module
	dir string
}

// findModule looks up a module by ID across both the compiled-in set and the
// installed set — Activate does not need to know or care which kind it got.
func (r *Runtime) findModule(id string) module.Module {
	for _, m := range r.modules {
		if m.Meta().ID == id {
			return m
		}
	}
	r.installedMu.RLock()
	defer r.installedMu.RUnlock()
	for _, im := range r.installed {
		if im.mod.Meta().ID == id {
			return im.mod
		}
	}
	return nil
}

// IsInstalled reports whether id refers to a process-loaded module rather
// than a compiled-in one — the UI needs this to know whether an Uninstall
// button makes sense for a given module at all.
func (r *Runtime) IsInstalled(id string) bool {
	r.installedMu.RLock()
	defer r.installedMu.RUnlock()
	for _, im := range r.installed {
		if im.mod.Meta().ID == id {
			return true
		}
	}
	return false
}

// Install copies a module directory (manifest.json plus its executable and
// assets — see internal/host/procmod's package doc for the format) into this
// app's own config location and registers it so it's usable immediately,
// with no restart needed. The disk operation itself (validate, copy, refuse
// a duplicate) is procmod.Install, which needs no Runtime or hardware, so a
// CLI flag can install a module offline; this method adds the one thing that
// needs a live Runtime — the collision check against compiled-in modules, and
// registering the result so Activate can find it right away.
func (r *Runtime) Install(srcDir string) (module.Meta, error) {
	man, err := procmod.LoadManifest(srcDir)
	if err != nil {
		return module.Meta{}, fmt.Errorf("install: %w", err)
	}
	if r.findModule(man.ID) != nil {
		return module.Meta{}, fmt.Errorf("install: a module with id %q already exists", man.ID)
	}

	if _, err := procmod.Install(srcDir); err != nil {
		return module.Meta{}, err
	}
	root, err := procmod.InstalledRoot()
	if err != nil {
		return module.Meta{}, fmt.Errorf("install: %w", err)
	}
	destDir := filepath.Join(root, man.ID)

	proc, err := procmod.New(destDir)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return module.Meta{}, fmt.Errorf("install: %w", err)
	}

	r.installedMu.Lock()
	r.installed = append(r.installed, &installedModule{mod: proc, dir: destDir})
	r.installedMu.Unlock()

	log.Printf("module: installed %s (%s) at %s", proc.Meta().Name, proc.Meta().ID, destDir)
	return proc.Meta(), nil
}

// Uninstall removes a process-loaded module: refuses if it's the active
// module (switch away first, same as you can't delete a program that's
// running) and refuses for a compiled-in module (there is nothing on disk to
// remove, and it will exist again next time this binary runs regardless).
func (r *Runtime) Uninstall(id string) error {
	if active := r.Active(); active.ID == id {
		return fmt.Errorf("uninstall: %q is active; switch to another module first", id)
	}

	r.installedMu.Lock()
	defer r.installedMu.Unlock()
	for i, im := range r.installed {
		if im.mod.Meta().ID != id {
			continue
		}
		r.installed = append(r.installed[:i], r.installed[i+1:]...)
		if err := os.RemoveAll(im.dir); err != nil {
			return fmt.Errorf("uninstall: removing %s: %w", im.dir, err)
		}
		log.Printf("module: uninstalled %s", id)
		return nil
	}

	for _, m := range r.modules {
		if m.Meta().ID == id {
			return fmt.Errorf("uninstall: %q is a built-in module, not removable", id)
		}
	}
	return fmt.Errorf("no module with id %q", id)
}

// LoadInstalled scans for modules a previous run installed and registers
// them, without copying anything — they are already in place. Call once,
// after New and before Run; a scan failure is not fatal to starting the app,
// since the compiled-in modules still work regardless.
func (r *Runtime) LoadInstalled() error {
	root, err := procmod.InstalledRoot()
	if err != nil {
		return fmt.Errorf("finding installed modules: %w", err)
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", root, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		proc, err := procmod.New(dir)
		if err != nil {
			log.Printf("module: skipping %s: %v", dir, err)
			continue
		}
		// A compiled-in module always wins the ID — findModule checks r.modules
		// first — so an installed one with the same ID would be silently
		// unreachable. Warn rather than let that happen quietly; Install()
		// itself already refuses this at the point of installing, but an
		// installed-modules directory can still end up here by hand-copying
		// files, or after a new built-in module ships under an ID someone
		// already used.
		if r.findModule(proc.Meta().ID) != nil {
			log.Printf("module: skipping %s: id %q collides with an existing module", dir, proc.Meta().ID)
			continue
		}
		r.installedMu.Lock()
		r.installed = append(r.installed, &installedModule{mod: proc, dir: dir})
		r.installedMu.Unlock()
		log.Printf("module: loaded %s (%s) from %s", proc.Meta().Name, proc.Meta().ID, dir)
	}
	return nil
}
