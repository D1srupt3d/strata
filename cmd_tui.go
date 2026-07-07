package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"strata/internal/config"
	"strata/internal/layers"
	"strata/internal/state"
	"strata/internal/tui"
)

// runTUI launches the read-only viewer. It is the default action when strata
// is run with no subcommand.
func runTUI(cmd *cobra.Command, args []string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("the TUI needs an interactive terminal (use 'strata status' in scripts)")
	}
	p, err := resolvePaths()
	if err != nil {
		return err
	}
	mc, err := config.LoadMachineConfig(p.Machine)
	if err != nil {
		return err
	}
	rc, err := config.LoadRepoConfig(mc.Repo)
	if err != nil {
		return err
	}
	st, err := state.Load(p.State)
	if err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		host = "this machine"
	}
	snap, err := tui.Build(rc, mc, p.Home, st, runtime.GOOS, layers.ReadOSRelease(), host)
	if err != nil {
		return err
	}
	return tui.Run(snap)
}
