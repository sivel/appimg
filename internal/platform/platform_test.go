package platform

import (
	"runtime"
	"testing"
)

func TestArch(t *testing.T) {
	got := Arch()
	if got == "" {
		t.Fatal("Arch() returned empty string")
	}
	// On known architectures the result must differ from the raw GOARCH value.
	known := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
		"386":   "i686",
		"arm":   "armhf",
	}
	if want, ok := known[runtime.GOARCH]; ok && got != want {
		t.Errorf("Arch() = %q, want %q for GOARCH=%q", got, want, runtime.GOARCH)
	}
}
