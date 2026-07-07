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
		Long: `Runs 'git pull --ff-only' in the dotfiles repo, then apply. The
"give me my other machine's latest changes" command.

strata doesn't wrap git beyond this — commit and push in the repo with
git as usual.`,
		Example: `  strata sync`,
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
