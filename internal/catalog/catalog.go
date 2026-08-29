// Package catalog fetches a hosted index of third-party push-tethered-app
// modules and resolves each entry to a downloadable release tarball.
//
// The catalog is an index of pointers, not a host: it names a GitHub repo
// and a release asset filename, module authors own their own releases.
// There is no checksum or signing check, deliberately — the trust model is
// the same as running any open-source release binary directly, and is the
// same call ableton-push-hack's own Push Catalog made (see its
// catalog/ARCHITECTURE.md). A hostile or compromised catalog entry, or a
// hostile release asset, can still run arbitrary code once installed and
// activated — same as any manually-installed module.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/federico-pepe/push-tethered-app/internal/archiveutil"
)

// DefaultCatalogURL is the built-in catalog source. Override with -catalog-url
// (cmd/pushapp) to test a fork's catalog before merging an entry upstream.
const DefaultCatalogURL = "https://raw.githubusercontent.com/federico-pepe/push-tethered-app/main/catalog/catalog.json"

// catalogVersion is the only catalog.json schema this package understands.
const catalogVersion = 1

// maxDownloadBytes caps a release asset download — a process module is a
// script plus small assets, not a multi-hundred-megabyte payload, so a much
// larger response is either a wrong URL or something to refuse rather than
// buffer.
const maxDownloadBytes = 50 * 1024 * 1024

var httpClient = &http.Client{Timeout: 30 * time.Second}

// githubAPIBase is a seam over the GitHub API host, overridable in tests
// (ResolveAsset otherwise always hits the real api.github.com).
var githubAPIBase = "https://api.github.com"

// Entry is one catalog.json listing.
type Entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	GithubRepo  string `json:"github_repo"`
	AssetName   string `json:"asset_name"`
}

// Catalog is catalog.json's top-level shape.
type Catalog struct {
	CatalogVersion int     `json:"catalog_version"`
	Entries        []Entry `json:"entries"`
}

// Fetch downloads and parses the catalog at url.
func Fetch(url string) (*Catalog, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching catalog: %s: %s", url, resp.Status)
	}

	var cat Catalog
	if err := json.NewDecoder(resp.Body).Decode(&cat); err != nil {
		return nil, fmt.Errorf("parsing catalog: %w", err)
	}
	if cat.CatalogVersion != catalogVersion {
		return nil, fmt.Errorf("catalog: unsupported catalog_version %d (expected %d)", cat.CatalogVersion, catalogVersion)
	}
	return &cat, nil
}

// Find returns the entry with the given id, or an error if none matches.
func (c *Catalog) Find(id string) (Entry, error) {
	for _, e := range c.Entries {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("catalog: no entry with id %q", id)
}

// githubAsset is the subset of a GitHub release asset this package needs.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// githubRelease is the subset of GitHub's releases/latest response this
// package needs.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// ResolveAsset finds entry's latest release on GitHub and returns the
// download URL for its named asset, plus the release's version tag.
func ResolveAsset(entry Entry) (downloadURL, version string, err error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, entry.GithubRepo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("resolving %s: %w", entry.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("resolving %s: %s: %s", entry.ID, url, resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("resolving %s: %w", entry.ID, err)
	}
	for _, a := range rel.Assets {
		if a.Name == entry.AssetName {
			return a.BrowserDownloadURL, rel.TagName, nil
		}
	}
	return "", "", fmt.Errorf("resolving %s: latest release %s has no asset named %q", entry.ID, rel.TagName, entry.AssetName)
}

// DownloadAndExtract downloads the tarball at url and extracts it, returning
// the module directory (already resolved past a single wrapping
// subdirectory, if present) and a cleanup func that removes every temporary
// file. The caller must call cleanup once done, typically via defer.
func DownloadAndExtract(url string) (dir string, cleanup func(), err error) {
	tmpFile, err := os.CreateTemp("", "push-tethered-app-download-*.tar.gz")
	if err != nil {
		return "", nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := download(url, tmpFile); err != nil {
		tmpFile.Close()
		return "", nil, err
	}
	if err := tmpFile.Close(); err != nil {
		return "", nil, fmt.Errorf("downloading %s: %w", url, err)
	}

	extracted, err := archiveutil.ExtractTarGz(tmpPath)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(extracted) }

	resolved, err := archiveutil.ResolveWrappedDir(extracted, "manifest.json")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("extracting archive: %w", err)
	}
	return resolved, cleanup, nil
}

func download(url string, w io.Writer) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	limited := io.LimitReader(resp.Body, maxDownloadBytes+1)
	n, err := io.Copy(w, limited)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if n > maxDownloadBytes {
		return fmt.Errorf("downloading %s: exceeds %d byte limit", url, maxDownloadBytes)
	}
	return nil
}

// CheckUpdate resolves entry's latest release and reports whether it is
// newer than installedVersion.
func CheckUpdate(entry Entry, installedVersion string) (available bool, latestVersion, downloadURL string, err error) {
	downloadURL, latestVersion, err = ResolveAsset(entry)
	if err != nil {
		return false, "", "", err
	}
	return CompareVersions(latestVersion, installedVersion) > 0, latestVersion, downloadURL, nil
}

// CompareVersions compares two version strings shaped like this project's
// own scheme, "vMAJOR.MINOR.PATCH[-alpha|-beta|-rc.N]" (a leading "v" and a
// prerelease suffix are both optional). Returns a positive number if a is
// newer than b, negative if older, 0 if equal. Falls back to a plain string
// compare if either side doesn't parse as three numeric segments, so an
// unexpected format degrades to "different means available" rather than
// erroring.
func CompareVersions(a, b string) int {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func parseVersion(v string) (segs [3]int, ok bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return segs, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return segs, false
		}
		segs[i] = n
	}
	return segs, true
}
