package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

// testPNG is the 8-byte PNG signature — enough for detectIconFormat.
var testPNG = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// testSVG is a minimal SVG document.
var testSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"/>`)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Bitwarden", "bitwarden"},
		{"Free CAD", "free-cad"},
		{"My App Name", "my-app-name"},
		{"already-lower", "already-lower"},
	}
	for _, tc := range cases {
		got := NormalizeName(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectIconFormat(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"PNG magic", []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, ".png"},
		{"SVG tag", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"), ".svg"},
		{"XML prolog SVG", []byte("<?xml version=\"1.0\"?><svg/>"), ".svg"},
		{"XPM fallback", []byte("/* XPM */"), ".xpm"},
		{"empty", []byte{}, ".xpm"},
		{"short non-PNG", []byte{0x89, 'P'}, ".xpm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectIconFormat(tc.data)
			if got != tc.want {
				t.Errorf("detectIconFormat(...) = %q, want %q", got, tc.want)
			}
		})
	}
}

// checkMksquashfs skips the test if mksquashfs is not installed.
func checkMksquashfs(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		t.Skip("mksquashfs not found; install squashfs-tools to run desktop tests")
	}
}

// findELF returns a path to a small ELF binary to use as the AppImage runtime,
// skipping the test if none is found.
func findELF(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"/usr/bin/true", "/bin/true"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no suitable ELF binary found for AppImage assembly")
	return ""
}

// makeAppImage builds a minimal Type 2 AppImage by concatenating a real ELF
// binary with a squashfs filesystem packed from rootDir. Any extra arguments
// are forwarded to mksquashfs (e.g. "-comp", "zstd").
func makeAppImage(t *testing.T, rootDir string, mksquashfsArgs ...string) string {
	t.Helper()
	elfPath := findELF(t)

	tmp := t.TempDir()
	sfsPath := filepath.Join(tmp, "fs.sfs")

	args := append([]string{rootDir, sfsPath, "-noappend", "-no-progress"}, mksquashfsArgs...)
	out, err := exec.Command("mksquashfs", args...).CombinedOutput()
	if err != nil {
		// Older squashfs-tools may not support zstd; skip rather than fail.
		if strings.Contains(strings.ToLower(string(out)), "compressor") {
			t.Skipf("mksquashfs does not support requested compression: %s", out)
		}
		t.Fatalf("mksquashfs: %v\n%s", err, out)
	}

	elf, err := os.ReadFile(elfPath)
	if err != nil {
		t.Fatal(err)
	}
	sfs, err := os.ReadFile(sfsPath)
	if err != nil {
		t.Fatal(err)
	}

	appPath := filepath.Join(tmp, "test.AppImage")
	if err := os.WriteFile(appPath, append(elf, sfs...), 0755); err != nil {
		t.Fatal(err)
	}
	return appPath
}

// overrideXDGDataHome redirects XDG_DATA_HOME to a temp directory for the
// duration of the test, preventing writes to the real user data directory.
func overrideXDGDataHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	xdg.Reload()
	t.Cleanup(xdg.Reload)
}

func TestInstall_CreatesDesktopFile(t *testing.T) {
	overrideXDGDataHome(t)

	// Use a non-AppImage file; icon extraction will fail gracefully.
	appPath := filepath.Join(t.TempDir(), "TestApp.AppImage")
	if err := os.WriteFile(appPath, []byte("not an appimage"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := Install("TestApp", appPath, "", []string{"Utility"}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	desktopPath := desktopFilePath("TestApp")
	data, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("desktop file not written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Name=TestApp") {
		t.Errorf("desktop file missing Name=TestApp:\n%s", content)
	}
	if !strings.Contains(content, "Exec="+appPath) {
		t.Errorf("desktop file missing Exec path:\n%s", content)
	}
	if !strings.Contains(content, "Categories=Utility;") {
		t.Errorf("desktop file missing Categories:\n%s", content)
	}
}

func TestInstall_Remove_WithIcon(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".DirIcon"), testPNG, 0644); err != nil {
		t.Fatal(err)
	}
	appPath := makeAppImage(t, root)

	if err := Install("IconApp", appPath, "", nil); err != nil {
		t.Fatalf("Install: %v", err)
	}

	files := IntegrationFiles("IconApp")
	if len(files) == 0 {
		t.Fatal("IntegrationFiles returned nothing after Install")
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("integration file missing: %s", f)
		}
	}

	if err := Remove("IconApp"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, f := range files {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("integration file still exists after Remove: %s", f)
		}
	}
}

func TestIntegrationFiles_Empty(t *testing.T) {
	overrideXDGDataHome(t)
	files := IntegrationFiles("NoSuchApp")
	if len(files) != 0 {
		t.Errorf("expected no files for uninstalled app, got %v", files)
	}
}

func TestRemoveIfExists_NonExistent(t *testing.T) {
	if err := removeIfExists(filepath.Join(t.TempDir(), "ghost.txt")); err != nil {
		t.Errorf("removeIfExists on missing file should not error: %v", err)
	}
}

func TestRemoveIfExists_Existing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := removeIfExists(path); err != nil {
		t.Fatalf("removeIfExists: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after removeIfExists")
	}
}

func TestExtractIcon_DirectPNG(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".DirIcon"), testPNG, 0644); err != nil {
		t.Fatal(err)
	}

	appPath := makeAppImage(t, root)
	iconPath, err := extractIcon("TestApp", appPath)
	if err != nil {
		t.Fatalf("extractIcon: %v", err)
	}
	if !strings.HasSuffix(iconPath, ".png") {
		t.Errorf("icon path = %q, want .png suffix", iconPath)
	}
	data, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 || data[0] != 0x89 || string(data[1:4]) != "PNG" {
		t.Error("extracted file does not start with PNG magic bytes")
	}
}

func TestExtractIcon_DirectSVG(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".DirIcon"), testSVG, 0644); err != nil {
		t.Fatal(err)
	}

	appPath := makeAppImage(t, root)
	iconPath, err := extractIcon("TestApp", appPath)
	if err != nil {
		t.Fatalf("extractIcon: %v", err)
	}
	if !strings.HasSuffix(iconPath, ".svg") {
		t.Errorf("icon path = %q, want .svg suffix", iconPath)
	}
	data, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "<svg") {
		t.Error("extracted file does not start with <svg")
	}
}

// TestExtractIcon_SymlinkToSVG covers the case where .DirIcon is a symlink
// (the most common real-world pattern, as seen with FreeCAD).
func TestExtractIcon_SymlinkToSVG(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.svg"), testSVG, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("app.svg", filepath.Join(root, ".DirIcon")); err != nil {
		t.Fatal(err)
	}

	appPath := makeAppImage(t, root)
	iconPath, err := extractIcon("TestApp", appPath)
	if err != nil {
		t.Fatalf("extractIcon: %v", err)
	}
	if !strings.HasSuffix(iconPath, ".svg") {
		t.Errorf("icon path = %q, want .svg suffix", iconPath)
	}
}

// TestExtractIcon_ZstdSymlink reproduces the exact FreeCAD scenario: zstd
// compression, squashfs after an ELF runtime (testing ELF offset detection),
// and .DirIcon as a symlink.
func TestPatchDesktopFile(t *testing.T) {
	input := `[Desktop Entry]
Name=Bitwarden
Exec=bitwarden --no-sandbox %U
TryExec=bitwarden
Icon=bitwarden
Type=Application
Categories=Utility;

[Desktop Action NewWindow]
Name=New Window
Exec=bitwarden --no-sandbox --new-window
`
	got := string(patchDesktopFile([]byte(input), "/opt/bitwarden.AppImage --no-sandbox", "/icons/bitwarden.png"))

	cases := []struct {
		desc    string
		want    string
		present bool
	}{
		{"Exec patched with field code", "Exec=/opt/bitwarden.AppImage --no-sandbox %U", true},
		{"no double --no-sandbox", "Exec=/opt/bitwarden.AppImage --no-sandbox --no-sandbox", false},
		{"Icon patched", "Icon=/icons/bitwarden.png", true},
		{"TryExec removed", "TryExec=", false},
		{"Name preserved", "Name=Bitwarden", true},
		{"Categories preserved", "Categories=Utility;", true},
		{"Action Exec patched", "Exec=/opt/bitwarden.AppImage --no-sandbox --new-window", true},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if strings.Contains(got, tc.want) != tc.present {
				if tc.present {
					t.Errorf("expected %q in output:\n%s", tc.want, got)
				} else {
					t.Errorf("unexpected %q in output:\n%s", tc.want, got)
				}
			}
		})
	}
}

func TestInstall_UsesEmbeddedDesktopFile(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	embeddedDesktop := `[Desktop Entry]
Name=MyApp
Exec=myapp %F
Icon=myapp
Type=Application
Categories=Graphics;
`
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "myapp.desktop"), []byte(embeddedDesktop), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".DirIcon"), testPNG, 0644); err != nil {
		t.Fatal(err)
	}
	appPath := makeAppImage(t, root)

	if err := Install("MyApp", appPath, "", []string{"Utility"}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	desktopPath := desktopFilePath("MyApp")
	data, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("desktop file not written: %v", err)
	}
	content := string(data)

	// Author categories should win, not the caller-supplied "Utility".
	if !strings.Contains(content, "Categories=Graphics;") {
		t.Errorf("expected embedded Categories=Graphics;, got:\n%s", content)
	}
	// Exec should point to the installed AppImage.
	if !strings.Contains(content, "Exec="+appPath+" %F") {
		t.Errorf("expected Exec=%s %%F, got:\n%s", appPath, content)
	}
	// Icon should point to the extracted icon, not the original name.
	if strings.Contains(content, "Icon=myapp") {
		t.Errorf("Icon should be patched away from 'myapp', got:\n%s", content)
	}
}

func TestExtractIcon_ZstdSymlink(t *testing.T) {
	checkMksquashfs(t)
	overrideXDGDataHome(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.svg"), testSVG, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("app.svg", filepath.Join(root, ".DirIcon")); err != nil {
		t.Fatal(err)
	}

	appPath := makeAppImage(t, root, "-comp", "zstd")
	iconPath, err := extractIcon("TestApp", appPath)
	if err != nil {
		t.Fatalf("extractIcon: %v", err)
	}
	if !strings.HasSuffix(iconPath, ".svg") {
		t.Errorf("icon path = %q, want .svg suffix", iconPath)
	}
}
