package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/fsutil"
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
	if err != nil || strings.HasPrefix(rel, "..") {
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
  absorb  edits you made directly in $HOME on a managed file — the drifted
          content becomes the repo content and the file reads clean again

Default target layer is whichever layer currently wins for that file,
else base. Paths may be ~-relative, $HOME-relative, or absolute.

If the file uses {{var}} substitution you'll get a warning: the copy you
captured contains the expanded values — restore the {{tokens}} by hand.`,
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

			target := layer
			if target == "" {
				target = "base"
				if items, err := app.plan(); err == nil {
					for _, it := range items {
						if it.Rel == rel { // winning layer = first path element under repo
							if l, err := filepath.Rel(app.Cfg.RepoDir, it.Source); err == nil {
								target = strings.Split(filepath.ToSlash(l), "/")[0]
							}
						}
					}
				}
			}
			for _, s := range app.Cfg.Substitute {
				if s == rel {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: %s uses {{var}} substitution — you just captured the EXPANDED values; restore the {{tokens}} by hand (strata edit %s)\n", rel, rel)
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
			app.State.Files[rel] = fsutil.Hash(content)
			if err := app.State.Save(app.Paths.State); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s → %s/%s\n", rel, target, rel)
			return nil
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", "target layer (default: winning layer, else base)")
	return cmd
}
