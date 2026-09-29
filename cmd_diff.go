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
		Use:   "diff",
		Short: "Show what apply would change, including drift from edits made in $HOME",
		Long: `Unified diff of every non-clean file: home/<file> (what's on disk now)
against repo/<file> (what apply would write).

Because it compares in both directions, edits you made directly in $HOME
show up too - as lines apply would remove. No drift is ever silent.

Each header shows that copy's last-modified time, and the home header says
which copy has the newer edit (drifted: $HOME, update: the repo). That
comes from strata's hashes, not the clock. The times help with a conflict,
where both changed - but a git pull stamps repo files with the pull time,
not the time of the edit.`,
		Example: `  strata diff
  strata diff | less`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			return writeDiff(app, cmd.OutOrStdout())
		},
	}
}

// writeDiff prints a unified diff for every file that isn't clean.
func writeDiff(app *appContext, out io.Writer) error {
	items, err := app.plan()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Status == engine.Clean {
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
