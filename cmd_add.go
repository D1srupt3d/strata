package main

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/engine"
	"strata/internal/fsutil"
	"strata/internal/layers"
	"strata/internal/state"
)

// relFromArg turns a user-supplied path (~/.zshrc, .zshrc, /Users/x/.zshrc)
// into a home-relative slash path.
func relFromArg(arg, home string) (string, error) {
	p := arg
	if p == "~" {
		p = home
	} else if strings.HasPrefix(p, "~/") {
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
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add <file|dir>...",
		Short: "Copy files from $HOME into the repo (adopt new files, or absorb local edits)",
		Long: `One command for two jobs:

  adopt   a file strata doesn't manage yet: it lands in base/ (or --layer)
  absorb  edits you made directly in $HOME on a managed file - the drifted
          content becomes the repo content and the file reads clean again

Default target layer is whichever layer currently wins for that file,
else base. If the repo can't be planned (e.g. an undefined {{var}}), add
refuses rather than guess - fix the error, or name the layer with --layer.
Paths may be ~-relative, $HOME-relative, or absolute.

Several files can be added at once, each to its own winning layer (or all
to --layer). Every path is checked before anything is written, so one bad
path means nothing is added.

A directory adds every file under it, each to its own winning layer. The
walk skips ignored files (the same ignore rules apply uses), .git folders
and symlinks, and says how many it skipped. Your home directory itself is
refused, and a directory with nothing to add is an error. Preview a big one
with --dry-run first: it lists each file and its layer, and writes nothing.

--layer must be a folder in the repo or one of this machine's layers. When
that layer isn't the one this machine gets the file from (a later layer
overrides it, or it isn't one of this machine's layers), the copy is saved
there but $HOME is left alone, and add says which layer wins.

If the file uses {{var}} substitution you'll get a warning: the copy you
captured contains the expanded values - restore the {{tokens}} by hand.`,
		Example: `  strata add .vimrc              adopt a new file into base/
  strata add .zshrc              absorb your local .zshrc edits
  strata add .zshrc .vimrc ~/.config/git/*
  strata add -n ~/.config/nvim   preview a whole directory, then drop -n
  strata add .Brewfile --layer mac`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			order := app.order()

			var items []engine.Item
			if layer != "" {
				if err := checkTargetLayer(app.Cfg.RepoDir, layer, order); err != nil {
					return err
				}
			} else {
				// The target is the layer that wins for each file. If planning
				// fails we can't know it, and guessing base/ could push a
				// work-only file to every machine - refuse instead.
				if items, err = app.plan(); err != nil {
					return fmt.Errorf("can't tell which layer %s belongs in: %w (fix that, or pass --layer)", strings.Join(args, ", "), err)
				}
			}

			// First pass: check every file before writing any, so one bad
			// argument can't leave the others half-added.
			type addFile struct {
				rel, target string
				content     []byte
				mode        os.FileMode
			}
			var rels, notes []string
			for _, arg := range args {
				rel, err := relFromArg(arg, app.Paths.Home)
				if err != nil {
					return err
				}
				if rel == "." {
					return fmt.Errorf("refusing to add the home directory itself (%s) - it holds caches, keys and tokens; name the files or folders you want", app.Paths.Home)
				}
				info, err := os.Stat(filepath.Join(app.Paths.Home, filepath.FromSlash(rel)))
				if err != nil {
					return err
				}
				if !info.IsDir() {
					rels = append(rels, rel)
					continue
				}
				found, skipped, err := filesUnder(app.Paths.Home, rel, app.Cfg.Ignore)
				if err != nil {
					return err
				}
				if len(found) == 0 {
					return fmt.Errorf("nothing to add under %s: no files, or only ones strata skips (ignore patterns, .git folders, symlinks)", rel)
				}
				if skipped > 0 {
					notes = append(notes, fmt.Sprintf("note: skipped %d under %s (ignore patterns, .git folders and symlinks aren't added)", skipped, rel))
				}
				rels = append(rels, found...)
			}
			var files []addFile
			seen := map[string]bool{}
			for _, rel := range rels {
				if seen[rel] {
					continue
				}
				seen[rel] = true
				homePath := filepath.Join(app.Paths.Home, filepath.FromSlash(rel))
				content, err := os.ReadFile(homePath)
				if err != nil {
					return err
				}
				info, err := os.Stat(homePath)
				if err != nil {
					return err
				}
				target := layer
				if target == "" {
					target = "base"
					for _, it := range items {
						if it.Rel == rel { // winning layer = first path element under repo
							if l, err := filepath.Rel(app.Cfg.RepoDir, it.Source); err == nil {
								target = strings.Split(filepath.ToSlash(l), "/")[0]
							}
						}
					}
				}
				files = append(files, addFile{rel, target, content, info.Mode().Perm()})
			}

			// Second pass: write. Only when target is the layer this machine
			// gets rel from do $HOME and the layers agree, so only then is the
			// $HOME copy recorded as applied. Recording it otherwise made the
			// next apply "update" $HOME back to the winning layer's copy - or,
			// for a layer this machine doesn't use, delete the file as removed.
			out := cmd.OutOrStdout()
			for _, n := range notes {
				fmt.Fprintln(out, n)
			}
			record := map[string]string{}
			var writeErr error
			for _, f := range files {
				for _, s := range app.Cfg.Substitute {
					if s == f.rel {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"warning: %s uses {{var}} substitution - you just captured the EXPANDED values; restore the {{tokens}} by hand (strata edit %s)\n", f.rel, f.rel)
					}
				}
				if dryRun {
					fmt.Fprintf(out, "would add %s → %s/%s\n", f.rel, f.target, f.rel)
					continue
				}
				dest := filepath.Join(app.Cfg.RepoDir, f.target, filepath.FromSlash(f.rel))
				if writeErr = fsutil.WriteFileAtomic(dest, f.content, f.mode); writeErr != nil {
					break
				}
				fmt.Fprintf(out, "added %s → %s/%s\n", f.rel, f.target, f.rel)

				winner, err := winningLayer(app.Cfg.RepoDir, f.rel, order, app.Cfg.Ignore)
				if err != nil {
					writeErr = err
					break
				}
				switch {
				case winner == f.target:
					record[f.rel] = fsutil.Hash(f.content)
				case winner != "":
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: %s/%s wins on this machine, so $HOME keeps getting that copy and your $HOME file is left as it is - to use it here too: strata add %s --layer %s\n",
						winner, f.rel, f.rel, winner)
				case !slices.Contains(order, f.target):
					fmt.Fprintf(out, "note: %s isn't one of this machine's layers (%s), so strata doesn't manage %s here - the $HOME copy is left as it is\n",
						f.target, strings.Join(order, ", "), f.rel)
				default:
					fmt.Fprintf(out, "note: %s matches an ignore pattern, so strata doesn't manage it\n", f.rel)
				}
			}

			// Record what was written even when a later write failed (as
			// runApply does), then return that error.
			if len(record) > 0 {
				unlock, err := state.Lock(app.Paths.State)
				if err != nil {
					return err
				}
				defer unlock()
				st, err := state.Load(app.Paths.State) // fresh copy under the lock
				if err != nil {
					return err
				}
				maps.Copy(st.Files, record)
				if err := st.Save(app.Paths.State); err != nil {
					return err
				}
			}
			return writeErr
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be added, and where, without writing")
	cmd.Flags().StringVar(&layer, "layer", "", "target layer: a folder in the repo or one of this machine's layers (default: winning layer, else base)")
	return cmd
}

// filesUnder lists the regular files under the home-relative directory dir,
// as home-relative slash paths. It skips what apply would never manage
// (ignore patterns), .git folders (git won't commit them, so they'd exist in
// this machine's repo only) and symlinks (not followed: they can point
// outside $HOME or loop), and returns how many entries it skipped.
func filesUnder(home, dir string, ignore []string) (files []string, skipped int, err error) {
	err = filepath.WalkDir(filepath.Join(home, filepath.FromSlash(dir)), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				skipped++
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			skipped++
			return nil
		}
		rel, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if ignored, err := layers.Ignored(rel, ignore); err != nil {
			return err
		} else if ignored {
			skipped++
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, skipped, err
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
