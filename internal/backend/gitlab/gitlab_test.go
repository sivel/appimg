package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sivel/appimg/internal/backend"
)

// compile-time check that GitLab implements backend.Backend.
var _ backend.Backend = (*GitLab)(nil)

func setAPIBase(t *testing.T, url string) {
	t.Helper()
	orig := apiBase
	apiBase = url
	t.Cleanup(func() { apiBase = orig })
}

func makeRelease(tag string, upcoming bool, links ...glLink) glRelease {
	return glRelease{
		TagName:         tag,
		ReleasedAt:      "2024-01-01T00:00:00Z",
		UpcomingRelease: upcoming,
		Assets:          glAssets{Links: links},
	}
}

func appImageLink(name string) glLink {
	return glLink{Name: name, URL: "https://example.com/" + name, LinkType: "package"}
}

func TestLatestRelease_Stable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rels := []glRelease{makeRelease("v1.2.3", false, appImageLink("app-x86_64.AppImage"))}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}
}

func TestLatestRelease_SkipsUpcoming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rels := []glRelease{
			makeRelease("v2.0.0", true, appImageLink("app-x86_64.AppImage")),
			makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage")),
		}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_SkipsNoAppImageAssets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rels := []glRelease{
			makeRelease("v2.0.0", false, glLink{Name: "checksums.txt", URL: "https://example.com/checksums.txt"}),
			makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage")),
		}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rels := []glRelease{
			makeRelease("cli-v2.0.0", false, appImageLink("cli-x86_64.AppImage")),
			makeRelease("desktop-v1.0.0", false, appImageLink("desktop-x86_64.AppImage")),
		}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{VersionPrefix: "desktop-"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "desktop-v1.0.0" {
		t.Errorf("TagName = %q, want desktop-v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_Version(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage")))
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionVPrefix(t *testing.T) {
	// Requesting "1.0.0" should match the "v1.0.0" tag via the v-prefix fallback.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// First attempt (exact "1.0.0") returns 404.
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage")))
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	rel, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
}

func TestLatestRelease_VersionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	_, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{Version: "v9.9.9"})
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestLatestRelease_NoneFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]glRelease{})
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	_, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err == nil {
		t.Fatal("expected error when no releases found, got nil")
	}
}

func TestAllReleases_ExcludesUpcoming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode([]glRelease{})
			return
		}
		rels := []glRelease{
			makeRelease("v2.0.0", false),
			makeRelease("v1.5.0", true), // upcoming — should be excluded
			makeRelease("v1.0.0", false),
		}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	releases, err := g.AllReleases(context.Background(), "ns/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Errorf("got %d releases, want 2 (upcoming excluded)", len(releases))
	}
}

func TestAllReleases_VersionPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode([]glRelease{})
			return
		}
		rels := []glRelease{
			makeRelease("desktop-v2.0.0", false),
			makeRelease("cli-v1.0.0", false),
		}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	releases, err := g.AllReleases(context.Background(), "ns/project", backend.Options{VersionPrefix: "desktop-"})
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "desktop-v2.0.0" {
		t.Errorf("got %v, want [desktop-v2.0.0]", releases)
	}
}

func TestEncodePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"owner/repo", "owner%2Frepo"},
		{"a/b/c", "a%2Fb%2Fc"},
		{"no-slash", "no-slash"},
	}
	for _, tc := range cases {
		got := encodePath(tc.in)
		if got != tc.want {
			t.Errorf("encodePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestToRelease(t *testing.T) {
	r := glRelease{
		TagName:    "v1.0.0",
		ReleasedAt: "2024-06-01T00:00:00Z",
		Assets: glAssets{Links: []glLink{
			{Name: "app-x86_64.AppImage", URL: "https://example.com/app.AppImage", LinkType: "package"},
		}},
	}
	rel := toRelease(r)
	if rel.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want v1.0.0", rel.TagName)
	}
	if rel.PublishedAt != "2024-06-01T00:00:00Z" {
		t.Errorf("PublishedAt = %q, want 2024-06-01T00:00:00Z", rel.PublishedAt)
	}
	if rel.Draft {
		t.Error("Draft = true, want false")
	}
	if len(rel.Assets) != 1 {
		t.Fatalf("len(Assets) = %d, want 1", len(rel.Assets))
	}
	if rel.Assets[0].Name != "app-x86_64.AppImage" {
		t.Errorf("Assets[0].Name = %q, want app-x86_64.AppImage", rel.Assets[0].Name)
	}
	if rel.Assets[0].BrowserDownloadURL != "https://example.com/app.AppImage" {
		t.Errorf("Assets[0].BrowserDownloadURL = %q", rel.Assets[0].BrowserDownloadURL)
	}
}

func TestProjectURL(t *testing.T) {
	g := New()
	got := g.ProjectURL("ns/project")
	want := "https://gitlab.com/ns/project"
	if got != want {
		t.Errorf("ProjectURL = %q, want %q", got, want)
	}
}

func TestReleaseURL(t *testing.T) {
	g := New()
	got := g.ReleaseURL("ns/project", "v1.0.0")
	want := "https://gitlab.com/ns/project/-/releases/v1.0.0"
	if got != want {
		t.Errorf("ReleaseURL = %q, want %q", got, want)
	}
}

func TestAPIGet_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	_, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestAPIGet_RateLimit(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()
			setAPIBase(t, srv.URL)

			g := New()
			_, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
			if err == nil {
				t.Fatalf("expected error for %d, got nil", status)
			}
		})
	}
}

func TestAPIGet_GitLabToken(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token-xyz")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("PRIVATE-TOKEN")
		if tok != "test-token-xyz" {
			t.Errorf("PRIVATE-TOKEN = %q, want test-token-xyz", tok)
		}
		rels := []glRelease{makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage"))}
		_ = json.NewEncoder(w).Encode(rels)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	_, err := g.LatestRelease(context.Background(), "ns/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodedPathInRequest(t *testing.T) {
	// Verify that nested project paths are URL-encoded in the API request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/projects/ns%2Fgroup%2Fproject/releases"
		if r.URL.EscapedPath() != want {
			t.Errorf("path = %q, want %q", r.URL.EscapedPath(), want)
		}
		_ = json.NewEncoder(w).Encode([]glRelease{
			makeRelease("v1.0.0", false, appImageLink("app-x86_64.AppImage")),
		})
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	g := New()
	_, err := g.LatestRelease(context.Background(), "ns/group/project", backend.Options{})
	if err != nil {
		t.Fatal(err)
	}
}
