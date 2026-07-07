package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"strata/internal/engine"
)

func newApplyCmd() *cobra.Command {
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Build files from layers + vars and copy changes into $HOME, then run hooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			if dryRun {
				for _, it := range items {
					if it.Status != engine.Clean {
						fmt.Fprintf(cmd.OutOrStdout(), "would write %-9s %s\n", it.Status, it.Rel)
					}
				}
				return nil
			}
			res, err := engine.Apply(items, app.Paths.Home, &app.State, force)
			if err != nil {
				return err
			}
			if err := app.State.Save(app.Paths.State); err != nil {
				return err
			}
			for _, rel := range res.Written {
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", rel)
			}
			if len(res.Written) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing to do")
			}
			return engine.RunHooks(app.Cfg.Hooks, res.Written, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite drifted/conflicting/unmanaged files")
	return cmd
}
