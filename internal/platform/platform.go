// Package platform provides platform detection utilities for AppImage asset selection.
package platform

import (
	"log/slog"
	"os"
	"runtime"
	"strings"
)

// Arch returns the AppImage asset architecture string for the current platform.
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armhf"
	default:
		return runtime.GOARCH
	}
}

// OSInfo holds OS identification parsed from /etc/os-release.
type OSInfo struct {
	ID      string   // e.g. "ubuntu", "fedora"
	IDLike  []string // e.g. "ubuntu debian"
	Version string   // e.g. "25.10", "41"
}

// SupportsUserNamespaces reports whether unprivileged user namespaces are
// available to an unconfined process. Two conditions can prevent this:
//   - kernel.unprivileged_userns_clone=0 (Debian/Ubuntu sysctl)
//   - kernel.apparmor_restrict_unprivileged_userns=1 (Ubuntu 24.04+)
func SupportsUserNamespaces() bool {
	if data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		slog.Debug("unprivileged_userns_clone", "value", string(data[:1]))
		if len(data) > 0 && data[0] == '0' {
			return false
		}
	}
	if data, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns"); err == nil {
		slog.Debug("apparmor_restrict_unprivileged_userns", "value", string(data[:1]))
		if len(data) > 0 && data[0] == '1' {
			return false
		}
	}
	return true
}

// OS reads /etc/os-release and returns the current distribution info.
// Returns a zero value if the file cannot be read.
func OS() OSInfo {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OSInfo{}
	}
	var info OSInfo
	matches := 0
	for _, line := range strings.Split(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, "ID="); ok {
			info.ID = strings.ToLower(strings.Trim(after, "\""))
			matches++
		} else if after, ok := strings.CutPrefix(line, "ID_LIKE="); ok {
			info.IDLike = strings.Split(strings.Trim(after, "\""), " ")
			matches++
		} else if after, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
			info.Version = strings.Trim(after, "\"")
			matches++
		}
		if matches == 3 {
			break
		}
	}
	return info
}
