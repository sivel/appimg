package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/resolver"
	"github.com/sivel/appimg/internal/transfer"
)

var downloadCmd = &cobra.Command{
	Use:   "download <source>",
	Short: "Download an AppImage without installing it",
	Long: `Download an AppImage to a directory without installing it.

Examples:
  appimg download Bitwarden
  appimg download github:bambulab/BambuStudio --directory ~/Downloads`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

var (
	downloadDir           string
	downloadAssetPattern  string
	downloadPrerelease    bool
	downloadVersionPrefix string
)

func init() {
	downloadCmd.Flags().StringVarP(&downloadDir, "directory", "d", "", "destination directory (default: XDG_DOWNLOAD_DIR)")
	downloadCmd.Flags().StringVar(&downloadAssetPattern, "asset-pattern", "", "glob pattern to select a specific release asset")
	downloadCmd.Flags().BoolVar(&downloadPrerelease, "pre", false, "include pre-releases when selecting the latest version")
	downloadCmd.Flags().StringVar(&downloadVersionPrefix, "version-prefix", "", "restrict releases to those whose tag begins with this prefix")
}

func runDownload(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	ctx := context.Background()

	src, version, err := resolver.Parse(ctx, args[0])
	if err != nil {
		return err
	}

	opts := backend.Options{
		Version:       version,
		Prerelease:    downloadPrerelease,
		AssetPattern:  downloadAssetPattern,
		VersionPrefix: downloadVersionPrefix,
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

	destDir := downloadDir
	if destDir == "" {
		destDir = xdg.UserDirs.Download
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create download dir: %w", err)
	}
	destPath := filepath.Join(destDir, asset.Name)

	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s %s...\n", src.AppName, release.TagName)
	size := asset.Size
	if size == 0 {
		size = transfer.ContentLength(ctx, asset.BrowserDownloadURL)
	}
	bar := progressbar.DefaultBytes(size, "downloading")
	if err := transfer.Download(ctx, asset.BrowserDownloadURL, destPath, bar); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Downloaded to %s\n", destPath)
	return nil
}
