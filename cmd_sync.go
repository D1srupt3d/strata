package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "git pull the repo, then apply",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			pull := exec.Command("git", "-C", app.Cfg.RepoDir, "pull", "--ff-only")
			pull.Stdout, pull.Stderr = os.Stdout, os.Stderr
			if err := pull.Run(); err != nil {
				return err
			}
			apply := newApplyCmd()
			apply.SetOut(cmd.OutOrStdout())
			return apply.RunE(apply, nil)
		},
	}
}
