package cmd

import (
	"strings"
	"testing"

	_ "github.com/sivel/appimg/internal/backend/github"
	_ "github.com/sivel/appimg/internal/backend/gitlab"
)

func TestIsNewer_Semver(t *testing.T) {
	cases := []struct {
		name       string
		currentTag string
		latestTag  string
		want       bool
	}{
		{"newer minor", "v1.0.0", "v1.1.0", true},
		{"newer patch", "v1.0.0", "v1.0.1", true},
		{"newer major", "v1.0.0", "v2.0.0", true},
		{"older", "v1.1.0", "v1.0.0", false},
		{"same", "v1.0.0", "v1.0.0", false},
		{"no v prefix", "1.0.0", "1.0.1", true},
		{"mixed prefix", "1.0.0", "v1.0.1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isNewer(tc.currentTag, "", tc.latestTag, "", false)
			if got != tc.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tc.currentTag, tc.latestTag, got, tc.want)
			}
		})
	}
}

func TestIsNewer_TimestampFallback(t *testing.T) {
	// Non-semver tags fall back to published_at comparison.
	cases := []struct {
		name             string
		currentPublished string
		latestPublished  string
		want             bool
	}{
		{"newer timestamp", "2024-01-01T00:00:00Z", "2024-06-01T00:00:00Z", true},
		{"older timestamp", "2024-06-01T00:00:00Z", "2024-01-01T00:00:00Z", false},
		{"same timestamp", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Use non-parseable tags to force timestamp fallback.
			got := isNewer("not-semver-old", tc.currentPublished, "not-semver-new", tc.latestPublished, false)
			if got != tc.want {
				t.Errorf("isNewer(timestamps %q -> %q) = %v, want %v",
					tc.currentPublished, tc.latestPublished, got, tc.want)
			}
		})
	}
}

func TestIsNewer_FourPartVersion(t *testing.T) {
	// Four-part versions (e.g. BambuStudio's 02.05.00.67) coerce to the same
	// three-part semver, so published_at is used as the tiebreaker.
	cases := []struct {
		name             string
		currentTag       string
		latestTag        string
		currentPublished string
		latestPublished  string
		want             bool
	}{
		{
			name:             "newer fourth part",
			currentTag:       "02.05.00.67",
			latestTag:        "02.05.00.68",
			currentPublished: "2024-01-01T00:00:00Z",
			latestPublished:  "2024-06-01T00:00:00Z",
			want:             true,
		},
		{
			name:             "same fourth part same timestamp",
			currentTag:       "02.05.00.67",
			latestTag:        "02.05.00.67",
			currentPublished: "2024-01-01T00:00:00Z",
			latestPublished:  "2024-01-01T00:00:00Z",
			want:             false,
		},
		{
			name:             "older fourth part",
			currentTag:       "02.05.00.68",
			latestTag:        "02.05.00.67",
			currentPublished: "2024-06-01T00:00:00Z",
			latestPublished:  "2024-01-01T00:00:00Z",
			want:             false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isNewer(tc.currentTag, tc.currentPublished, tc.latestTag, tc.latestPublished, false)
			if got != tc.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tc.currentTag, tc.latestTag, got, tc.want)
			}
		})
	}
}

func TestIsNewer_SameTagNotNewer(t *testing.T) {
	// Same tag, non-rolling: short-circuits to false without comparing timestamps.
	got := isNewer("v1.0.0", "2024-01-01T00:00:00Z", "v1.0.0", "2024-06-01T00:00:00Z", false)
	if got {
		t.Error("isNewer with same tag and rolling=false should return false")
	}
}

func TestIsNewer_Rolling(t *testing.T) {
	cases := []struct {
		name             string
		currentPublished string
		latestPublished  string
		want             bool
	}{
		{"newer asset behind same tag", "2024-01-01T00:00:00Z", "2024-06-01T00:00:00Z", true},
		{"same published_at", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", false},
		{"older published_at", "2024-06-01T00:00:00Z", "2024-01-01T00:00:00Z", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Rolling releases use the same tag; only published_at differs.
			got := isNewer("nightly", tc.currentPublished, "nightly", tc.latestPublished, true)
			if got != tc.want {
				t.Errorf("isNewer rolling (timestamps %q -> %q) = %v, want %v",
					tc.currentPublished, tc.latestPublished, got, tc.want)
			}
		})
	}
}

func TestCutPrefix(t *testing.T) {
	cases := []struct {
		s      string
		prefix string
		want   string
		ok     bool
	}{
		{"catalog:Bitwarden", "catalog:", "Bitwarden", true},
		{"catalog:", "catalog:", "", true},
		{"github:owner/repo", "catalog:", "github:owner/repo", false},
		{"", "catalog:", "", false},
	}
	for _, tc := range cases {
		got, ok := strings.CutPrefix(tc.s, tc.prefix)
		if ok != tc.ok || got != tc.want {
			t.Errorf("strings.CutPrefix(%q, %q) = %q, %v; want %q, %v", tc.s, tc.prefix, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBackendFromSource_GitHub(t *testing.T) {
	b, project, err := backendFromSource("github:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", project)
	}
}

func TestBackendFromSource_GitLab(t *testing.T) {
	b, project, err := backendFromSource("gitlab:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	if project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", project)
	}
}

func TestBackendFromSource_Local(t *testing.T) {
	_, _, err := backendFromSource("local:MyApp")
	if err == nil {
		t.Fatal("expected error for local source, got nil")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("error = %q, want message about local installs", err)
	}
}

func TestBackendFromSource_Unknown(t *testing.T) {
	_, _, err := backendFromSource("unknown:something")
	if err == nil {
		t.Fatal("expected error for unknown source format, got nil")
	}
}
