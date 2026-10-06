package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompare(t *testing.T) {
	// Ascending order: every earlier entry is older than every later one.
	order := []string{
		"v0.1.0-alpha", "v0.1.0-beta", "v0.1.0-rc.1", "v0.1.0-rc.2",
		"v0.1.0-rc.10", "v0.1.0", "v0.1.1-alpha", "v0.1.1", "v0.2.0-alpha", "v1.0.0",
	}
	for i := range order {
		for j := range order {
			want := sign(i - j)
			if got := Compare(order[i], order[j]); got != want {
				t.Errorf("Compare(%q, %q) = %d, want %d", order[i], order[j], got, want)
			}
		}
	}
	if Compare("0.1.0", "v0.1.0") != 0 {
		t.Error("leading v should not matter")
	}
	if Compare("dev", "v0.1.0") >= 0 {
		t.Error("unparseable should sort below a real version")
	}
}

func serve(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old })
}

const releases = `[
 {"tag_name":"v0.1.5-alpha","html_url":"https://x/alpha"},
 {"tag_name":"v0.2.0-beta","html_url":"https://x/beta"},
 {"tag_name":"v0.3.0","html_url":"https://x/draft","draft":true},
 {"tag_name":"v0.1.9","html_url":"https://x/stable"},
 {"tag_name":"not-a-version","html_url":"https://x/junk"}
]`

func TestCheckPrereleaseSeesBeta(t *testing.T) {
	serve(t, releases)
	got, err := Check(context.Background(), "v0.1.5-alpha")
	if err != nil || got == nil || got.Version != "v0.2.0-beta" || got.URL != "https://x/beta" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCheckPicksHighestNotFirst(t *testing.T) {
	serve(t, `[{"tag_name":"v0.1.6-alpha","html_url":"a"},{"tag_name":"v0.1.7-alpha","html_url":"b"},{"tag_name":"v0.1.5-alpha","html_url":"c"}]`)
	got, _ := Check(context.Background(), "v0.1.0-alpha")
	if got == nil || got.Version != "v0.1.7-alpha" {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckStableIgnoresPrerelease(t *testing.T) {
	serve(t, releases)
	got, err := Check(context.Background(), "v0.1.0")
	if err != nil || got == nil || got.Version != "v0.1.9" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCheckUpToDate(t *testing.T) {
	serve(t, releases)
	if got, err := Check(context.Background(), "v0.2.0-beta"); got != nil || err != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCheckDevSkipsNetwork(t *testing.T) {
	old := githubAPIBase
	githubAPIBase = "http://127.0.0.1:1" // would fail if contacted
	defer func() { githubAPIBase = old }()
	if got, err := Check(context.Background(), "dev"); got != nil || err != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCheckHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()
	if _, err := Check(context.Background(), "v0.1.0"); err == nil {
		t.Fatal("want error on 403")
	}
}
