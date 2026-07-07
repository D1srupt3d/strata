package main

import (
	"fmt"

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
show up too — as lines apply would remove. No drift is ever silent.`,
		Example: `  strata diff
  strata diff | less`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			for _, it := range items {
				if it.Status == engine.Clean {
					continue
				}
				text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
					A:        difflib.SplitLines(string(it.Current)),
					B:        difflib.SplitLines(string(it.Desired)),
					FromFile: "home/" + it.Rel + " (" + it.Status.String() + ")",
					ToFile:   "repo/" + it.Rel,
					Context:  3,
				})
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), text)
			}
			return nil
		},
	}
}
