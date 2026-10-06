package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain keeps every test off the real catalog-cache.json in the user's
// config directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "catalog-test-")
	if err != nil {
		panic(err)
	}
	cachePath = func() (string, error) { return filepath.Join(dir, "catalog-cache.json"), nil }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// cleanCache points the cache at an empty file for one test, and restores the
// shared one after.
func cleanCache(t *testing.T) {
	t.Helper()
	prev := cachePath
	path := filepath.Join(t.TempDir(), "catalog-cache.json")
	cachePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { cachePath = prev })
}

func TestFetchParsesCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Catalog{
			CatalogVersion: 1,
			Entries: []Entry{
				{ID: "hello-py", Name: "Hello", GithubRepo: "someone/hello-py", AssetName: "hello-py.tar.gz"},
			},
		})
	}))
	defer srv.Close()

	cat, err := Fetch(srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(cat.Entries) != 1 || cat.Entries[0].ID != "hello-py" {
		t.Errorf("Entries = %+v", cat.Entries)
	}

	if _, err := cat.Find("hello-py"); err != nil {
		t.Errorf("Find(hello-py): %v", err)
	}
	if _, err := cat.Find("does-not-exist"); err == nil {
		t.Error("Find did not error for an unknown id")
	}
}

func TestFetchRejectsWrongVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Catalog{CatalogVersion: 99})
	}))
	defer srv.Close()

	if _, err := Fetch(srv.URL); err == nil {
		t.Error("Fetch did not reject an unsupported catalog_version")
	}
}

func TestResolveAssetFindsNamedAsset(t *testing.T) {
	// ResolveAsset hits api.github.com directly, which isn't reachable/
	// deterministic in a unit test — this test exercises the JSON-decoding
	// and matching logic against a local server standing in for that shape,
	// via a small seam: reuse the same decode path by pointing at our own
	// server's URL through a package-level override.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.2.0",
			Assets: []githubAsset{
				{Name: "other.tar.gz", BrowserDownloadURL: "http://example.com/other.tar.gz"},
				{Name: "hello-py.tar.gz", BrowserDownloadURL: "http://example.com/hello-py.tar.gz"},
			},
		})
	}))
	defer srv.Close()

	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })

	url, version, err := ResolveAsset(Entry{ID: "hello-py", GithubRepo: "someone/hello-py", AssetName: "hello-py.tar.gz"})
	if err != nil {
		t.Fatalf("ResolveAsset: %v", err)
	}
	if url != "http://example.com/hello-py.tar.gz" || version != "v1.2.0" {
		t.Errorf("got url=%q version=%q", url, version)
	}
}

func TestResolveAssetMissingAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{TagName: "v1.0.0"})
	}))
	defer srv.Close()

	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })

	if _, _, err := ResolveAsset(Entry{ID: "hello-py", GithubRepo: "someone/hello-py", AssetName: "hello-py.tar.gz"}); err == nil {
		t.Error("ResolveAsset did not error when the asset is missing")
	}
}

func TestLatestVersionsSkipsFailures(t *testing.T) {
	cleanCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/good/") {
			json.NewEncoder(w).Encode(githubRelease{
				TagName: "v2.0.0",
				Assets:  []githubAsset{{Name: "m.tar.gz", BrowserDownloadURL: "http://example.com/m.tar.gz"}},
			})
			return
		}
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })

	got := LatestVersions([]Entry{
		{ID: "a", GithubRepo: "x/good/a", AssetName: "m.tar.gz"},
		{ID: "b", GithubRepo: "x/bad/b", AssetName: "m.tar.gz"},
	})
	if len(got) != 1 || got["a"] != "v2.0.0" {
		t.Errorf("got %v, want only a=v2.0.0", got)
	}
}

// releaseServer answers /releases/latest with tag, and counts requests. It
// honors If-None-Match the way GitHub does.
func releaseServer(t *testing.T, tag string, hits *int, sawIfNoneMatch *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			if sawIfNoneMatch != nil {
				*sawIfNoneMatch = inm
			}
			if inm == `"etag-1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("ETag", `"etag-1"`)
		json.NewEncoder(w).Encode(githubRelease{
			TagName: tag,
			Assets:  []githubAsset{{Name: "m.tar.gz", BrowserDownloadURL: "http://example.com/m.tar.gz"}},
		})
	}))
	t.Cleanup(srv.Close)
	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })
	return srv
}

// ageCache makes the cache look d older, by moving the clock forward.
func ageCache(t *testing.T, d time.Duration) {
	t.Helper()
	prev := now
	now = func() time.Time { return prev().Add(d) }
	t.Cleanup(func() { now = prev })
}

var cacheEntry = Entry{ID: "m", GithubRepo: "x/m", AssetName: "m.tar.gz"}

func TestLatestVersionFreshCacheMakesNoRequest(t *testing.T) {
	cleanCache(t)
	var hits int
	releaseServer(t, "v1.0.0", &hits, nil)
	for i := 0; i < 3; i++ {
		v, err := LatestVersion(cacheEntry)
		if err != nil || v != "v1.0.0" {
			t.Fatalf("call %d: got %q, %v", i, v, err)
		}
	}
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
}

func TestLatestVersionStaleUsesETag(t *testing.T) {
	cleanCache(t)
	var hits int
	var inm string
	releaseServer(t, "v1.0.0", &hits, &inm)
	if _, err := LatestVersion(cacheEntry); err != nil {
		t.Fatal(err)
	}
	ageCache(t, cacheTTL+time.Minute)
	v, err := LatestVersion(cacheEntry)
	if err != nil || v != "v1.0.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if hits != 2 || inm != `"etag-1"` {
		t.Errorf("hits = %d, If-None-Match = %q, want 2 and the saved ETag", hits, inm)
	}
	// The 304 renewed the record, so the next call is free again.
	if _, err := LatestVersion(cacheEntry); err != nil || hits != 2 {
		t.Errorf("hits = %d, err = %v, want no further request", hits, err)
	}
}

func TestLatestVersionOfflineKeepsStaleVersion(t *testing.T) {
	cleanCache(t)
	var hits int
	srv := releaseServer(t, "v1.0.0", &hits, nil)
	if _, err := LatestVersion(cacheEntry); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	ageCache(t, cacheTTL+time.Minute)
	v, err := LatestVersion(cacheEntry)
	if err != nil || v != "v1.0.0" {
		t.Errorf("got %q, %v, want the saved version and no error", v, err)
	}
}

func TestLatestVersionOfflineNoCacheIsError(t *testing.T) {
	cleanCache(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })
	srv.Close()
	if _, err := LatestVersion(cacheEntry); err == nil {
		t.Error("want an error when nothing is saved and GitHub is unreachable")
	}
}

func TestResolveAssetSavesVersion(t *testing.T) {
	cleanCache(t)
	var hits int
	releaseServer(t, "v3.0.0", &hits, nil)
	if _, _, err := ResolveAsset(cacheEntry); err != nil {
		t.Fatal(err)
	}
	v, err := LatestVersion(cacheEntry)
	if err != nil || v != "v3.0.0" || hits != 1 {
		t.Errorf("got %q, %v after %d hits, want v3.0.0 from the cache", v, err, hits)
	}
}

func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestDownloadAndExtractResolvesWrappedDir(t *testing.T) {
	archive := buildTarGz(t, map[string]string{
		"hello-py-v1.0.0/manifest.json": `{"id":"hello-py"}`,
		"hello-py-v1.0.0/run.py":        "print('hi')",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()

	dir, cleanup, err := DownloadAndExtract(srv.URL)
	if err != nil {
		t.Fatalf("DownloadAndExtract: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(dir + "/manifest.json"); err != nil {
		t.Errorf("manifest.json not found at resolved dir %q: %v", dir, err)
	}
}

func TestDownloadAndExtractSizeLimit(t *testing.T) {
	huge := make([]byte, maxDownloadBytes+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(huge)
	}))
	defer srv.Close()

	if dir, cleanup, err := DownloadAndExtract(srv.URL); err == nil {
		cleanup()
		os.RemoveAll(dir)
		t.Error("DownloadAndExtract did not reject a payload over the size limit")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct{ a, b string; want int }{
		{"v1.1.0", "v1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "v1.1.0", -1},
		{"v2.0.0-alpha", "v1.9.9", 1},
	}
	for _, tt := range tests {
		got := CompareVersions(tt.a, tt.b)
		sign := func(n int) int {
			switch {
			case n > 0:
				return 1
			case n < 0:
				return -1
			default:
				return 0
			}
		}
		if sign(got) != tt.want {
			t.Errorf("CompareVersions(%q, %q) sign = %d, want %d", tt.a, tt.b, sign(got), tt.want)
		}
	}
}

func TestCheckUpdate(t *testing.T) {
	cleanCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.1.0",
			Assets:  []githubAsset{{Name: "hello-py.tar.gz", BrowserDownloadURL: "http://example.com/x.tar.gz"}},
		})
	}))
	defer srv.Close()
	prev := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prev })

	entry := Entry{ID: "hello-py", GithubRepo: "someone/hello-py", AssetName: "hello-py.tar.gz"}
	available, latest, err := CheckUpdate(entry, "v1.0.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if !available || latest != "v1.1.0" {
		t.Errorf("available=%v latest=%q, want true, v1.1.0", available, latest)
	}

	available, _, err = CheckUpdate(entry, "v1.1.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if available {
		t.Error("available = true when installed version already matches latest")
	}
}
