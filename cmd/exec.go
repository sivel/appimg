package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/sivel/appimg/internal/appimage"
	"github.com/sivel/appimg/internal/database"
	"github.com/sivel/appimg/internal/resolver"
)

var execCmd = &cobra.Command{
	Use:   "exec <name> [args...]",
	Short: "Run an installed AppImage with error reporting",
	Long: `Run an installed AppImage by name, forwarding any additional arguments.
On non-zero exit, an error dialog is shown via zenity, kdialog, or notify-send.

This command is used as the Exec target in .desktop files so that launch
failures are surfaced to the user rather than silently discarded.`,
	// DisableFlagParsing passes all args after the subcommand through as-is,
	// so app-level flags (e.g. --new-window) are not consumed by cobra.
	DisableFlagParsing: true,
	RunE:               runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	if len(args) == 0 {
		return fmt.Errorf("exec requires an app name")
	}
	name := args[0]
	passArgs := args[1:]

	db, err := database.Open()
	if err != nil {
		return err
	}
	entry, ok := db.Find(name)
	// Release the lock immediately; the app may run for a long time.
	_ = db.Close()
	if !ok {
		return &resolver.ErrNotFound{Name: name}
	}

	cmdArgs := []string{entry.InstalledPath}
	if appimage.RequiresNoSandbox(entry.InstalledPath) {
		cmdArgs = append(cmdArgs, "--no-sandbox")
	}
	cmdArgs = append(cmdArgs, passArgs...)

	var stderrBuf bytes.Buffer
	c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	c.Stdin = os.Stdin

	if err := c.Run(); err != nil {
		showErrorDialog(entry.Name, stderrBuf.String())
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// showErrorDialog attempts to display an error dialog using the first available
// tool: zenity (GTK), kdialog (KDE), or notify-send as a last resort.
func showErrorDialog(appName, errText string) {
	const maxLen = 2000
	msg := errText
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "\n..."
	}

	title := appName + " failed to start"

	tools := [][]string{
		{"zenity", "--error", "--no-markup", "--title", title, "--text", msg},
		{"kdialog", "--error", msg, "--title", title},
		{"notify-send", "-u", "critical", "-a", appName, title, msg},
	}

	for _, tool := range tools {
		if path, err := exec.LookPath(tool[0]); err == nil {
			_ = exec.Command(path, tool[1:]...).Run()
			return
		}
	}
}
