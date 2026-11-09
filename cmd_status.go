package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"strata/internal/engine"
)

// statusReport is the shape of `status --json`.
type statusReport struct {
	Clean        bool         `json:"clean"`
	Files        []statusFile `json:"files"`
	PendingHooks []string     `json:"pending_hooks"`
}

type statusFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Layer  string `json:"layer,omitempty"` // empty for removed files
}

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "One line per managed file: clean / create / update / drifted / conflict / unmanaged",
		Long: `Shows every managed file that needs attention (clean files are summarized).

Statuses:
  create     file doesn't exist in $HOME yet - apply will write it
  update     repo changed, you haven't touched the $HOME copy - apply writes it
  drifted    you edited the $HOME copy - apply refuses; keep it with 'strata add'
  conflict   repo AND $HOME both changed - inspect with 'strata diff', pick a side
  unmanaged  file exists but strata never wrote it - first-apply protection
  removed    no layer provides it anymore - apply keeps it; 'apply --prune' deletes it
  chmod      content matches but the file mode doesn't - apply fixes the mode
  hook       a hook failed or was interrupted - apply retries it
  clean      $HOME matches the repo-built content

Exit status: 0 when everything is clean, 1 when anything needs attention.

--json prints every file, clean ones included, with the layer it comes from:
  {"clean": false, "files": [{"path": ".zshrc", "status": "drifted", "layer": "base"}],
   "pending_hooks": []}`,
		Example: `  strata status
  strata status || strata diff   # show the diff only when something needs attention
  strata status --json | jq -r '.files[] | select(.status != "clean") | .path'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadContext()
			if err != nil {
				return err
			}
			items, err := app.plan()
			if err != nil {
				return err
			}
			var hooks []string
			for _, rel := range app.State.PendingHooks {
				if _, ok := app.Cfg.Hooks[rel]; ok { // hook removed from dots.toml: apply will drop it
					hooks = append(hooks, rel)
				}
			}
			if asJSON {
				return printStatusJSON(cmd.OutOrStdout(), app.Cfg.RepoDir, items, hooks)
			}
			dirty := len(hooks)
			for _, it := range items {
				if it.Status == engine.Clean {
					continue
				}
				dirty++
				fmt.Fprintf(cmd.OutOrStdout(), "%-9s %s\n", it.Status, it.Rel)
			}
			for _, rel := range hooks {
				fmt.Fprintf(cmd.OutOrStdout(), "%-9s %s (pending: failed or interrupted; apply retries it)\n", "hook", rel)
			}
			if dirty > 0 {
				return exitCode(1)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "clean: %d files up to date\n", len(items))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print every file as JSON, for scripts")
	return cmd
}

// printStatusJSON writes every item (clean ones too) and the pending hooks,
// and exits 1 under the same rule as the text output.
func printStatusJSON(w io.Writer, repoDir string, items []engine.Item, hooks []string) error {
	// Empty lists, not null, so `jq '.files | length'` just works.
	r := statusReport{Clean: len(hooks) == 0, Files: []statusFile{}, PendingHooks: append([]string{}, hooks...)}
	for _, it := range items {
		f := statusFile{Path: it.Rel, Status: it.Status.String()}
		if rel, err := filepath.Rel(repoDir, it.Source); it.Source != "" && err == nil {
			f.Layer = strings.Split(filepath.ToSlash(rel), "/")[0]
		}
		if it.Status != engine.Clean {
			r.Clean = false
		}
		r.Files = append(r.Files, f)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if !r.Clean {
		return exitCode(1)
	}
	return nil
}
