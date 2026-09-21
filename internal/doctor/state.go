package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"strata/internal/layers"
	"strata/internal/state"
)

// checkState reads state.json the way every command does (state.Load, no
// lock), then looks for entries apply refuses and leftovers from files or
// hooks that are gone. It needs no config, so it runs even when machine.toml
// is broken; only the pending-hook check needs dots.toml.
func checkState(r *report, in Inputs, l loaded) {
	r.group = "state"
	st, err := state.Load(in.StatePath)
	if err != nil {
		r.add(Error, "state.json", err.Error(),
			"written by a newer strata: run 'strata upgrade'. Corrupt: move it aside and run 'strata apply' - files that already match are adopted again, and ones that differ show as unmanaged instead of being overwritten")
		return
	}
	if _, err := os.Stat(in.StatePath); errors.Is(err, fs.ErrNotExist) {
		r.add(OK, "state.json", "none yet - nothing has been applied on this machine", "")
	} else {
		word := "files"
		if len(st.Files) == 1 {
			word = "file"
		}
		r.add(OK, "state.json", fmt.Sprintf("%d %s tracked", len(st.Files), word), "")
	}

	mark := len(r.findings)
	for _, rel := range slices.Sorted(maps.Keys(st.Files)) {
		subject := fmt.Sprintf("entry %q", rel)
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			r.add(Error, subject, "points outside your home folder, so every apply refuses to run", "remove this entry from "+in.StatePath)
			continue
		}
		// Ignoring is not removing: engine.Plan skips a state entry that
		// matches an ignore pattern, so apply never removes it from state.
		// A bad pattern is already reported by the dots.toml group; here it
		// just means "not ignored", so this entry is still checked below.
		if l.dots {
			if ignored, err := layers.Ignored(rel, l.cfg.Ignore); err == nil && ignored {
				continue
			}
		}
		// Lstat: a symlink that points nowhere is still there.
		if _, err := os.Lstat(filepath.Join(in.Home, filepath.FromSlash(rel))); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				r.add(Warn, subject, "strata wrote this file, but it is gone from your home folder",
					"run 'strata apply': it writes the file again, or forgets it if no layer provides it anymore")
			} else {
				r.add(Error, subject, err.Error(), "check the permissions of that file and the folders above it - apply fails on it too")
			}
		}
	}
	r.okIfClean(mark, "entries", "every tracked file is in your home folder")

	if len(st.PendingHooks) == 0 {
		return
	}
	if !l.dots {
		r.add(Skip, "pending hooks", "need machine.toml and dots.toml to load", "")
		return
	}
	mark = len(r.findings)
	for _, rel := range st.PendingHooks {
		if _, ok := l.cfg.Hooks[rel]; !ok {
			r.add(Warn, fmt.Sprintf("pending hook %q", rel), "a hook for this file failed or was interrupted, but dots.toml has no hook for it anymore",
				"run 'strata apply': it drops the leftover entry")
		}
	}
	r.okIfClean(mark, "pending hooks", fmt.Sprintf("%d waiting to retry on the next apply", len(st.PendingHooks)))
}
