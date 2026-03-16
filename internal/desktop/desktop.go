// Package desktop manages .desktop file and icon integration for installed AppImages.
package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/CalebQ42/squashfs"
	"github.com/adrg/xdg"

	"github.com/sivel/appimg/internal/appimage"
)

const desktopTemplate = `[Desktop Entry]
Name=%s
Exec=%s
Icon=%s
Type=Application
Categories=%s
`

// NormalizeName converts an app name to the normalized form used for file naming:
// lowercased with spaces replaced by hyphens.
func NormalizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// AppInfo holds metadata extracted from an AppImage's embedded .desktop file.
type AppInfo struct {
	Name    string
	Version string
}

// ExtractAppInfo reads Name= and X-AppImage-Version= from the embedded
// .desktop file. Falls back to the filename (without .AppImage suffix) for
// Name if the embedded file is absent or has no Name field.
func ExtractAppInfo(appImagePath string) AppInfo {
	info := AppInfo{Name: strings.TrimSuffix(filepath.Base(appImagePath), ".AppImage")}

	data, err := extractDesktopFile(appImagePath)
	if err != nil || len(data) == 0 {
		return info
	}

	inEntry := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		s := string(line)
		if strings.HasPrefix(s, "[") {
			inEntry = strings.TrimSpace(s) == "[Desktop Entry]"
		}
		if !inEntry {
			continue
		}
		if after, ok := strings.CutPrefix(s, "Name="); ok && info.Name != after {
			info.Name = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(s, "X-AppImage-Version="); ok {
			info.Version = strings.TrimSpace(after)
		}
	}
	return info
}

// Install writes a .desktop file and extracts the icon for the given AppImage.
// It prefers the .desktop file embedded in the AppImage by the author, patching
// only Exec= and Icon= to reflect the installed paths. categories is used only
// when falling back to the built-in template (no embedded .desktop found).
// When appimgExe is non-empty, the Exec line uses "appimgExe exec appName" so
// that launch errors are surfaced via an error dialog; sandbox detection is
// deferred to exec time.
func Install(appName, appImagePath, appimgExe string, categories []string) error {
	iconPath, err := extractIcon(appName, appImagePath)
	if err != nil {
		// Non-fatal: log and continue without an icon.
		fmt.Fprintf(os.Stderr, "warning: could not extract icon for %s: %v\n", appName, err)
		iconPath = ""
	}

	embedded, _ := extractDesktopFile(appImagePath)

	var execCmd string
	if appimgExe != "" {
		// Delegate launch (and sandbox handling) to "appimg exec".
		execCmd = appimgExe + " exec " + appName
	} else {
		execCmd = appImagePath
		switch {
		case embeddedHasNoSandbox(embedded):
			// Author already requires --no-sandbox; honour it without interrogating the system.
			execCmd = appImagePath + " --no-sandbox"
		case appimage.RequiresNoSandbox(appImagePath):
			execCmd = appImagePath + " --no-sandbox"
			fmt.Fprintf(os.Stderr, "note: added --no-sandbox to desktop entry for %s (user namespaces unavailable)\n", appName)
		}
	}

	desktopPath := desktopFilePath(appName)
	if err := os.MkdirAll(filepath.Dir(desktopPath), 0755); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	var content []byte
	if len(embedded) > 0 {
		content = patchDesktopFile(embedded, execCmd, iconPath)
	} else {
		cats := strings.Join(categories, ";")
		if len(categories) > 0 {
			cats += ";"
		}
		content = fmt.Appendf(nil, desktopTemplate, appName, execCmd, iconPath, cats)
	}
	return os.WriteFile(desktopPath, content, 0644)
}

// extractDesktopFile finds and reads the first .desktop file at the root of
// the AppImage's squashfs. Returns nil if none is found.
func extractDesktopFile(appImagePath string) ([]byte, error) {
	f, err := os.Open(appImagePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	offset, err := appimage.FindSquashFSOffset(f)
	if err != nil {
		return nil, err
	}

	rdr, err := squashfs.NewReaderAtOffset(f, offset)
	if err != nil {
		return nil, err
	}

	entries, err := rdr.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".desktop") {
			slog.Debug("found embedded desktop file", "appimage", appImagePath, "file", e.Name())
			fil, err := rdr.OpenFile(e.Name())
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(fil)
			_ = fil.Close()
			return data, err
		}
	}
	slog.Debug("no embedded desktop file found", "appimage", appImagePath)
	return nil, nil
}

// patchDesktopFile rewrites Exec= in all sections and Icon= / TryExec= in
// [Desktop Entry] to use the installed AppImage path and extracted icon.
// All other content from the author's file is preserved verbatim.
func patchDesktopFile(data []byte, execCmd, iconPath string) []byte {
	var out []byte
	inDesktopEntry := false
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i+1], data[i+1:]
		} else {
			line, data = data, nil
		}
		s := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(s, "[") {
			inDesktopEntry = strings.TrimSpace(s) == "[Desktop Entry]"
		}
		switch {
		case inDesktopEntry && strings.HasPrefix(s, "TryExec="):
			// Omit: points to a location not accessible outside the AppImage.
		case inDesktopEntry && strings.HasPrefix(s, "Icon="):
			out = append(out, ("Icon=" + iconPath + "\n")...)
		case strings.HasPrefix(s, "Exec="):
			out = append(out, (patchExecLine(s, execCmd) + "\n")...)
		default:
			out = append(out, line...)
		}
	}
	return out
}

// patchExecLine replaces the binary in an Exec= line with execCmd, preserving
// any trailing field codes or flags (e.g. %U, --new-window). --no-sandbox is
// stripped from the original args since execCmd controls whether it is added.
func patchExecLine(line, execCmd string) string {
	val := strings.TrimPrefix(line, "Exec=")
	parts := strings.Fields(val)
	var rest []string
	if len(parts) == 0 {
		return "Exec=" + execCmd
	}
	for _, p := range parts[1:] {
		if p != "--no-sandbox" {
			rest = append(rest, p)
		}
	}
	if len(rest) > 0 {
		return "Exec=" + execCmd + " " + strings.Join(rest, " ")
	}
	return "Exec=" + execCmd
}

// embeddedHasNoSandbox reports whether any Exec= line in the embedded .desktop
// data already includes --no-sandbox, meaning the author requires it.
func embeddedHasNoSandbox(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		s := string(line)
		if strings.HasPrefix(s, "Exec=") {
			for _, f := range strings.Fields(strings.TrimPrefix(s, "Exec=")) {
				if f == "--no-sandbox" {
					slog.Debug("embedded desktop file already includes --no-sandbox")
					return true
				}
			}
		}
	}
	return false
}

// IntegrationFiles returns the desktop integration files (if they exist) that
// would be removed by Remove: the .desktop file and any installed icon.
func IntegrationFiles(appName string) []string {
	var files []string
	if p := desktopFilePath(appName); fileExists(p) {
		files = append(files, p)
	}
	for _, ext := range []string{".png", ".svg", ".xpm"} {
		if p := iconFilePath(appName, ext); fileExists(p) {
			files = append(files, p)
		}
	}
	return files
}

func Remove(appName string) error {
	if err := removeIfExists(desktopFilePath(appName)); err != nil {
		return err
	}

	// Remove icon regardless of extension by checking all known extensions.
	for _, ext := range []string{".png", ".svg", ".xpm"} {
		if err := removeIfExists(iconFilePath(appName, ext)); err != nil {
			return err
		}
	}
	return nil
}

func extractIcon(appName, appImagePath string) (string, error) {
	f, err := os.Open(appImagePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	offset, err := appimage.FindSquashFSOffset(f)
	if err != nil {
		return "", err
	}

	rdr, err := squashfs.NewReaderAtOffset(f, offset)
	if err != nil {
		return "", fmt.Errorf("open squashfs: %w", err)
	}

	fil, err := rdr.OpenFile(".DirIcon")
	if err != nil {
		return "", fmt.Errorf("open .DirIcon: %w", err)
	}
	defer func() { _ = fil.Close() }()

	if fil.IsSymlink() {
		slog.Debug("following .DirIcon symlink", "target", fil.SymlinkPath())
		target, ok := fil.GetSymlinkFile().(*squashfs.File)
		if !ok {
			return "", fmt.Errorf("read .DirIcon: broken symlink to %s", fil.SymlinkPath())
		}
		_ = fil.Close()
		fil = target
	}

	data, err := io.ReadAll(fil)
	if err != nil {
		return "", fmt.Errorf("read .DirIcon: %w", err)
	}

	ext := detectIconFormat(data)
	slog.Debug("detected icon format", "ext", ext, "size", len(data))
	iconPath := iconFilePath(appName, ext)
	if err := os.MkdirAll(filepath.Dir(iconPath), 0755); err != nil {
		return "", fmt.Errorf("create icons dir: %w", err)
	}
	if err := os.WriteFile(iconPath, data, 0644); err != nil {
		return "", err
	}
	slog.Debug("wrote icon", "path", iconPath)
	return iconPath, nil
}

func detectIconFormat(data []byte) string {
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return ".png"
	}
	if bytes.HasPrefix(data, []byte("<svg")) || bytes.HasPrefix(data, []byte("<?xml")) {
		return ".svg"
	}
	return ".xpm"
}

func desktopFilePath(appName string) string {
	name := NormalizeName(appName) + ".desktop"
	return filepath.Join(xdg.DataHome, "applications", name)
}

func iconFilePath(appName, ext string) string {
	name := NormalizeName(appName) + ext
	return filepath.Join(xdg.DataHome, "icons", name)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
