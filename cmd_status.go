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
