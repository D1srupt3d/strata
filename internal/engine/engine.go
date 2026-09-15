// Package engine computes what apply would do (Plan) and does it (Apply).
package engine

import (
	"bytes"
	"errors"
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
	Chmod     // content matches, but the file mode doesn't
)

func (s FileStatus) String() string {
	return [...]string{"clean", "create", "update", "drifted", "conflict", "unmanaged", "removed", "chmod"}[s]
}

type Item struct {
	Rel     string
	Source  string // winning layer file (absolute)
	Desired []byte
	Mode    os.FileMode
	Current []byte // nil if the file doesn't exist in home
	Status  FileStatus
	Symlink bool // the $HOME path is a symlink: replacing it needs --force
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
	sources, err := layers.Resolve(cfg.RepoDir, order, cfg.Ignore)
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
		mode, explicit, err := perms.ModeFor(rel, srcInfo.Mode(), cfg.Permissions)
		if err != nil {
			return nil, err
		}

		it := Item{Rel: rel, Source: src, Desired: desired, Mode: mode}
		homePath := filepath.Join(homeDir, filepath.FromSlash(rel))
		if fi, lerr := os.Lstat(homePath); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			it.Symlink = true
		}
		current, err := os.ReadFile(homePath)
		last, tracked := st.Files[rel]
		switch {
		case os.IsNotExist(err):
			it.Status = Create
		case err != nil:
			return nil, err
		default:
			it.Current = current
			switch {
			case bytes.Equal(current, desired):
				it.Status = Clean
				fix, err := modeNeedsFix(homePath, mode, explicit, goos)
				if err != nil {
					return nil, err
				}
				if fix {
					it.Status = Chmod
				}
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
		if _, ok := sources[rel]; ok {
			continue
		}
		// Ignoring is not removing. A path strata used to write and now
		// ignores drops out of the plan entirely, so the $HOME copy survives
		// untouched; classifying it Removed would make one new ignore line
		// delete live config on the next apply.
		ignored, err := layers.Ignored(rel, cfg.Ignore)
		if err != nil {
			return nil, err
		}
		if !ignored {
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
	Written  []string // rels actually written
	Chmodded []string // rels whose mode was fixed without rewriting content
	Deleted  []string // rels deleted from $HOME (file left every layer)
	Blocked  []Item   // changes refused (drift/conflict/unmanaged/edited-removal)
}

// Blocked reports whether apply refuses this item without --force: the
// $HOME copy holds content strata didn't write (drifted, conflict,
// unmanaged); applying would replace a symlink the user set up (writing
// renames a regular file over the link); or a removed file was hand-edited
// after the last apply, so deleting it would destroy those edits.
func (it Item) Blocked(st state.State) bool {
	switch it.Status {
	case Drifted, Conflict, Unmanaged:
		return true
	case Update, Chmod:
		return it.Symlink
	case Removed:
		return it.Current != nil && fsutil.Hash(it.Current) != st.Files[it.Rel]
	}
	return false
}

// Apply writes Create/Update items, fixes Chmod items' modes, and deletes
// Removed ones. If any item is Blocked and force is false, it changes
// NOTHING and returns an error; resolve with 'strata add <file>' (keep
// home) or --force (keep repo).
func Apply(items []Item, homeDir string, st *state.State, force bool) (ApplyResult, error) {
	var res ApplyResult
	for _, it := range items {
		if it.Blocked(*st) {
			res.Blocked = append(res.Blocked, it)
		}
	}
	if len(res.Blocked) > 0 && !force {
		names := ""
		for _, it := range res.Blocked {
			names += fmt.Sprintf("\n  %-9s %s", it.Status, it.Rel)
			if it.Symlink {
				names += " (a symlink — strata won't replace it; --force swaps in a regular file)"
			}
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
			(force && (it.Status == Drifted || it.Status == Conflict || it.Status == Unmanaged)) ||
			(force && it.Symlink && it.Status == Chmod) // chmod would follow the link; replace it instead
		if write {
			dest := filepath.Join(homeDir, filepath.FromSlash(it.Rel))
			if err := fsutil.WriteFileAtomic(dest, it.Desired, it.Mode); err != nil {
				return res, fmt.Errorf("writing %s: %w", it.Rel, err)
			}
			res.Written = append(res.Written, it.Rel)
		}
		if it.Status == Chmod && !write {
			dest := filepath.Join(homeDir, filepath.FromSlash(it.Rel))
			if err := os.Chmod(dest, it.Mode); err != nil {
				return res, fmt.Errorf("chmod %s: %w", it.Rel, err)
			}
			res.Chmodded = append(res.Chmodded, it.Rel)
		}
		if write || it.Status == Clean || it.Status == Chmod {
			st.Files[it.Rel] = fsutil.Hash(it.Desired) // adopt Clean files into state
		}
	}
	return res, nil
}

// modeNeedsFix reports whether a file whose content already matches still
// needs a chmod. Only an explicit [permissions] rule, or an exec bit the
// repo has and $HOME lacks, counts: the 0644 default is for new files and is
// never grounds to loosen a mode the user tightened by hand. Windows has no
// POSIX modes to compare (Perm() reports 0666/0444), so it never needs one.
func modeNeedsFix(path string, want os.FileMode, explicit bool, goos string) (bool, error) {
	if goos == "windows" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	have := info.Mode().Perm()
	if explicit {
		return have != want, nil
	}
	return want&0o111 != 0 && have&0o111 == 0, nil
}

// RunHooks runs the hook for each rel, in sorted order, with dir ($HOME) as
// the working directory so relative paths in a hook are stable. Every hook
// runs even if an earlier one fails. done lists the rels needing no further
// attention — succeeded, or no hook configured anymore — so the caller can
// keep only the failures queued for retry. Hook commands come from the
// user's own dots.toml and are deliberately run through the shell, like git
// hooks (see README "Security note").
func RunHooks(hooks map[string]string, rels []string, dir string, out io.Writer) (done []string, err error) {
	sorted := append([]string(nil), rels...)
	sort.Strings(sorted)
	var failures []error
	for _, rel := range sorted {
		cmdStr, ok := hooks[rel]
		if !ok {
			done = append(done, rel)
			continue
		}
		fmt.Fprintf(out, "hook [%s]: %s\n", rel, cmdStr)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			failures = append(failures, fmt.Errorf("hook for %s failed: %w", rel, err))
			continue
		}
		done = append(done, rel)
	}
	return done, errors.Join(failures...)
}
