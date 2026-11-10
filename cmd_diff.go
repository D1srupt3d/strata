package main

import (
	"fmt"
	"io"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"

	"strata/internal/engine"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [file]...",
		Short: "Show what apply would change, including drift from edits made in $HOME",
		Long: `Unified diff of every non-clean file: home/<file> (what's on disk now)
against repo/<file> (what apply would write). Name files to see just
those; a named file that's clean prints nothing.

Because it compares in both directions, edits you made directly in $HOME
show up too - as lines apply would remove. No drift is ever silent.

Each header shows that copy's last-modified time, and the home header says
which copy has the newer edit (drifted: $HOME, update: the repo). That
comes from strata's hashes, not the clock. The times help with a conflict,
where both changed - but a git pull stamps repo files with the pull time,
not the time of the edit.`,
		Example: `  strata diff
  strata diff .zshrc ~/.gitconfig   # just these files
  strata diff | less`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeManagedFiles,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			var only map[string]bool
			if len(args) > 0 {
				items, err := app.plan()
				if err != nil {
					return err
				}
				managed := map[string]bool{}
				for _, it := range items {
					managed[it.Rel] = true
				}
				only = map[string]bool{}
				for _, arg := range args {
					rel, err := relFromArg(arg, app.Paths.Home)
					if err != nil {
						return err
					}
					if !managed[rel] {
						return fmt.Errorf("%s is not managed (try: strata add %s)", rel, rel)
					}
					only[rel] = true
				}
			}
			return writeDiff(app, cmd.OutOrStdout(), only)
		},
	}
}

// writeDiff prints a unified diff for every file that isn't clean, or only
// for the files in only when it's non-nil.
func writeDiff(app *appContext, out io.Writer, only map[string]bool) error {
	items, err := app.plan()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Status == engine.Clean || only != nil && !only[it.Rel] {
			continue
		}
		from, to := engine.DiffHeaders(it)
		text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(it.Current)),
			B:        difflib.SplitLines(string(it.Desired)),
			FromFile: from,
			ToFile:   to,
			Context:  3,
		})
		if err != nil {
			return err
		}
		fmt.Fprint(out, text)
	}
	return nil
}

// noFileArgs rejects arguments on commands that always cover every managed
// file (status, apply, sync). Without it Cobra accepts them silently, so
// `strata apply .zshrc` applied everything; its own NoArgs message reads
// as if .zshrc were mistaken for a subcommand.
func noFileArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%s takes no file names: it always covers every managed file (to look at one file: strata diff %s)", cmd.Name(), args[0])
	}
	return nil
}
