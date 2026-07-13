// Package engine computes what apply would do (Plan) and does it (Apply).
package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"strata/internal/config"
	"strata/internal/fsutil"
	"strata/internal/layers"
	"strata/internal/perms"
	"strata/internal/state"
	"strata/internal/subst"
)

type FileStatus int

const (
	Clean FileStatus = iota
	Create
	Update    // repo changed, home untouched since last apply
	Drifted   // home edited, repo unchanged
	Conflict  // both changed
	Unmanaged // existing file strata never wrote, and it differs
	Removed   // strata wrote it before, but no layer provides it anymore
)

func (s FileStatus) String() string {
	return [...]string{"clean", "create", "update", "drifted", "conflict", "unmanaged", "removed"}[s]
}

type Item struct {
	Rel     string
	Source  string // winning layer file (absolute)
	Desired []byte
	Mode    os.FileMode
	Current []byte // nil if the file doesn't exist in home
	Status  FileStatus
}

func inList(s string, list []string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Plan builds the desired file set and classifies every managed path.
// goos/osRelease are parameters so tests can simulate any platform.
func Plan(cfg config.Config, homeDir string, st state.State, goos, osRelease string) ([]Item, error) {
	order := layers.Order(cfg.RoleLayers, goos, osRelease)
	sources, err := layers.Resolve(cfg.RepoDir, order)
	if err != nil {
		return nil, err
	}
	rels := make([]string, 0, len(sources))
	for rel := range sources {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	items := make([]Item, 0, len(rels))
	for _, rel := range rels {
		src := sources[rel]
		desired, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		if inList(rel, cfg.Substitute) {
			if desired, err = subst.Apply(desired, cfg.Vars); err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
		}
		srcInfo, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		mode, err := perms.ModeFor(rel, srcInfo.Mode(), cfg.Permissions)
		if err != nil {
			return nil, err
		}

		it := Item{Rel: rel, Source: src, Desired: desired, Mode: mode}
		current, err := os.ReadFile(filepath.Join(homeDir, filepath.FromSlash(rel)))
		last, tracked := st.Files[rel]
		switch {
		case os.IsNotExist(err):
			it.Status = Create
		case err != nil:
			return nil, err
		default:
			it.Current = current
			switch {
			case string(current) == string(desired):
				it.Status = Clean
			case !tracked:
				it.Status = Unmanaged
			case fsutil.Hash(current) == last:
				it.Status = Update
			case fsutil.Hash(desired) == last:
				it.Status = Drifted
			default:
				it.Status = Conflict
			}
		}
		items = append(items, it)
	}

	// Files strata previously wrote that no longer exist in any layer.
	var gone []string
	for rel := range st.Files {
		if _, ok := sources[rel]; !ok {
			gone = append(gone, rel)
		}
	}
	sort.Strings(gone)
	for _, rel := range gone {
		it := Item{Rel: rel, Status: Removed}
		current, err := os.ReadFile(filepath.Join(homeDir, filepath.FromSlash(rel)))
		switch {
		case err == nil:
			it.Current = current
		case !os.IsNotExist(err):
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

type ApplyResult struct {
	Written []string // rels actually written
	Deleted []string // rels deleted from $HOME (file left every layer)
	Blocked []Item   // changes refused (drift/conflict/unmanaged/edited-removal)
}

// removedEdited reports whether a Removed item's $HOME copy was hand-edited
// after the last apply — deleting it would destroy those edits.
func removedEdited(it Item, st *state.State) bool {
	return it.Status == Removed && it.Current != nil && fsutil.Hash(it.Current) != st.Files[it.Rel]
}

// Apply writes Create/Update items and deletes Removed ones. If any
// Drifted/Conflict/Unmanaged items exist — or a Removed file was locally
// edited — and force is false, it changes NOTHING and returns an error;
// resolve with 'strata add <file>' (keep home) or --force (keep repo).
func Apply(items []Item, homeDir string, st *state.State, force bool) (ApplyResult, error) {
	var res ApplyResult
	for _, it := range items {
		if it.Status == Drifted || it.Status == Conflict || it.Status == Unmanaged || removedEdited(it, st) {
			res.Blocked = append(res.Blocked, it)
		}
	}
	if len(res.Blocked) > 0 && !force {
		names := ""
		for _, it := range res.Blocked {
			names += fmt.Sprintf("\n  %-9s %s", it.Status, it.Rel)
		}
		return res, fmt.Errorf("refusing to overwrite local changes:%s\nkeep your version with 'strata add <file>', or overwrite with 'strata apply --force'", names)
	}
	for _, it := range items {
		if it.Status == Removed {
			if it.Current != nil {
				if err := os.Remove(filepath.Join(homeDir, filepath.FromSlash(it.Rel))); err != nil && !os.IsNotExist(err) {
					return res, fmt.Errorf("removing %s: %w", it.Rel, err)
				}
				res.Deleted = append(res.Deleted, it.Rel)
			}
			delete(st.Files, it.Rel) // stale entries clean up even if already gone
			continue
		}
		write := it.Status == Create || it.Status == Update ||
			(force && (it.Status == Drifted || it.Status == Conflict || it.Status == Unmanaged))
		if write {
			dest := filepath.Join(homeDir, filepath.FromSlash(it.Rel))
			if err := fsutil.WriteFileAtomic(dest, it.Desired, it.Mode); err != nil {
				return res, fmt.Errorf("writing %s: %w", it.Rel, err)
			}
			res.Written = append(res.Written, it.Rel)
		}
		if write || it.Status == Clean {
			st.Files[it.Rel] = fsutil.Hash(it.Desired) // adopt Clean files into state
		}
	}
	return res, nil
}

// RunHooks runs each hook whose file is among the changed rels, in sorted
// rel order, after all writes succeeded. Hook commands come from the user's
// own dots.toml and are deliberately run through the shell (like git hooks).
func RunHooks(hooks map[string]string, changed []string, out io.Writer) error {
	sorted := append([]string(nil), changed...)
	sort.Strings(sorted)
	for _, rel := range sorted {
		cmdStr, ok := hooks[rel]
		if !ok {
			continue
		}
		fmt.Fprintf(out, "hook [%s]: %s\n", rel, cmdStr)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook for %s failed: %w", rel, err)
		}
	}
	return nil
}
