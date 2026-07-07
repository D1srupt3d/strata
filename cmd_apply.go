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
		Long: `Builds every managed file (stack layers → substitute {{vars}} → resolve
permissions) and copies the ones that changed into $HOME, then runs hooks
for the files that were written.

Safety rules:
  - If ANY file is drifted/conflicted/unmanaged, apply writes NOTHING and
    lists them — keep your version with 'strata add <file>', or take the
    repo's with --force. All-or-nothing: a half-applied state can't happen.
  - Writes are atomic (temp file + rename); a crash never leaves a
    half-written dotfile.
  - An undefined {{var}} in a substituted file aborts before anything is
    written.`,
		Example: `  strata apply --dry-run    preview without writing
  strata apply              write changes, run hooks
  strata apply --force      also overwrite drifted/conflicting files`,
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
