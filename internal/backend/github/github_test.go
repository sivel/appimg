package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sivel/appimg/internal/backend"
)

// compile-time check that GitHub implements backend.Backend.
var _ backend.Backend = (*GitHub)(nil)

func newTestGitHub(apiBase string) *GitHub {
	return NewWithConfig(Config{
		APIBase:     apiBase,
		HostURL:     "https://github.com",
		TokenEnvVar: "GITHUB_TOKEN",
		ExtraHeaders: map[string]string{
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": apiVer,
		},
	})
}

func TestLatestRelease_Stable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		rel := ghRelease{
			TagName: "v1.2.3",
			Assets:  []ghAsset{{Name: "app-x86_64.AppImage", Size: 100}},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}
}

func TestLatestRelease_SkipsNonAppImageRelease(t *testing.T) {
	// /releases/latest returns a release with no AppImage assets; the backend
	// should fall through to the list search and find the one that has assets.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(ghRelease{TagName: "cli-v1.0.0"})
		case "/repos/owner/repo/releases":
			_ = json.NewEncoder(w).Encode([]ghRelease{
				{TagName: "cli-v1.0.0"},
				{TagName: "desktop-v1.0.0", Assets: []ghAsset{{Name: "app-x86_64.AppImage"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "desktop-v1.0.0" {
		t.Errorf("TagName = %q, want desktop-v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ghRelease{
			{TagName: "cli-v2.0.0", Assets: []ghAsset{{Name: "cli-x86_64.AppImage"}}},
			{TagName: "desktop-v1.0.0", Assets: []ghAsset{{Name: "desktop-x86_64.AppImage"}}},
		})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{VersionPrefix: "desktop-"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "desktop-v1.0.0" {
		t.Errorf("TagName = %q, want desktop-v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_Prerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		releases := []ghRelease{
			{TagName: "v2.0.0-beta", Prerelease: true, Assets: []ghAsset{{Name: "app-x86_64.AppImage"}}},
			{TagName: "v1.0.0", Assets: []ghAsset{{Name: "app-x86_64.AppImage"}}},
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{Prerelease: true})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v2.0.0-beta" {
		t.Errorf("TagName = %q, want v2.0.0-beta", rel.TagName)
	}
}

func TestLatestRelease_Version(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		releases := []ghRelease{
			{TagName: "v1.0.0"},
			{TagName: "v2.0.0"},
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionVPrefix(t *testing.T) {
	// Requesting "1.0.0" should match "v1.0.0" tag.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ghRelease{{TagName: "v1.0.0"}})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	rel, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ghRelease{{TagName: "v2.0.0"}})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{Version: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestAllReleases_Pagination(t *testing.T) {
	page1 := []ghRelease{{TagName: "v1.0.0"}, {TagName: "v1.1.0"}}
	page2 := []ghRelease{{TagName: "v2.0.0"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1", "":
			_ = json.NewEncoder(w).Encode(page1)
		case "2":
			_ = json.NewEncoder(w).Encode(page2)
		default:
			_ = json.NewEncoder(w).Encode([]ghRelease{})
		}
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	releases, err := g.AllReleases(context.Background(), "owner/repo", backend.Options{Prerelease: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 3 {
		t.Errorf("got %d releases, want 3", len(releases))
	}
}

func TestAllReleases_ExcludeDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			_ = json.NewEncoder(w).Encode([]ghRelease{})
			return
		}
		_ = json.NewEncoder(w).Encode([]ghRelease{
			{TagName: "v1.0.0", Draft: true},
			{TagName: "v2.0.0"},
		})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	releases, err := g.AllReleases(context.Background(), "owner/repo", backend.Options{Prerelease: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "v2.0.0" {
		t.Errorf("got %v, want [v2.0.0]", releases)
	}
}

func TestAllReleases_ExcludePrerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			_ = json.NewEncoder(w).Encode([]ghRelease{})
			return
		}
		_ = json.NewEncoder(w).Encode([]ghRelease{
			{TagName: "v2.0.0-beta", Prerelease: true},
			{TagName: "v1.0.0"},
		})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	releases, err := g.AllReleases(context.Background(), "owner/repo", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "v1.0.0" {
		t.Errorf("got %v, want [v1.0.0]", releases)
	}
}

func TestLatestRelease_Prerelease_AllDrafts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ghRelease{
			{TagName: "v1.0.0", Draft: true},
			{TagName: "v2.0.0", Draft: true},
		})
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{Prerelease: true})
	if err == nil {
		t.Fatal("expected error when all releases are drafts, got nil")
	}
}

func TestAPIGet_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestAPIGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestAPIGet_RateLimit_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}

func TestAPIGet_RateLimit_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
}

func TestAPIGet_GithubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token-abc")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token-abc" {
			t.Errorf("Authorization = %q, want Bearer test-token-abc", auth)
		}
		rel := ghRelease{
			TagName: "v1.0.0",
			Assets:  []ghAsset{{Name: "app-x86_64.AppImage", Size: 100}},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()
	g := newTestGitHub(srv.URL)
	_, err := g.LatestRelease(context.Background(), "owner/repo", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectURL(t *testing.T) {
	g := New()
	got := g.ProjectURL("owner/repo")
	want := "https://github.com/owner/repo"
	if got != want {
		t.Errorf("ProjectURL = %q, want %q", got, want)
	}
}

func TestReleaseURL(t *testing.T) {
	g := New()
	got := g.ReleaseURL("owner/repo", "v1.0.0")
	want := "https://github.com/owner/repo/releases/tag/v1.0.0"
	if got != want {
		t.Errorf("ReleaseURL = %q, want %q", got, want)
	}
}
