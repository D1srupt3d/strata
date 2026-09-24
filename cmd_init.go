package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"strata/internal/config"
	"strata/internal/fsutil"
	"strata/internal/layers"
)

func newInitCmd() *cobra.Command {
	var repoFlag, dirFlag, layersFlag string
	cmd := &cobra.Command{
		Use:   "init [git-url | local-repo]",
		Short: "Set up this machine: clone (if URL given), choose role layers, write machine.toml, first apply",
		Long: `First-time setup. Writes ~/.config/strata/machine.toml (the only
per-machine state) and runs the first apply.

With a git URL, clones the repo first (default destination ~/dotfiles).
A path to an existing local folder is used in place, like --repo; add
--dir to clone it instead.
The first apply never overwrites existing files it didn't write - it
stops and lists them so you can 'strata add' the keepers and --force the
rest.

Re-running init (say, after moving the repo) replaces repo and layers but
keeps this machine's [vars] overrides. Comments in the old machine.toml
are not kept.

If your repo uses vars, init lists the ones this machine takes from
dots.toml ([vars] defaults, and [layer_vars] values for its layers) and
where each came from; override any of them under [vars] in machine.toml.
It also warns when the repo isn't a git clone, since 'strata sync' needs
one.`,
		Example: `  strata init git@github.com:you/dotfiles.git
  strata init ~/src/dotfiles                     # existing local repo, used in place
  strata init --repo ~/dotfiles --layers work
  strata init --repo ~/dotfiles --layers ""      # no role layers`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolvePaths()
			if err != nil {
				return err
			}
			// Read the overrides to keep before doing anything (cloning
			// included), so an unreadable machine.toml stops init up front.
			keep, err := existingVars(p.Machine)
			if err != nil {
				return err
			}
			repoDir := repoFlag
			// A local folder passed without --dir is the repo itself, not a
			// clone source: `strata init ~/src/dotfiles` used to clone it into
			// ~/dotfiles and point this machine at the copy.
			if len(args) == 1 && dirFlag == "" && isDir(args[0]) {
				repoDir, args = args[0], nil
			}
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
			// A typo'd [layer_vars] section fails here, before the role
			// prompt, like a typo'd role layer does below.
			if err := layers.CheckVarSections(abs, slices.Sorted(maps.Keys(rc.LayerVars))); err != nil {
				return fmt.Errorf("dots.toml: %w", err)
			}

			roles := splitCSV(layersFlag)
			// Changed, not == "": the documented `--layers ""` means "no role
			// layers, don't ask", which an empty-value check can't tell apart
			// from omitting the flag.
			if !cmd.Flags().Changed("layers") {
				fmt.Fprint(cmd.OutOrStdout(), "role layers (comma-separated, e.g. work - empty for none): ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				roles = splitCSV(line)
			}
			// A typo'd layer must fail here, not leave a machine.toml that
			// every later command rejects.
			if err := layers.CheckRoles(abs, roles); err != nil {
				return fmt.Errorf("role layers: %w", err)
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
			if len(keep) > 0 {
				b.WriteString("\n")
				enc := toml.NewEncoder(&b)
				enc.Indent = ""
				if err := enc.Encode(struct {
					Vars map[string]string `toml:"vars"`
				}{keep}); err != nil {
					return err
				}
			}
			if err := fsutil.WriteFileAtomic(p.Machine, []byte(b.String()), 0o644); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "wrote %s\n", p.Machine)
			if len(keep) > 0 {
				fmt.Fprintf(out, "kept %d [vars] override(s) from the previous machine.toml\n", len(keep))
			}
			if !isGitRepo(abs) {
				fmt.Fprintf(out, "warning: %s is not a git repository - apply works, but 'strata sync' (git pull) won't until it's a git clone\n", abs)
			}
			app, err := loadContext()
			if err != nil {
				return err
			}
			// Every var this machine takes from the repo (a [vars] default, or
			// a [layer_vars] value for one of its layers) and where it came
			// from, so picking a layer shows the values it brought along.
			var fromRepo []string
			for n, from := range app.Cfg.VarFrom {
				if from != "machine.toml" {
					fromRepo = append(fromRepo, n)
				}
			}
			if len(fromRepo) > 0 {
				sort.Strings(fromRepo)
				fmt.Fprintf(out, "note: these vars use values from dots.toml on this machine - override any of them under [vars] in %s:\n", p.Machine)
				for _, n := range fromRepo {
					fmt.Fprintf(out, "  %s = %q  (%s)\n", n, app.Cfg.Vars[n], app.Cfg.VarFrom[n])
				}
			}
			return runApply(app, cmd.OutOrStdout(), applyOpts{})
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "use an existing local repo instead of cloning")
	cmd.Flags().StringVar(&dirFlag, "dir", "", "clone destination (default ~/dotfiles)")
	cmd.Flags().StringVar(&layersFlag, "layers", "", "role layers, comma-separated (skips prompt)")
	return cmd
}

// existingVars returns the [vars] of the machine.toml init is about to
// replace, so re-running init keeps this machine's overrides. Only [vars]
// carries over: repo and layers are what init replaces, and a stale or
// invalid key is exactly what re-running init repairs. A file that can't be
// parsed at all is refused rather than overwritten - it may hold the only
// copy of those overrides.
func existingVars(path string) (map[string]string, error) {
	var old struct {
		Vars map[string]string `toml:"vars"`
	}
	if _, err := toml.DecodeFile(path, &old); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s exists but can't be read (%w) - fix or remove it, then rerun init", path, err)
	}
	return old.Vars, nil
}

// isGitRepo reports whether dir is inside a git work tree - where
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

// isDir reports whether path is an existing folder.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
