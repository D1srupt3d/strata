package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"strata/internal/engine"
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
wins again and the $HOME copy is rewritten to it instead.

If apply would refuse right now (a drifted or conflicting file anywhere),
rm refuses before deleting anything, so it never stops half done.`,
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
			var blocked []engine.Item
			for _, it := range items {
				if it.Rel == rel {
					source = it.Source
				}
				if it.Blocked(app.State) {
					blocked = append(blocked, it)
				}
			}
			if source == "" {
				return fmt.Errorf("%s is not managed by any layer on this machine", rel)
			}
			// rm is delete-then-apply. If that apply is going to refuse, the
			// delete must not happen either: rm would stop half done, with the
			// source gone from the repo and $HOME untouched.
			if len(blocked) > 0 {
				return fmt.Errorf("not removing %s: the apply that follows would refuse these local changes:%s\nresolve them first ('strata add <file>' keeps yours, 'strata apply --force' takes the repo's), then run strata rm again",
					rel, engine.BlockedList(blocked))
			}
			if err := os.Remove(source); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", source)
			return runApply(app, cmd.OutOrStdout(), applyOpts{})
		},
	}
}
