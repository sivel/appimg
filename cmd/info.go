package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/resolver"
)

var infoCmd = &cobra.Command{
	Use:   "info <source>",
	Short: "Show release information for an AppImage source",
	Long: `Show the latest release version and available AppImage assets for a source.

Examples:
  appimg info Bitwarden
  appimg info Bitwarden@2024.1.0
  appimg info github:bambulab/BambuStudio
  appimg info github:bambulab/BambuStudio@02.05.00.67
  appimg info --pre github:bambulab/BambuStudio`,
	Args: cobra.ExactArgs(1),
	RunE: runInfo,
}

var (
	infoPrerelease    bool
	infoAll           bool
	infoVersionPrefix string
)

func init() {
	infoCmd.Flags().BoolVar(&infoPrerelease, "pre", false, "include pre-releases when selecting the latest version")
	infoCmd.Flags().BoolVar(&infoAll, "all", false, "list all available versions instead of showing details for one")
	infoCmd.Flags().StringVar(&infoVersionPrefix, "version-prefix", "", "restrict releases to those whose tag begins with this prefix")
}

func runInfo(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	ctx := context.Background()

	src, version, err := resolver.Parse(ctx, args[0])
	if err != nil {
		return err
	}

	if infoAll {
		return runInfoAll(cmd, ctx, src)
	}

	opts := backend.Options{
		Version:       version,
		Prerelease:    infoPrerelease,
		VersionPrefix: infoVersionPrefix,
	}
	release, err := src.Backend.LatestRelease(ctx, src.Project, opts)
	if err != nil {
		return err
	}

	projectURL := src.Backend.ProjectURL(src.Project)
	releaseURL := src.Backend.ReleaseURL(src.Project, release.TagName)

	fmt.Fprintf(cmd.OutOrStdout(), "Name:        %s\n", src.AppName)
	fmt.Fprintf(cmd.OutOrStdout(), "Source:      %s\n", src.Source)
	if src.Description != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", src.Description)
	}
	if len(src.Categories) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Categories:  %s\n", strings.Join(src.Categories, ", "))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "URL:         %s\n", projectURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Version:     %s\n", release.TagName)
	fmt.Fprintf(cmd.OutOrStdout(), "Published:   %s\n", release.PublishedAt)
	fmt.Fprintf(cmd.OutOrStdout(), "Pre-release: %v\n", release.Prerelease)
	fmt.Fprintf(cmd.OutOrStdout(), "Release URL: %s\n", releaseURL)

	scored, err := backend.ScoreAssets(release.Assets, opts)
	if err != nil {
		// No AppImage assets — still show the section header.
		fmt.Fprintf(cmd.OutOrStdout(), "Assets (0):\n")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Assets (%d):\n", len(scored))
	for i, s := range scored {
		marker := "  "
		if i == 0 {
			marker = "* "
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s%s (%s) [score: %d]\n", marker, s.Asset.Name, formatBytes(s.Asset.Size), s.Score)
	}
	return nil
}

func runInfoAll(cmd *cobra.Command, ctx context.Context, src *resolver.ResolvedSource) error {
	releases, err := src.Backend.AllReleases(ctx, src.Project, backend.Options{Prerelease: infoPrerelease, VersionPrefix: infoVersionPrefix})
	if err != nil {
		return err
	}
	if len(releases) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No releases found.")
		return nil
	}
	for _, r := range releases {
		appimageCount := 0
		for _, a := range r.Assets {
			if strings.HasSuffix(strings.ToLower(a.Name), ".appimage") {
				appimageCount++
			}
		}
		preLabel := ""
		if r.Prerelease {
			preLabel = " [pre-release]"
		}
		published := r.PublishedAt
		if len(published) > 10 {
			published = published[:10]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %d AppImage(s)%s\n",
			r.TagName, published, appimageCount, preLabel)
	}
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
