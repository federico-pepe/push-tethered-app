package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

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
	available, latest, _, err := CheckUpdate(entry, "v1.0.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if !available || latest != "v1.1.0" {
		t.Errorf("available=%v latest=%q, want true, v1.1.0", available, latest)
	}

	available, _, _, err = CheckUpdate(entry, "v1.1.0")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if available {
		t.Error("available = true when installed version already matches latest")
	}
}
