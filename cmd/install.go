package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/database"
	"github.com/sivel/appimg/internal/desktop"
	"github.com/sivel/appimg/internal/lock"
	"github.com/sivel/appimg/internal/resolver"
	"github.com/sivel/appimg/internal/self"
	"github.com/sivel/appimg/internal/transfer"
)

var installCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install an AppImage",
	Long: `Install an AppImage from the catalog, a GitHub release, a local file, or a URL.

Local and URL installs are stored with source "local:" and cannot be updated via appimg update.
Use --force to reinstall or change the pinned version of an already-installed package.

Examples:
  appimg install Bitwarden
  appimg install Bitwarden@2024.1.0
  appimg install --force Bitwarden@2026.1.0
  appimg install github:bambulab/BambuStudio
  appimg install github:bambulab/BambuStudio@02.05.00.67 --asset-pattern "*linux*x86_64*.AppImage"
  appimg install ~/Downloads/Bitwarden-2026.2.1-x86_64.AppImage
  appimg install https://example.com/SomeApp-x86_64.AppImage`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

var (
	installAssetPattern  string
	installPrerelease    bool
	installRolling       bool
	installVersionPrefix string
	installForce         bool
)

func init() {
	installCmd.Flags().StringVar(&installAssetPattern, "asset-pattern", "", "glob pattern to select a specific release asset")
	installCmd.Flags().BoolVar(&installPrerelease, "pre", false, "include pre-releases when selecting the latest version")
	installCmd.Flags().BoolVar(&installRolling, "rolling", false, "mark as a rolling release; update uses published_at instead of tag comparison")
	installCmd.Flags().StringVar(&installVersionPrefix, "version-prefix", "", "restrict releases to those whose tag begins with this prefix")
	installCmd.Flags().BoolVarP(&installForce, "force", "f", false, "reinstall even if already installed, replacing the existing entry")
}

// installDir returns ~/Applications, creating it if necessary.
func installDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	dir := filepath.Join(home, "Applications")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create install dir: %w", err)
	}
	return dir, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") || strings.HasSuffix(s, ".AppImage")
}

func runInstall(cmd *cobra.Command, args []string) (retErr error) {
	cmd.SilenceUsage = true

	if isURL(args[0]) || isLocalPath(args[0]) {
		return runInstallLocal(cmd, args[0])
	}

	ctx := context.Background()

	src, version, err := resolver.Parse(ctx, args[0])
	if err != nil {
		return err
	}

	l, err := lock.Acquire()
	if err != nil {
		return err
	}
	defer func() { _ = l.Release() }()

	db, err := database.Open()
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	existing, installed := db.Get(src.AppName)
	if installed && !installForce {
		return fmt.Errorf("%s is already installed; use 'appimg update' to upgrade or 'appimg install --force' to reinstall", src.AppName)
	}

	opts := backend.Options{
		Version:       version,
		Prerelease:    installPrerelease,
		AssetPattern:  installAssetPattern,
		VersionPrefix: installVersionPrefix,
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Fetching release info for %s...\n", src.AppName)
	release, err := src.Backend.LatestRelease(ctx, src.Project, opts)
	if err != nil {
		return err
	}

	asset, err := backend.SelectAsset(release.Assets, opts)
	if err != nil {
		return err
	}

	dir, err := installDir()
	if err != nil {
		return err
	}
	destPath := filepath.Join(dir, asset.Name)

	fmt.Fprintf(cmd.OutOrStdout(), "Installing %s %s...\n", src.AppName, release.TagName)
	size := asset.Size
	if size == 0 {
		size = transfer.ContentLength(ctx, asset.BrowserDownloadURL)
	}
	bar := progressbar.DefaultBytes(size, "downloading")
	if err := transfer.Download(ctx, asset.BrowserDownloadURL, destPath, bar); err != nil {
		return err
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("chmod AppImage: %w", err)
	}

	if installed && existing.InstalledPath != destPath {
		if err := os.Remove(existing.InstalledPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not remove old file %s: %v\n", existing.InstalledPath, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Setting up desktop integration...\n")
	if err := desktop.Install(src.AppName, destPath, self.AppimgPath(), src.Categories); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: desktop integration failed: %v\n", err)
	}

	rolling := installRolling || backend.IsRollingTag(release.TagName)

	entry := &database.Entry{
		Name:          src.AppName,
		Source:        src.Source,
		AssetPattern:  installAssetPattern,
		InstalledPath: destPath,
		Version:       release.TagName,
		InstalledAt:   time.Now().UTC(),
		DownloadURL:   asset.BrowserDownloadURL,
		PublishedAt:   release.PublishedAt,
		Prerelease:    installPrerelease,
		PinnedVersion: version,
		Rolling:       rolling,
		VersionPrefix: installVersionPrefix,
	}
	db.Set(entry)

	fmt.Fprintf(cmd.OutOrStdout(), "Installed %s %s\n", src.AppName, release.TagName)
	return nil
}

func runInstallLocal(cmd *cobra.Command, arg string) (retErr error) {
	ctx := context.Background()

	dir, err := installDir()
	if err != nil {
		return err
	}

	var destPath string
	if isURL(arg) {
		filename := path.Base(arg)
		destPath = filepath.Join(dir, filename)
		fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n", filename)
		bar := progressbar.DefaultBytes(-1, "downloading")
		if err := transfer.Download(ctx, arg, destPath, bar); err != nil {
			return err
		}
	} else {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return err
		}
		destPath = filepath.Join(dir, filepath.Base(abs))
		if abs != destPath {
			fmt.Fprintf(cmd.OutOrStdout(), "Copying %s...\n", filepath.Base(abs))
			if err := transfer.Copy(abs, destPath); err != nil {
				return err
			}
		}
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("chmod AppImage: %w", err)
	}

	info := desktop.ExtractAppInfo(destPath)

	l, err := lock.Acquire()
	if err != nil {
		return err
	}
	defer func() { _ = l.Release() }()

	db, err := database.Open()
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	existing, installed := db.Get(info.Name)
	if installed && !installForce {
		return fmt.Errorf("%s is already installed; use 'appimg update' to upgrade or 'appimg install --force' to reinstall", info.Name)
	}
	if installed && existing.InstalledPath != destPath {
		if err := os.Remove(existing.InstalledPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not remove old file %s: %v\n", existing.InstalledPath, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Setting up desktop integration...\n")
	if err := desktop.Install(info.Name, destPath, self.AppimgPath(), nil); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: desktop integration failed: %v\n", err)
	}

	db.Set(&database.Entry{
		Name:          info.Name,
		Source:        "local:" + info.Name,
		InstalledPath: destPath,
		Version:       info.Version,
		InstalledAt:   time.Now().UTC(),
	})

	fmt.Fprintf(cmd.OutOrStdout(), "Installed %s", info.Name)
	if info.Version != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " %s", info.Version)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}
