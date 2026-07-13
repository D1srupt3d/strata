package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <file>",
		Short: "Delete a file from its winning layer, then apply",
		Long: `Deletes the file from the layer that currently wins on this machine,
then runs apply.

If no other layer provides the file, apply removes it from $HOME too
(status 'removed') — refusing first if you'd edited it locally, same as
any overwrite. If an earlier layer still provides the file, that layer
wins again and the $HOME copy is rewritten to it instead.`,
		Example: `  strata rm .tmux.conf        stop managing it AND remove it from $HOME
  strata rm .gitconfig        drop the work override; base wins again`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			rel, err := relFromArg(args[0], app.Paths.Home)
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			source := ""
			for _, it := range items {
				if it.Rel == rel {
					source = it.Source
				}
			}
			if source == "" {
				return fmt.Errorf("%s is not managed by any layer on this machine", rel)
			}
			if err := os.Remove(source); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", source)
			apply := newApplyCmd()
			apply.SetOut(cmd.OutOrStdout())
			return apply.RunE(apply, nil)
		},
	}
}
