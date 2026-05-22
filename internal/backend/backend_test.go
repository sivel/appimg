package backend

import (
	"testing"

	"github.com/sivel/appimg/internal/platform"
)

func TestRegisterAndLookup(t *testing.T) {
	Register("testpfx", func() Backend { return nil })
	t.Cleanup(func() { delete(registry, "testpfx") })

	b, project, appName, ok := Lookup("testpfx:owner/repo")
	if !ok {
		t.Fatal("Lookup returned false for registered prefix")
	}
	if project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", project)
	}
	if appName != "repo" {
		t.Errorf("appName = %q, want repo", appName)
	}
	_ = b
}

func TestLookup_UnknownPrefix(t *testing.T) {
	_, _, _, ok := Lookup("unknown:owner/repo")
	if ok {
		t.Error("Lookup returned true for unknown prefix")
	}
}

func TestLookup_NestedProject(t *testing.T) {
	Register("testpfx2", func() Backend { return nil })
	t.Cleanup(func() { delete(registry, "testpfx2") })

	_, project, appName, ok := Lookup("testpfx2:ns/group/project")
	if !ok {
		t.Fatal("Lookup returned false")
	}
	if project != "ns/group/project" {
		t.Errorf("project = %q, want ns/group/project", project)
	}
	if appName != "project" {
		t.Errorf("appName = %q, want project", appName)
	}
}

func TestSelectAsset_Pattern(t *testing.T) {
	assets := []Asset{
		{Name: "app-x86_64.AppImage"},
		{Name: "app-aarch64.AppImage"},
		{Name: "app-linux-x86_64.AppImage"},
	}
	opts := Options{AssetPattern: "*aarch64*"}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	got, err := selectAsset(assets, opts, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-aarch64.AppImage" {
		t.Errorf("got %q, want app-aarch64.AppImage", got.Name)
	}
}

func TestSelectAsset_PatternNoMatch(t *testing.T) {
	assets := []Asset{
		{Name: "app-x86_64.AppImage"},
	}
	opts := Options{AssetPattern: "*arm*"}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	_, err := selectAsset(assets, opts, "x86_64", osInfo)
	if err == nil {
		t.Fatal("expected error for non-matching pattern, got nil")
	}
}

func TestSelectAsset_NoAppImages(t *testing.T) {
	assets := []Asset{
		{Name: "app.tar.gz"},
		{Name: "app.deb"},
		{Name: "checksums.sha256"},
	}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	_, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err == nil {
		t.Fatal("expected error when no AppImage assets, got nil")
	}
}

func TestSelectAsset_ArchScore(t *testing.T) {
	assets := []Asset{
		{Name: "app.AppImage"},
		{Name: "app-x86_64.AppImage"},
	}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-x86_64.AppImage" {
		t.Errorf("got %q, want app-x86_64.AppImage", got.Name)
	}
}

func TestSelectAsset_PenaltyKeywords(t *testing.T) {
	cases := []struct {
		name     string
		assets   []Asset
		wantName string
	}{
		{
			name: "lite penalized",
			assets: []Asset{
				{Name: "app-lite-x86_64.AppImage"},
				{Name: "app-x86_64.AppImage"},
			},
			wantName: "app-x86_64.AppImage",
		},
		{
			name: "minimal penalized",
			assets: []Asset{
				{Name: "app-minimal-x86_64.AppImage"},
				{Name: "app-x86_64.AppImage"},
			},
			wantName: "app-x86_64.AppImage",
		},
		{
			name: "debug penalized",
			assets: []Asset{
				{Name: "app-debug-x86_64.AppImage"},
				{Name: "app-x86_64.AppImage"},
			},
			wantName: "app-x86_64.AppImage",
		},
		{
			name: "dev penalized",
			assets: []Asset{
				{Name: "app-dev-x86_64.AppImage"},
				{Name: "app-x86_64.AppImage"},
			},
			wantName: "app-x86_64.AppImage",
		},
	}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectAsset(tc.assets, Options{}, "x86_64", osInfo)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.wantName {
				t.Errorf("got %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}

func TestSelectAsset_TiebreakShorterName(t *testing.T) {
	assets := []Asset{
		{Name: "MyApp-1.0.0-x86_64.AppImage"},
		{Name: "MyApp-x86_64.AppImage"},
	}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "MyApp-x86_64.AppImage" {
		t.Errorf("got %q, want MyApp-x86_64.AppImage", got.Name)
	}
}

func TestSelectAsset_CaseInsensitive(t *testing.T) {
	assets := []Asset{
		{Name: "app-x86_64.APPIMAGE"},
	}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-x86_64.APPIMAGE" {
		t.Errorf("got %q, want app-x86_64.APPIMAGE", got.Name)
	}
}

func TestSelectAsset_DistroScore(t *testing.T) {
	// Simulates BambuStudio on Ubuntu 25.10: ubuntu-24.04 should win over
	// ubuntu-22.04 (closer version) and the fedora asset (wrong distro).
	assets := []Asset{
		{Name: "Bambu_Studio_ubuntu-22.04_PR-9540.AppImage"},
		{Name: "Bambu_Studio_ubuntu-24.04_PR-9540.AppImage"},
		{Name: "Bambu_Studio_linux_fedora-v02.05.00.66.AppImage"},
	}
	osInfo := platform.OSInfo{ID: "ubuntu", IDLike: []string{}, Version: "25.10"}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Bambu_Studio_ubuntu-24.04_PR-9540.AppImage" {
		t.Errorf("got %q, want ubuntu-24.04 asset", got.Name)
	}
}

func TestSelectAsset_DistroExactVersion(t *testing.T) {
	assets := []Asset{
		{Name: "app-ubuntu-22.04.AppImage"},
		{Name: "app-ubuntu-24.04.AppImage"},
	}
	// Exact match for 24.04 should win.
	osInfo := platform.OSInfo{ID: "ubuntu", IDLike: []string{}, Version: "24.04"}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-ubuntu-24.04.AppImage" {
		t.Errorf("got %q, want app-ubuntu-24.04.AppImage", got.Name)
	}
}

func TestDistroVersionFromAsset(t *testing.T) {
	cases := []struct {
		asset  string
		distro string
		want   string
	}{
		{"bambu_studio_ubuntu-22.04_pr.appimage", "ubuntu", "22.04"},
		{"bambu_studio_ubuntu-24.04_pr.appimage", "ubuntu", "24.04"},
		// App version after distro name has >1 dot -> rejected
		{"bambu_studio_linux_fedora-v02.05.00.66.appimage", "fedora", ""},
		{"app-fedora-38.appimage", "fedora", "38"},
		{"app-ubuntu.appimage", "ubuntu", ""},
	}
	for _, tc := range cases {
		got := distroVersionFromAsset(tc.asset, tc.distro)
		if got != tc.want {
			t.Errorf("distroVersionFromAsset(%q, %q) = %q, want %q", tc.asset, tc.distro, got, tc.want)
		}
	}
}

func TestVersionProximityScore(t *testing.T) {
	cases := []struct {
		asset string
		sys   string
		want  int
	}{
		{"25.10", "25.10", 5}, // exact
		{"24.04", "25.10", 2}, // one step back
		{"22.04", "25.10", 1}, // capped at min 1
		{"26.04", "25.10", 1}, // newer than current
		{"41", "41", 5},       // fedora exact
		{"40", "41", 2},       // fedora one back (diff=100 -> 3-1=2)
	}
	for _, tc := range cases {
		got := versionProximityScore(tc.asset, tc.sys)
		if got != tc.want {
			t.Errorf("versionProximityScore(%q, %q) = %d, want %d", tc.asset, tc.sys, got, tc.want)
		}
	}
}

func TestSelectAsset_IDLikeScore(t *testing.T) {
	// Simulates Pop!_OS 24.04 (ID=pop_os, ID_LIKE=ubuntu debian): the ubuntu-24.04
	// asset should win via IDLike version proximity over ubuntu-22.04 and debian-12.
	assets := []Asset{
		{Name: "app-ubuntu22.04.AppImage"},
		{Name: "app-ubuntu24.04.AppImage"},
		{Name: "app-debian12.AppImage"},
		{Name: "app.AppImage"},
	}
	osInfo := platform.OSInfo{ID: "pop_os", IDLike: []string{"ubuntu", "debian"}, Version: "24.04"}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-ubuntu24.04.AppImage" {
		t.Errorf("got %q, want app-ubuntu24.04.AppImage", got.Name)
	}
}

func TestSelectAsset_IDBeatsIDLike(t *testing.T) {
	// A direct ID match (baseScore=5) should beat an IDLike match (baseScore=3).
	assets := []Asset{
		{Name: "app-debian.AppImage"},
		{Name: "app-ubuntu.AppImage"},
	}
	osInfo := platform.OSInfo{ID: "ubuntu", IDLike: []string{"debian"}, Version: ""}
	got, err := selectAsset(assets, Options{}, "x86_64", osInfo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-ubuntu.AppImage" {
		t.Errorf("got %q, want app-ubuntu.AppImage", got.Name)
	}
}

func TestSelectAsset_InvalidPattern(t *testing.T) {
	assets := []Asset{{Name: "app-x86_64.AppImage"}}
	osInfo := platform.OSInfo{ID: "", IDLike: []string{}, Version: ""}
	_, err := selectAsset(assets, Options{AssetPattern: "["}, "x86_64", osInfo)
	if err == nil {
		t.Fatal("expected error for invalid pattern, got nil")
	}
}

func TestIsRollingTag(t *testing.T) {
	rolling := []string{"nightly", "latest", "continuous", "rolling", "edge", "dev", "unstable", "preview",
		"NIGHTLY", "Latest"}
	for _, tag := range rolling {
		if !IsRollingTag(tag) {
			t.Errorf("IsRollingTag(%q) = false, want true", tag)
		}
	}
	notRolling := []string{"v1.0.0", "1.2.3", "release", "stable"}
	for _, tag := range notRolling {
		if IsRollingTag(tag) {
			t.Errorf("IsRollingTag(%q) = true, want false", tag)
		}
	}
}

func TestHasAppImageAssets(t *testing.T) {
	cases := []struct {
		name   string
		assets []Asset
		want   bool
	}{
		{"empty", []Asset{}, false},
		{"no appimage", []Asset{{Name: "app.tar.gz"}, {Name: "app.deb"}}, false},
		{"has appimage", []Asset{{Name: "app.AppImage"}}, true},
		{"case insensitive", []Asset{{Name: "app.APPIMAGE"}}, true},
		{"mixed", []Asset{{Name: "app.tar.gz"}, {Name: "app-x86_64.AppImage"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HasAppImageAssets(tc.assets)
			if got != tc.want {
				t.Errorf("HasAppImageAssets = %v, want %v", got, tc.want)
			}
		})
	}
}
