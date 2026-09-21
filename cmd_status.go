package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"strata/internal/engine"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "One line per managed file: clean / create / update / drifted / conflict / unmanaged",
		Long: `Shows every managed file that needs attention (clean files are summarized).

Statuses:
  create     file doesn't exist in $HOME yet - apply will write it
  update     repo changed, you haven't touched the $HOME copy - apply writes it
  drifted    you edited the $HOME copy - apply refuses; keep it with 'strata add'
  conflict   repo AND $HOME both changed - inspect with 'strata diff', pick a side
  unmanaged  file exists but strata never wrote it - first-apply protection
  removed    no layer provides it anymore - apply deletes it from $HOME
  chmod      content matches but the file mode doesn't - apply fixes the mode
  hook       a hook failed or was interrupted - apply retries it
  clean      $HOME matches the repo-built content

Exit status: 0 when everything is clean, 1 when anything needs attention.`,
		Example: `  strata status
  strata status || strata diff   # show the diff only when something needs attention`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			dirty := 0
			for _, it := range items {
				if it.Status == engine.Clean {
					continue
				}
				dirty++
				fmt.Fprintf(cmd.OutOrStdout(), "%-9s %s\n", it.Status, it.Rel)
			}
			for _, rel := range app.State.PendingHooks {
				if _, ok := app.Cfg.Hooks[rel]; !ok {
					continue // hook removed from dots.toml; apply will drop it
				}
				dirty++
				fmt.Fprintf(cmd.OutOrStdout(), "%-9s %s (pending: failed or interrupted; apply retries it)\n", "hook", rel)
			}
			if dirty > 0 {
				return exitCode(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "clean: %d files up to date\n", len(items))
			return nil
		},
	}
}
