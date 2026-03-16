package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/database"
	"github.com/sivel/appimg/internal/desktop"
	"github.com/sivel/appimg/internal/lock"
	"github.com/sivel/appimg/internal/resolver"
)

var removeCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"uninstall"},
	Short:   "Remove an installed AppImage",
	Args:    cobra.ExactArgs(1),
	RunE:    runRemove,
}

var removeYes bool

func init() {
	removeCmd.Flags().BoolVarP(&removeYes, "yes", "y", false, "skip confirmation prompt")
}

func runRemove(cmd *cobra.Command, args []string) (retErr error) {
	cmd.SilenceUsage = true
	name := args[0]

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

	entry, ok := db.Find(name)
	if !ok {
		return &resolver.ErrNotFound{Name: name}
	}

	if !removeYes {
		fmt.Fprintf(cmd.OutOrStdout(), "Uninstalling %s:\n", entry.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "  Would remove:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", entry.InstalledPath)
		for _, f := range desktop.IntegrationFiles(entry.Name) {
			fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", f)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Proceed (y/N)? ")
		scanner := bufio.NewScanner(cmd.InOrStdin())
		scanner.Scan()
		if !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	if err := os.Remove(entry.InstalledPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove AppImage: %w", err)
	}

	if err := desktop.Remove(entry.Name); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: desktop cleanup failed: %v\n", err)
	}

	db.Delete(entry.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", entry.Name)
	return nil
}
