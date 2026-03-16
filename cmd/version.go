package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags for release builds.
// Development builds fall back to VCS metadata from runtime/debug.
var version string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), buildVersion())
	},
}

func buildVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	// Set by go install module@vX.Y.Z; not set for local go build.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				revision = s.Value[:7]
			} else {
				revision = s.Value
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	if revision == "" {
		return "unknown"
	}
	if dirty {
		return revision + "-dirty"
	}
	return revision
}

func init() {
	rootCmd.Version = buildVersion()
	rootCmd.AddCommand(versionCmd)
}
