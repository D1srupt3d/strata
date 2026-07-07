package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/fsutil"
)

func newInitCmd() *cobra.Command {
	var repoFlag, dirFlag, layersFlag string
	cmd := &cobra.Command{
		Use:   "init [git-url]",
		Short: "Set up this machine: clone (if URL given), choose role layers, write machine.toml, first apply",
		Long: `First-time setup. Writes ~/.config/strata/machine.toml (the only
per-machine state) and runs the first apply.

With a git URL, clones the repo first (default destination ~/dotfiles).
The first apply never overwrites existing files it didn't write — it
stops and lists them so you can 'strata add' the keepers and --force the
rest.

If your repo uses [vars], add per-machine overrides to machine.toml
afterwards under a [vars] section.`,
		Example: `  strata init git@github.com:you/dotfiles.git
  strata init --repo ~/dotfiles --layers work
  strata init --repo ~/dotfiles --layers ""      # no role layers`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolvePaths()
			if err != nil {
				return err
			}
			repoDir := repoFlag
			if len(args) == 1 { // clone mode
				repoDir = dirFlag
				if repoDir == "" {
					repoDir = filepath.Join(p.Home, "dotfiles")
				}
				clone := exec.Command("git", "clone", args[0], repoDir)
				clone.Stdout, clone.Stderr = os.Stdout, os.Stderr
				if err := clone.Run(); err != nil {
					return fmt.Errorf("git clone failed: %w", err)
				}
			}
			if repoDir == "" {
				return fmt.Errorf("either pass a git URL or --repo /path/to/existing/repo")
			}
			if _, err := os.Stat(repoDir); err != nil {
				return fmt.Errorf("repo dir: %w", err)
			}
			abs, err := filepath.Abs(repoDir)
			if err != nil {
				return err
			}

			roles := splitCSV(layersFlag)
			if layersFlag == "" {
				fmt.Fprint(cmd.OutOrStdout(), "role layers (comma-separated, e.g. work — empty for none): ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				roles = splitCSV(line)
			}

			var b strings.Builder
			fmt.Fprintf(&b, "repo = %q\n", filepath.ToSlash(abs))
			b.WriteString("layers = [")
			for i, r := range roles {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%q", r)
			}
			b.WriteString("]\n")
			if err := fsutil.WriteFileAtomic(p.Machine, []byte(b.String()), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", p.Machine)

			apply := newApplyCmd()
			apply.SetOut(cmd.OutOrStdout())
			return apply.RunE(apply, nil)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "use an existing local repo instead of cloning")
	cmd.Flags().StringVar(&dirFlag, "dir", "", "clone destination (default ~/dotfiles)")
	cmd.Flags().StringVar(&layersFlag, "layers", "", "role layers, comma-separated (skips prompt)")
	return cmd
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
