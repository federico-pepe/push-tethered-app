// Package updatecheck asks GitHub whether a newer release of this app
// exists. It only looks; it never downloads or installs anything. See
// plans/2026-10-01-update-check.md.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository whose releases are checked.
const Repo = "federico-pepe/push-tethered-app"

// githubAPIBase is a seam over the GitHub API host, overridable in tests.
var githubAPIBase = "https://api.github.com"

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Result describes a newer release.
type Result struct {
	Version string `json:"version"` // tag, e.g. "v0.2.0-beta"
	URL     string `json:"url"`     // release page
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Check returns the newest release that is newer than current, or nil if
// the app is up to date. It returns nil, nil when current is "dev" or not a
// version: a local build has nothing to compare.
//
// A stable build (no pre-release suffix) is only offered stable releases. A
// pre-release build is offered every release.
func Check(ctx context.Context, current string) (*Result, error) {
	cur, ok := parse(current)
	if !ok {
		return nil, nil
	}

	url := fmt.Sprintf("%s/repos/%s/releases?per_page=30", githubAPIBase, Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check: %s: %s", url, resp.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("update check: %w", err)
	}
	return newest(cur, releases), nil
}

// newest picks the highest-versioned release that beats cur. GitHub orders
// the list by creation date, so the first entry is not necessarily the
// highest version.
func newest(cur version, releases []githubRelease) *Result {
	var best *Result
	var bestV version
	for _, r := range releases {
		v, ok := parse(r.TagName)
		if r.Draft || !ok {
			continue
		}
		if cur.pre == "" && v.pre != "" {
			continue // stable build: stable releases only
		}
		if compare(v, cur) <= 0 {
			continue
		}
		if best == nil || compare(v, bestV) > 0 {
			best, bestV = &Result{Version: r.TagName, URL: r.HTMLURL}, v
		}
	}
	return best
}

// Compare orders two version strings by semver precedence: -1, 0 or 1. A
// string that does not parse sorts below every one that does.
func Compare(a, b string) int {
	va, oka := parse(a)
	vb, okb := parse(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	return compare(va, vb)
}

type version struct {
	num [3]int
	pre string // text after the first "-", empty for a stable release
}

func parse(s string) (version, bool) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s, v.pre = s[:i], s[i+1:]
		if v.pre == "" {
			return v, false
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.num[i] = n
	}
	return v, true
}

func compare(a, b version) int {
	for i := range a.num {
		if a.num[i] != b.num[i] {
			return sign(a.num[i] - b.num[i])
		}
	}
	// Same numbers: a release outranks any pre-release of it.
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}
	return comparePre(a.pre, b.pre)
}

// comparePre follows semver 2.0.0 section 11: dot-separated identifiers,
// numeric ones compare as numbers and sort below text ones, text compares
// in ASCII order, and a shorter list sorts below a longer one it prefixes.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := compareIdent(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return sign(len(as) - len(bs))
}

func compareIdent(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return sign(an - bn)
	case aerr == nil:
		return -1
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
