package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// cacheTTL is how long a saved "latest version" is used without asking
// GitHub again. GitHub's unauthenticated API allows 60 requests per hour per
// IP, and the UI and CLI would otherwise spend them on every catalog view and
// every update check.
const cacheTTL = 6 * time.Hour

// now is a seam over time.Now, so tests can age the cache.
var now = time.Now

// cachePath returns the cache file's path. A seam, so tests can point it at a
// temp directory instead of the real config directory.
var cachePath = func() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "push-tethered-app", "catalog-cache.json"), nil
}

// cacheRecord is what is known about one repo's latest release.
type cacheRecord struct {
	Version   string    `json:"version"`
	ETag      string    `json:"etag,omitempty"` // lets a later request ask "has it changed?"
	CheckedAt time.Time `json:"checkedAt"`
}

// cacheMu serializes the read-modify-write of the cache file. LatestVersions
// looks up several repos in parallel.
var cacheMu sync.Mutex

// The cache is keyed by GitHub repo ("owner/name"). It only saves requests,
// so every failure here (no config dir, unreadable or corrupt file, failed
// write) is ignored: the caller then asks GitHub, as it would with no cache.

func readCache() map[string]cacheRecord {
	recs := map[string]cacheRecord{}
	path, err := cachePath()
	if err != nil {
		return recs
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &recs)
	}
	if recs == nil {
		recs = map[string]cacheRecord{}
	}
	return recs
}

func cacheGet(repo string) (cacheRecord, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	rec, ok := readCache()[repo]
	return rec, ok
}

func cachePut(repo string, rec cacheRecord) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	path, err := cachePath()
	if err != nil {
		return
	}
	recs := readCache()
	recs[repo] = rec
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}
