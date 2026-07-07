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
