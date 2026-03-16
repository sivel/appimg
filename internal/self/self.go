// Package self provides helpers for locating the appimg executable.
package self

import (
	"os"
	"os/exec"
)

// AppimgPath returns the path to use for the appimg executable in desktop
// files. It prefers the result of exec.LookPath("appimg") so that shim-based
// version managers (e.g. mise) resolve to their stable shim rather than the
// versioned installation path, which would break on upgrade. Falls back to
// os.Executable() if the name is not on PATH.
func AppimgPath() string {
	if p, err := exec.LookPath("appimg"); err == nil {
		return p
	}
	p, _ := os.Executable()
	return p
}
