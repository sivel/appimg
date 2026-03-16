package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/sivel/appimg/internal/cache"
)

func overrideXDGCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	xdg.Reload()
	t.Cleanup(xdg.Reload)
}

func writeFeedFile(t *testing.T, content string) {
	t.Helper()
	path, err := cache.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	overrideXDGCacheHome(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing feed file, got nil")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	overrideXDGCacheHome(t)
	writeFeedFile(t, "not json at all")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoad_Valid(t *testing.T) {
	overrideXDGCacheHome(t)
	writeFeedFile(t, sampleFeed)
	feed, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(feed.Items) != 3 {
		t.Errorf("got %d items, want 3", len(feed.Items))
	}
}

const sampleFeed = `{
	"items": [
		{
			"name": "Bitwarden",
			"description": "A password manager",
			"categories": ["Utility"],
			"links": [
				{"type": "GitHub", "url": "bitwarden/clients"},
				{"type": "Download", "url": "https://github.com/bitwarden/clients/releases"}
			]
		},
		{
			"name": "FreeCAD",
			"description": null,
			"categories": ["Graphics"],
			"links": [
				{"type": "GitHub", "url": "FreeCAD/FreeCAD"}
			]
		},
		{
			"name": "NoGitHub",
			"description": "An app with no GitHub link",
			"links": [
				{"type": "Download", "url": "https://example.com"}
			]
		}
	]
}`

func parseFeed(t *testing.T) *Feed {
	t.Helper()
	var feed Feed
	if err := json.Unmarshal([]byte(sampleFeed), &feed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return &feed
}

func TestFeedParsing(t *testing.T) {
	feed := parseFeed(t)
	if len(feed.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(feed.Items))
	}
	bw := feed.Items[0]
	if bw.Name != "Bitwarden" {
		t.Errorf("Name = %q, want Bitwarden", bw.Name)
	}
	if bw.Description != "A password manager" {
		t.Errorf("Description = %q", bw.Description)
	}
	if len(bw.Categories) != 1 || bw.Categories[0] != "Utility" {
		t.Errorf("Categories = %v, want [Utility]", bw.Categories)
	}
}

func TestFeedParsing_NullDescription(t *testing.T) {
	feed := parseFeed(t)
	// FreeCAD has null description — should unmarshal as empty string, not error.
	fc := feed.Items[1]
	if fc.Name != "FreeCAD" {
		t.Fatalf("expected FreeCAD, got %q", fc.Name)
	}
	if fc.Description != "" {
		t.Errorf("expected empty description for null JSON field, got %q", fc.Description)
	}
}

func TestGitHubRepo_Found(t *testing.T) {
	feed := parseFeed(t)
	entry := feed.Items[0]
	repo, ok := entry.GitHubRepo()
	if !ok {
		t.Fatal("GitHubRepo() ok = false, want true")
	}
	if repo != "bitwarden/clients" {
		t.Errorf("GitHubRepo() = %q, want bitwarden/clients", repo)
	}
}

func TestGitHubRepo_NotFound(t *testing.T) {
	feed := parseFeed(t)
	entry := feed.Items[2] // NoGitHub
	_, ok := entry.GitHubRepo()
	if ok {
		t.Fatal("GitHubRepo() ok = true, want false")
	}
}

func TestGitHubRepo_CaseInsensitive(t *testing.T) {
	entry := Entry{
		Links: []Link{{Type: "github", URL: "owner/repo"}},
	}
	repo, ok := entry.GitHubRepo()
	if !ok || repo != "owner/repo" {
		t.Errorf("GitHubRepo() = %q, %v; want owner/repo, true", repo, ok)
	}
}

func TestFind(t *testing.T) {
	feed := parseFeed(t)

	e, ok := Find(feed, "bitwarden")
	if !ok {
		t.Fatal("Find(bitwarden) ok = false, want true")
	}
	if e.Name != "Bitwarden" {
		t.Errorf("Name = %q, want Bitwarden", e.Name)
	}
}

func TestFind_NotFound(t *testing.T) {
	feed := parseFeed(t)
	_, ok := Find(feed, "NotAnApp")
	if ok {
		t.Fatal("Find(NotAnApp) ok = true, want false")
	}
}

func TestFilter_EmptyPattern(t *testing.T) {
	feed := parseFeed(t)
	results, err := Filter(feed, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(feed.Items) {
		t.Errorf("got %d results, want %d", len(results), len(feed.Items))
	}
}

func TestFilter_NameMatch(t *testing.T) {
	feed := parseFeed(t)
	results, err := Filter(feed, "Free")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "FreeCAD" {
		t.Errorf("got %v, want [FreeCAD]", results)
	}
}

func TestFilter_DescriptionMatch(t *testing.T) {
	feed := parseFeed(t)
	results, err := Filter(feed, "password")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "Bitwarden" {
		t.Errorf("got %v, want [Bitwarden]", results)
	}
}

func TestFilter_InvalidPattern(t *testing.T) {
	feed := parseFeed(t)
	_, err := Filter(feed, "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}
