// Package cmd implements the appimg CLI commands.
package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var usagePrinted bool
var verbose bool

var rootCmd = &cobra.Command{
	Use:           "appimg",
	Short:         "AppImage package manager",
	Long:          "appimg is a package manager for AppImages using the appimage.github.io catalog and GitHub releases.",
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		_ = cmd.Help()
		return ErrNoSubcommand
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// UsagePrinted reports whether cobra printed usage during the last Execute call,
// indicating the error was caused by incorrect usage rather than a runtime failure.
func UsagePrinted() bool { return usagePrinted }

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging to stderr")

	// Wrap the usage func so we can detect when cobra prints usage (i.e. a
	// usage error like wrong arg count or unknown flag) vs a runtime error.
	usageFunc := rootCmd.UsageFunc()
	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		usagePrinted = true
		return usageFunc(cmd)
	})

	rootCmd.AddCommand(
		installCmd,
		removeCmd,
		infoCmd,
		updateCmd,
		listCmd,
		downloadCmd,
		execCmd,
		desktopCmd,
	)
}
