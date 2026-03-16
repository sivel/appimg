package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/cache"
	"github.com/sivel/appimg/internal/database"
	"github.com/sivel/appimg/internal/desktop"
	"github.com/sivel/appimg/internal/lock"
	"github.com/sivel/appimg/internal/resolver"
	"github.com/sivel/appimg/internal/self"
	"github.com/sivel/appimg/internal/transfer"
)

var updateCmd = &cobra.Command{
	Use:   "update [name...]",
	Short: "Upgrade installed AppImages to newer versions",
	Long: `Upgrade all installed AppImages, or specific ones if names are given.
Packages installed with a pinned version (@version) are skipped unless --bump is used.`,
	RunE: runUpdate,
}

var (
	updateList       bool
	updatePrerelease bool
	updateForce      bool
	updateBump       bool
)

func init() {
	updateCmd.Flags().BoolVar(&updateList, "list", false, "list available updates without applying them")
	updateCmd.Flags().BoolVar(&updatePrerelease, "pre", false, "include pre-releases when checking for updates")
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "re-download packages unconditionally, bypassing version and timestamp checks")
	updateCmd.Flags().BoolVar(&updateBump, "bump", false, "advance pinned packages to the latest version, updating the stored pin")
}

func runUpdate(cmd *cobra.Command, args []string) (retErr error) {
	cmd.SilenceUsage = true
	ctx := context.Background()

	if err := cache.RefreshIfExpired(ctx); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not refresh catalog cache: %v\n", err)
	}

	if !updateList {
		l, err := lock.Acquire()
		if err != nil {
			return err
		}
		defer func() { _ = l.Release() }()
	}

	db, err := database.Open()
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	entries := db.All()
	if len(args) > 0 {
		filter := make(map[string]struct{}, len(args))
		for _, a := range args {
			filter[a] = struct{}{}
		}
		var filtered []*database.Entry
		for _, e := range entries {
			if _, ok := filter[e.Name]; ok {
				filtered = append(filtered, e)
				continue
			}
			if _, ok := filter[e.Source]; ok {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No packages installed.")
		return nil
	}

	updatesFound := false
	for _, entry := range entries {
		if entry.PinnedVersion != "" && !updateBump {
			fmt.Fprintf(cmd.OutOrStdout(), "Skipping %s (pinned at %s)\n", entry.Name, entry.PinnedVersion)
			continue
		}

		prerelease := updatePrerelease || entry.Prerelease
		opts := backend.Options{
			Prerelease:    prerelease,
			AssetPattern:  entry.AssetPattern,
			VersionPrefix: entry.VersionPrefix,
		}

		b, project, err := backendFromSource(entry.Source)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping %s: %v\n", entry.Name, err)
			continue
		}

		latest, err := b.LatestRelease(ctx, project, opts)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check %s: %v\n", entry.Name, err)
			continue
		}

		if !updateForce && !isNewer(entry.Version, entry.PublishedAt, latest.TagName, latest.PublishedAt, entry.Rolling) {
			continue
		}

		updatesFound = true
		if updateList {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s -> %s\n", entry.Name, entry.Version, latest.TagName)
			continue
		}

		asset, err := backend.SelectAsset(latest.Assets, opts)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not select asset for %s: %v\n", entry.Name, err)
			continue
		}

		dir, err := installDir()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
			continue
		}
		newPath := filepath.Join(dir, asset.Name)

		fmt.Fprintf(cmd.OutOrStdout(), "Updating %s %s -> %s...\n", entry.Name, entry.Version, latest.TagName)
		size := asset.Size
		if size == 0 {
			size = transfer.ContentLength(ctx, asset.BrowserDownloadURL)
		}
		bar := progressbar.DefaultBytes(size, "downloading")
		if err := transfer.Download(ctx, asset.BrowserDownloadURL, newPath, bar); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to download %s: %v\n", entry.Name, err)
			continue
		}

		if err := os.Chmod(newPath, 0755); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: chmod %s: %v\n", entry.Name, err)
			continue
		}

		if entry.InstalledPath != newPath {
			if err := os.Remove(entry.InstalledPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not remove old file %s: %v\n", entry.InstalledPath, err)
			}
		}

		_ = desktop.Remove(entry.Name)
		if err := desktop.Install(entry.Name, newPath, self.AppimgPath(), nil); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: desktop integration failed: %v\n", err)
		}

		entry.InstalledPath = newPath
		entry.Version = latest.TagName
		entry.PublishedAt = latest.PublishedAt
		entry.DownloadURL = asset.BrowserDownloadURL
		entry.InstalledAt = time.Now().UTC()
		if updateBump && entry.PinnedVersion != "" {
			entry.PinnedVersion = latest.TagName
		}
		db.Set(entry)

		fmt.Fprintf(cmd.OutOrStdout(), "Updated %s to %s\n", entry.Name, latest.TagName)
	}

	if updateList && !updatesFound {
		fmt.Fprintln(cmd.OutOrStdout(), "All packages are up to date.")
	}
	return nil
}

// isNewer reports whether latestTag/latestPublished is newer than currentTag/currentPublished.
// For rolling releases, tag comparison is skipped and published_at timestamps are always used.
// For normal releases, semver comparison is attempted first with a published_at fallback.
func isNewer(currentTag, currentPublished, latestTag, latestPublished string, rolling bool) bool {
	if !rolling {
		if currentTag == latestTag {
			return false
		}
		cv, cerr := semver.NewVersion(currentTag)
		lv, lerr := semver.NewVersion(latestTag)
		if cerr == nil && lerr == nil {
			if lv.GreaterThan(cv) {
				return true
			}
			if !lv.Equal(cv) {
				return false
			}
			// Semver coerced both tags to the same value (e.g. four-part versions
			// like 02.05.00.67 and 02.05.00.68 both become 02.05.00). Fall through
			// to published_at comparison to break the tie.
		}
	}
	// Rolling releases, non-semver tags, or semver-equal tags with different
	// strings: compare by published_at (RFC3339 sorts lexicographically).
	return latestPublished > currentPublished
}

// backendFromSource extracts the backend and project from a source string like
// "catalog:Bitwarden", "github:owner/repo", or "gitlab:namespace/project".
func backendFromSource(source string) (backend.Backend, string, error) {
	if b, project, _, ok := backend.Lookup(source); ok {
		return b, project, nil
	}
	if after, ok := strings.CutPrefix(source, "catalog:"); ok {
		ctx := context.Background()
		resolved, _, err := resolver.Parse(ctx, after)
		if err != nil {
			return nil, "", err
		}
		return resolved.Backend, resolved.Project, nil
	}
	if strings.HasPrefix(source, "local:") {
		return nil, "", fmt.Errorf("local installs cannot be updated via appimg update")
	}
	return nil, "", fmt.Errorf("unknown source format: %s", source)
}
