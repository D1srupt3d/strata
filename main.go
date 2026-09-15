package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"strata/internal/config"
	"strata/internal/engine"
	"strata/internal/layers"
	"strata/internal/state"
)

// CalVer YYYY.M.PATCH with an un-padded month (2026.8.0 — GoReleaser
// enforces semver and rejects 2026.08.0). Release builds and install.sh
// overwrite this via -X main.version, so the in-source value only shows up
// in a bare `go build` — hence the -dev suffix.
var version = "2026.9.0-dev"

// channel says how this binary was built. GoReleaser stamps "release" via
// -X main.channel=release; every other build (install.sh, go build) stays
// "source". Only release builds replace themselves with `strata upgrade` —
// a source build is its owner's to update. A var, like version, because -X
// can't set a const.
var channel = "source"

// paths resolves where strata looks for things, honoring test/env overrides.
type paths struct {
	Home    string // target home dir (STRATA_HOME overrides)
	Machine string // machine.toml (STRATA_CONFIG overrides)
	State   string // state.json (STRATA_STATE overrides)
	Bin     string // the strata binary itself (STRATA_BIN overrides)
}

func resolvePaths() (paths, error) {
	home := os.Getenv("STRATA_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return paths{}, err
		}
		home = h
	}
	p := paths{Home: home}
	p.Machine = os.Getenv("STRATA_CONFIG")
	if p.Machine == "" {
		p.Machine = filepath.Join(home, ".config", "strata", "machine.toml")
	}
	p.State = os.Getenv("STRATA_STATE")
	if p.State == "" {
		p.State = filepath.Join(home, ".local", "state", "strata", "state.json")
	}
	p.Bin = os.Getenv("STRATA_BIN")
	if p.Bin == "" {
		if exe, err := os.Executable(); err == nil {
			p.Bin = exe
		}
	}
	return p, nil
}

// appContext assembles everything a command needs from disk.
type appContext struct {
	Paths paths
	Cfg   config.Config
	State state.State
}

func loadContext() (*appContext, error) {
	p, err := resolvePaths()
	if err != nil {
		return nil, err
	}
	mc, err := config.LoadMachineConfig(p.Machine)
	if err != nil {
		return nil, err
	}
	rc, err := config.LoadRepoConfig(mc.Repo)
	if err != nil {
		return nil, err
	}
	st, err := state.Load(p.State)
	if err != nil {
		return nil, err
	}
	return &appContext{Paths: p, Cfg: config.Merge(rc, mc), State: st}, nil
}

func (a *appContext) plan() ([]engine.Item, error) {
	return engine.Plan(a.Cfg, a.Paths.Home, a.State, runtime.GOOS, layers.ReadOSRelease())
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "strata",
		Short: "strata — layered dotfiles, sanely",
		Long: `strata manages your dotfiles from one git repo of real-named files.

Layers stack per machine: base → OS (mac / linux / <distro> / windows) →
role layers listed in ~/.config/strata/machine.toml. When two layers
contain the same path, the later layer's whole file wins. Files opted in
via dots.toml get {{var}} substitution; permission globs and post-apply
hooks are also declared there.

strata remembers what it last wrote, so it always knows the difference
between "the repo changed", "you edited the file in $HOME", and "both" —
and never silently overwrites your local edits.

Running strata with no subcommand opens a read-only TUI of the whole
picture: every file, which layer wins on every OS, and where each var,
hook, and permission comes from.`,
		Example: `  strata                  open the TUI
  strata status           one line per file that needs attention
  strata edit .zshrc      edit the source, see the diff, apply
  strata apply            write pending changes into $HOME
  strata init <git-url>   set up a brand-new machine`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          runTUI, // bare `strata` opens the read-only TUI
	}
	root.AddCommand(newStatusCmd(), newDiffCmd(), newApplyCmd(), newAddCmd(),
		newEditCmd(), newInitCmd(), newSyncCmd(), newRmCmd(), newUninstallCmd(), newUpgradeCmd())
	return root
}

// exitCode lets a command set a non-zero exit status without printing an
// error — e.g. `status` when files need attention: that's an answer, not a
// failure.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

// exitStatus maps a command's result to the process exit code and the
// message to print, if any.
func exitStatus(err error) (code int, msg string) {
	if err == nil {
		return 0, ""
	}
	var c exitCode
	if errors.As(err, &c) {
		return int(c), ""
	}
	return 1, "error: " + err.Error()
}

func main() {
	code, msg := exitStatus(newRootCmd().Execute())
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(code)
}
