// Package cache manages the local copy of the appimage.github.io feed.json.
package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"

	"github.com/sivel/appimg/internal/httpclient"
)

var feedURL = "https://appimage.github.io/feed.json"

const (
	cacheKey = "appimg/feed.json"
	cacheTTL = 24 * time.Hour
)

// Path returns the absolute path to the cached feed.json file.
func Path() (string, error) {
	return xdg.CacheFile(cacheKey)
}

// IsExpired reports whether the cache is missing or older than the TTL.
func IsExpired() (bool, error) {
	path, err := Path()
	if err != nil {
		return true, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return time.Since(info.ModTime()) > cacheTTL, nil
}

// Refresh fetches feed.json from the upstream source and writes it to the cache.
func Refresh(ctx context.Context) error {
	slog.Debug("fetching catalog", "url", feedURL)
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return fmt.Errorf("fetch feed: %w", err)
	}
	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch feed: unexpected status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "feed-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write feed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	return os.Rename(tmpName, path)
}

// RefreshIfExpired refreshes the cache only if it is missing or expired.
func RefreshIfExpired(ctx context.Context) error {
	expired, err := IsExpired()
	if err != nil {
		return err
	}
	if !expired {
		slog.Debug("catalog cache is fresh, skipping fetch")
		return nil
	}
	return Refresh(ctx)
}
