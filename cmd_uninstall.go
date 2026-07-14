package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/fsutil"
)

// rcMarker is the comment install.sh writes above the PATH line it appends,
// so uninstall can find and remove exactly what the installer added.
const rcMarker = "# added by strata install.sh"

func newUninstallCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove strata itself: the binary, machine.toml, state.json, and the installer's PATH line",
		Long: `Removes everything strata put on this machine:

  - the strata binary (the one you're running)
  - ~/.config/strata/machine.toml   (this machine's config)
  - ~/.local/state/strata/state.json (strata's memory of what it wrote)
  - the "export PATH" line install.sh appended to your shell rc

It does NOT touch the dotfiles strata copied into $HOME (.zshrc, .gitconfig,
and the rest). Those are just normal files now and stay exactly as they are.
It also leaves your dotfiles repo alone — delete that yourself if you want it gone.

Prompts for confirmation first; pass --yes to skip, or --dry-run to preview.`,
		Example: `  strata uninstall            list what will go, ask, then remove
  strata uninstall --dry-run  preview only, remove nothing
  strata uninstall --yes      remove without asking`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolvePaths()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// Gather what actually exists so we only list real targets.
			var files []string   // files to delete
			var rcFiles []string // shell rc files that carry the installer's PATH line
			for _, f := range []string{p.State, p.Machine, p.Bin} {
				if f == "" {
					continue
				}
				if _, err := os.Stat(f); err == nil {
					files = append(files, f)
				}
			}
			for _, name := range []string{".zshrc", ".bashrc", ".profile"} {
				rc := filepath.Join(p.Home, name)
				if data, err := os.ReadFile(rc); err == nil && strings.Contains(string(data), rcMarker) {
					rcFiles = append(rcFiles, rc)
				}
			}

			if len(files) == 0 && len(rcFiles) == 0 {
				fmt.Fprintln(out, "nothing to remove — strata isn't installed here")
				return nil
			}

			fmt.Fprintln(out, "This will remove:")
			for _, f := range files {
				fmt.Fprintf(out, "  %s\n", f)
			}
			for _, rc := range rcFiles {
				fmt.Fprintf(out, "  PATH line in %s\n", rc)
			}
			fmt.Fprintln(out, "\nYour dotfiles in $HOME and your dotfiles repo are left untouched.")

			if dryRun {
				fmt.Fprintln(out, "\n(dry run — nothing removed)")
				return nil
			}
			if !yes {
				fmt.Fprint(out, "\nRemove these? [y/N]: ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
					fmt.Fprintln(out, "aborted")
					return nil
				}
			}

			// Best-effort: keep going if one step fails, report at the end.
			// The binary is deleted last so a mid-run failure still leaves a
			// working binary to retry with.
			var failed []string
			for _, f := range orderBinLast(files, p.Bin) {
				if err := os.Remove(f); err != nil {
					fmt.Fprintf(out, "could not remove %s: %v\n", f, err)
					failed = append(failed, f)
					continue
				}
				fmt.Fprintf(out, "removed %s\n", f)
				// Clean up now-empty strata dirs (e.g. ~/.config/strata).
				if f == p.State || f == p.Machine {
					_ = os.Remove(filepath.Dir(f)) // no-op if not empty
				}
			}
			for _, rc := range rcFiles {
				if err := stripRCLine(rc); err != nil {
					fmt.Fprintf(out, "could not clean %s: %v\n", rc, err)
					failed = append(failed, rc)
					continue
				}
				fmt.Fprintf(out, "cleaned PATH line from %s\n", rc)
			}

			if len(failed) > 0 {
				return fmt.Errorf("uninstall finished with %d item(s) needing manual removal", len(failed))
			}
			fmt.Fprintln(out, "\nstrata uninstalled. Open a new terminal (or run 'hash -r') to clear the cached command.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be removed without removing it")
	return cmd
}

// orderBinLast returns files with bin moved to the end, if present, so the
// running binary is deleted only after config/state are cleaned up.
func orderBinLast(files []string, bin string) []string {
	if bin == "" {
		return files
	}
	out := make([]string, 0, len(files))
	found := false
	for _, f := range files {
		if f == bin {
			found = true
			continue
		}
		out = append(out, f)
	}
	if found {
		out = append(out, bin)
	}
	return out
}

// stripRCLine removes the installer's marker comment and the PATH export line
// directly below it, plus one blank line the installer left above the marker.
func stripRCLine(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == rcMarker {
			// Drop a single blank line the installer prepended, if present.
			if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
				out = out[:n-1]
			}
			// Drop the export line that follows the marker.
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "export PATH=") {
				i++
			}
			continue
		}
		out = append(out, lines[i])
	}
	return fsutil.WriteFileAtomic(path, []byte(strings.Join(out, "\n")), mode)
}
