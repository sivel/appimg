package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/cache"
	"github.com/sivel/appimg/internal/catalog"
	"github.com/sivel/appimg/internal/database"
)

var listCmd = &cobra.Command{
	Use:   "list [pattern]",
	Short: "List available or installed AppImages",
	Long: `List AppImages from the catalog. Optionally filter by a regex pattern matched
against the app name and description.

With --installed, lists only installed packages.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

var listInstalled bool

func init() {
	listCmd.Flags().BoolVar(&listInstalled, "installed", false, "list only installed packages")
}

func runList(cmd *cobra.Command, args []string) (retErr error) {
	cmd.SilenceUsage = true
	ctx := context.Background()
	pattern := ""
	if len(args) > 0 {
		pattern = "(?i)" + args[0]
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

	if listInstalled {
		return listInstalledPackages(cmd, db, pattern)
	}
	return listAvailablePackages(cmd, ctx, db, pattern)
}

func listInstalledPackages(cmd *cobra.Command, db *database.DB, pattern string) error {
	entries := db.All()
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No packages installed.")
		return nil
	}

	filtered, err := database.FilterEntries(entries, pattern)
	if err != nil {
		return err
	}

	for _, e := range filtered {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", e.Source, e.Version)
	}
	return nil
}

func listAvailablePackages(cmd *cobra.Command, ctx context.Context, db *database.DB, pattern string) error {
	if err := cache.RefreshIfExpired(ctx); err != nil {
		return fmt.Errorf("refresh catalog: %w", err)
	}

	feed, err := catalog.Load()
	if err != nil {
		return err
	}

	entries, err := catalog.Filter(feed, pattern)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if installed, ok := db.Get(e.Name); ok {
			fmt.Fprintf(cmd.OutOrStdout(), "catalog:%s [installed: %s]\n", e.Name, installed.Version)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "catalog:%s\n", e.Name)
		}
	}
	return nil
}
