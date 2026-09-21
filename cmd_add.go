package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/fsutil"
	"strata/internal/layers"
	"strata/internal/state"
)

// relFromArg turns a user-supplied path (~/.zshrc, .zshrc, /Users/x/.zshrc)
// into a home-relative slash path.
func relFromArg(arg, home string) (string, error) {
	p := arg
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(home, p)
	}
	rel, err := filepath.Rel(home, p)
	// IsLocal rather than a ".." prefix check: "..weird" is a file name.
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is not inside the home directory %s", arg, home)
	}
	return filepath.ToSlash(rel), nil
}

func newAddCmd() *cobra.Command {
	var layer string
	cmd := &cobra.Command{
		Use:   "add <file>",
		Short: "Copy a file from $HOME into the repo (adopt a new file, or absorb local edits)",
		Long: `One command for two jobs:

  adopt   a file strata doesn't manage yet: it lands in base/ (or --layer)
  absorb  edits you made directly in $HOME on a managed file - the drifted
          content becomes the repo content and the file reads clean again

Default target layer is whichever layer currently wins for that file,
else base. If the repo can't be planned (e.g. an undefined {{var}}), add
refuses rather than guess - fix the error, or name the layer with --layer.
Paths may be ~-relative, $HOME-relative, or absolute.

--layer must be a folder in the repo or one of this machine's layers. When
that layer isn't the one this machine gets the file from (a later layer
overrides it, or it isn't one of this machine's layers), the copy is saved
there but $HOME is left alone, and add says which layer wins.

If the file uses {{var}} substitution you'll get a warning: the copy you
captured contains the expanded values - restore the {{tokens}} by hand.`,
		Example: `  strata add .vimrc              adopt a new file into base/
  strata add .zshrc              absorb your local .zshrc edits
  strata add .Brewfile --layer mac`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			rel, err := relFromArg(args[0], app.Paths.Home)
			if err != nil {
				return err
			}
			homePath := filepath.Join(app.Paths.Home, filepath.FromSlash(rel))
			content, err := os.ReadFile(homePath)
			if err != nil {
				return err
			}
			order := app.order()

			target := layer
			if target != "" {
				if err := checkTargetLayer(app.Cfg.RepoDir, target, order); err != nil {
					return err
				}
			} else {
				// The target is the layer that wins for rel. If planning fails we
				// can't know it, and guessing base/ could push a work-only file
				// to every machine - refuse instead.
				items, err := app.plan()
				if err != nil {
					return fmt.Errorf("can't tell which layer %s belongs in: %w (fix that, or pass --layer)", rel, err)
				}
				target = "base"
				for _, it := range items {
					if it.Rel == rel { // winning layer = first path element under repo
						if l, err := filepath.Rel(app.Cfg.RepoDir, it.Source); err == nil {
							target = strings.Split(filepath.ToSlash(l), "/")[0]
						}
					}
				}
			}
			for _, s := range app.Cfg.Substitute {
				if s == rel {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: %s uses {{var}} substitution - you just captured the EXPANDED values; restore the {{tokens}} by hand (strata edit %s)\n", rel, rel)
				}
			}
			info, err := os.Stat(homePath)
			if err != nil {
				return err
			}
			dest := filepath.Join(app.Cfg.RepoDir, target, filepath.FromSlash(rel))
			if err := fsutil.WriteFileAtomic(dest, content, info.Mode().Perm()); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "added %s → %s/%s\n", rel, target, rel)

			// Only when target is the layer this machine gets rel from do
			// $HOME and the layers agree, so only then is the $HOME copy
			// recorded as applied. Recording it otherwise made the next apply
			// "update" $HOME back to the winning layer's copy - or, for a
			// layer this machine doesn't use, delete the file as removed.
			winner, err := winningLayer(app.Cfg.RepoDir, rel, order, app.Cfg.Ignore)
			if err != nil {
				return err
			}
			switch {
			case winner == target:
				unlock, err := state.Lock(app.Paths.State)
				if err != nil {
					return err
				}
				defer unlock()
				st, err := state.Load(app.Paths.State) // fresh copy under the lock
				if err != nil {
					return err
				}
				st.Files[rel] = fsutil.Hash(content)
				return st.Save(app.Paths.State)
			case winner != "":
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: %s/%s wins on this machine, so $HOME keeps getting that copy and your $HOME file is left as it is - to use it here too: strata add %s --layer %s\n",
					winner, rel, rel, winner)
			case !slices.Contains(order, target):
				fmt.Fprintf(out, "note: %s isn't one of this machine's layers (%s), so strata doesn't manage %s here - the $HOME copy is left as it is\n",
					target, strings.Join(order, ", "), rel)
			default:
				fmt.Fprintf(out, "note: %s matches an ignore pattern, so strata doesn't manage it\n", rel)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", "target layer: a folder in the repo or one of this machine's layers (default: winning layer, else base)")
	return cmd
}

// checkTargetLayer rejects a --layer that is neither a folder in the repo
// nor one of this machine's layers: that's a typo, and add used to create a
// brand-new layer folder for it. A layer of this machine's own stack may
// not have a folder yet - add is how its first file gets there.
func checkTargetLayer(repoDir, target string, order []string) error {
	if err := layers.ValidName(target); err != nil {
		return err
	}
	if slices.Contains(order, target) {
		return nil
	}
	if info, err := os.Stat(filepath.Join(repoDir, target)); err == nil && info.IsDir() {
		return nil
	}
	return fmt.Errorf("no layer %q: it isn't a folder in %s, and this machine's layers are %s - typo? (to start a new layer, create its folder first)",
		target, repoDir, strings.Join(order, ", "))
}

// winningLayer returns the layer this machine gets rel from - the last one
// in order that holds it - or "" if none does or rel is ignored. It reads
// the layer folders directly, so it works even when the full plan can't be
// built (an undefined {{var}} in some other file).
func winningLayer(repoDir, rel string, order, ignore []string) (string, error) {
	if ignored, err := layers.Ignored(rel, ignore); err != nil || ignored {
		return "", err
	}
	winner := ""
	for _, l := range order {
		if info, err := os.Stat(filepath.Join(repoDir, l, filepath.FromSlash(rel))); err == nil && info.Mode().IsRegular() {
			winner = l
		}
	}
	return winner, nil
}
