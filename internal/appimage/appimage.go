// Package appimage provides utilities for inspecting AppImage files.
package appimage

import (
	"bytes"
	"debug/elf"
	"fmt"
	"log/slog"
	"os"

	"github.com/CalebQ42/squashfs"

	"github.com/sivel/appimg/internal/platform"
)

// FindSquashFSOffset returns the byte offset where the squashfs filesystem
// begins inside the AppImage. It uses ELF program headers to skip past the
// embedded runtime, then scans for the squashfs magic bytes.
func FindSquashFSOffset(f *os.File) (int64, error) {
	// LE squashfs v4: "hsqs" = 0x68 0x73 0x71 0x73
	// BE squashfs v4: "sqsh" = 0x73 0x71 0x73 0x68
	leMagic := []byte{0x68, 0x73, 0x71, 0x73}
	beMagic := []byte{0x73, 0x71, 0x73, 0x68}

	// Determine the end of the ELF runtime so we don't match magic bytes
	// embedded in the ELF binary itself.
	startOffset := int64(0)
	if ef, err := elf.NewFile(f); err == nil {
		for _, phdr := range ef.Progs {
			if end := int64(phdr.Off + phdr.Filesz); end > startOffset {
				startOffset = end
			}
		}
	}

	const maxScan = 4 * 1024 * 1024
	const chunkSize = 64 * 1024
	// Read in large chunks; overlap by 3 bytes so magic bytes spanning
	// chunk boundaries are not missed.
	buf := make([]byte, chunkSize)
	for pos := startOffset; pos < startOffset+maxScan; pos += chunkSize - 3 {
		n, err := f.ReadAt(buf, pos)
		if n < 4 {
			break
		}
		for i := 0; i <= n-4; i++ {
			window := buf[i : i+4]
			if bytes.Equal(window, leMagic) || bytes.Equal(window, beMagic) {
				off := pos + int64(i)
				slog.Debug("found squashfs offset", "offset", off)
				return off, nil
			}
		}
		if err != nil {
			break
		}
	}
	return 0, fmt.Errorf("squashfs magic not found in AppImage (is this a Type 2 AppImage?)")
}

// HasChromeSandbox reports whether the AppImage contains a chrome-sandbox
// binary, indicating it is a Chromium/Electron-based application.
func HasChromeSandbox(appImagePath string) bool {
	f, err := os.Open(appImagePath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	offset, err := FindSquashFSOffset(f)
	if err != nil {
		return false
	}

	rdr, err := squashfs.NewReaderAtOffset(f, offset)
	if err != nil {
		return false
	}

	_, err = rdr.OpenFile("chrome-sandbox")
	found := err == nil
	slog.Debug("chrome-sandbox check", "appimage", appImagePath, "found", found)
	return found
}

// RequiresNoSandbox returns true when the AppImage is Chromium-based and the
// system does not support unprivileged user namespaces, meaning --no-sandbox
// must be passed to prevent a fatal sandbox error at startup.
func RequiresNoSandbox(appImagePath string) bool {
	result := HasChromeSandbox(appImagePath) && !platform.SupportsUserNamespaces()
	slog.Debug("sandbox check", "appimage", appImagePath, "needs_no_sandbox", result)
	return result
}
