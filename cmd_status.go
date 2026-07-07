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
  create     file doesn't exist in $HOME yet — apply will write it
  update     repo changed, you haven't touched the $HOME copy — apply writes it
  drifted    you edited the $HOME copy — apply refuses; keep it with 'strata add'
  conflict   repo AND $HOME both changed — inspect with 'strata diff', pick a side
  unmanaged  file exists but strata never wrote it — first-apply protection
  clean      $HOME matches the repo-built content`,
		Example: `  strata status
  strata status && strata diff   # see what the non-clean files would change`,
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
			if dirty == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "clean: %d files up to date\n", len(items))
			}
			return nil
		},
	}
}
