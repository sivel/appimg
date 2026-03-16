package main

import (
	"errors"
	"fmt"
	"os"

	_ "github.com/sivel/appimg/internal/backend/codeberg"
	_ "github.com/sivel/appimg/internal/backend/github"
	_ "github.com/sivel/appimg/internal/backend/gitlab"

	"github.com/sivel/appimg/cmd"
	"github.com/sivel/appimg/internal/resolver"
)

func main() {
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, cmd.ErrNoSubcommand) {
			fmt.Fprintln(os.Stderr, err)
		}
		if cmd.UsagePrinted() || errors.Is(err, cmd.ErrNoSubcommand) {
			os.Exit(2)
		}
		var nfe *resolver.ErrNotFound
		if errors.As(err, &nfe) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}
