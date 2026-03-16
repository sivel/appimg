package cmd

import "errors"

// ErrNoSubcommand is returned by the root command when invoked without a subcommand.
var ErrNoSubcommand = errors.New("no subcommand specified")
