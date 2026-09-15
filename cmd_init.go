package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/config"
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

If your repo uses [vars], init lists the ones running on their dots.toml
defaults; override any of them per machine under [vars] in machine.toml.
It also warns when the repo isn't a git clone, since 'strata sync' needs
one.`,
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
				clone.Stdout, clone.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
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
			// Validate the repo's config before writing anything for this
			// machine: a broken dots.toml should fail here, not mid-apply.
			rc, err := config.LoadRepoConfig(abs)
			if err != nil {
				return err
			}

			roles := splitCSV(layersFlag)
			// Changed, not == "": the documented `--layers ""` means "no role
			// layers, don't ask", which an empty-value check can't tell apart
			// from omitting the flag.
			if !cmd.Flags().Changed("layers") {
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
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "wrote %s\n", p.Machine)
			if !isGitRepo(abs) {
				fmt.Fprintf(out, "warning: %s is not a git repository — apply works, but 'strata sync' (git pull) won't until it's a git clone\n", abs)
			}
			if len(rc.Vars) > 0 {
				names := make([]string, 0, len(rc.Vars))
				for n := range rc.Vars {
					names = append(names, n)
				}
				sort.Strings(names)
				fmt.Fprintf(out, "note: these vars use their dots.toml defaults on this machine — override any of them under [vars] in %s:\n", p.Machine)
				for _, n := range names {
					fmt.Fprintf(out, "  %s = %q\n", n, rc.Vars[n])
				}
			}

			app, err := loadContext()
			if err != nil {
				return err
			}
			return runApply(app, cmd.OutOrStdout(), applyOpts{})
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "use an existing local repo instead of cloning")
	cmd.Flags().StringVar(&dirFlag, "dir", "", "clone destination (default ~/dotfiles)")
	cmd.Flags().StringVar(&layersFlag, "layers", "", "role layers, comma-separated (skips prompt)")
	return cmd
}

// isGitRepo reports whether dir is inside a git work tree — where
// 'strata sync' can git pull.
func isGitRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Run() == nil
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
