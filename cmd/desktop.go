package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/database"
	"github.com/sivel/appimg/internal/desktop"
	"github.com/sivel/appimg/internal/self"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Manage desktop integration for installed AppImages",
}

var desktopRegenerateCmd = &cobra.Command{
	Use:   "regenerate",
	Short: "Regenerate desktop files for all installed AppImages",
	Long: `Regenerate the .desktop file and icon for every installed AppImage.

Useful after moving or upgrading appimg itself, since the Exec= path in each
desktop file points to the appimg binary and would otherwise become stale.`,
	RunE: runDesktopRegenerate,
}

func init() {
	desktopCmd.AddCommand(desktopRegenerateCmd)
}

func runDesktopRegenerate(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	db, err := database.Open()
	if err != nil {
		return err
	}
	entries := db.All()
	if err := db.Close(); err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No packages installed.")
		return nil
	}

	exe := self.AppimgPath()
	var nerr int
	for _, entry := range entries {
		_ = desktop.Remove(entry.Name)
		if err := desktop.Install(entry.Name, entry.InstalledPath, exe, nil); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: desktop integration failed: %v\n", entry.Name, err)
			nerr++
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "regenerated %s\n", entry.Name)
	}

	if nerr > 0 {
		return fmt.Errorf("%d app(s) failed desktop regeneration", nerr)
	}
	return nil
}
