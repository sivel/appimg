// Package transfer provides utilities for downloading and copying files.
package transfer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/schollz/progressbar/v3"

	"github.com/sivel/appimg/internal/httpclient"
)

// ContentLength issues a HEAD request to url and returns the Content-Length
// value. Returns -1 if the header is absent, the server doesn't support HEAD,
// or any error occurs.
func ContentLength(ctx context.Context, url string) int64 {
	slog.Debug("head request for content length", "url", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return -1
	}
	req.Header.Set("User-Agent", "appimg")
	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return -1
	}
	_ = resp.Body.Close()
	slog.Debug("content length", "url", url, "size", resp.ContentLength)
	return resp.ContentLength
}

// Download downloads url to destPath via an atomic temp-file write.
func Download(ctx context.Context, url, destPath string, bar *progressbar.ProgressBar) error {
	slog.Debug("downloading file", "url", url, "dest", destPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "appimg")

	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: unexpected status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), "appimg-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(io.MultiWriter(tmp, bar), resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	return os.Rename(tmpName, destPath)
}

// Copy copies src to dst via an atomic temp-file write.
func Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "appimg-copy-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpName, dst)
}
