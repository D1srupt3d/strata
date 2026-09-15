// Package state remembers the hash of what strata last wrote to each file,
// enabling three-way drift detection, plus the hooks still owed a run.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"strata/internal/fsutil"
)

// formatVersion is the state.json schema this build reads and writes. Bump
// it — and teach Load to migrate — whenever a field's meaning changes. Files
// from before versioning (no "version" key) read as 0 and are compatible.
const formatVersion = 1

type State struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"` // rel path → sha256 of last-applied content
	// PendingHooks are rels whose hook must still run: queued before hooks
	// start, cleared only on success, so a failed or interrupted hook is
	// retried on the next apply even though its file already reads clean.
	PendingHooks []string `json:"pending_hooks,omitempty"`
}

func Load(path string) (State, error) {
	s := State{Files: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("parsing %s: %w", path, err)
	}
	if s.Version > formatVersion {
		return s, fmt.Errorf("%s was written by a newer strata (state format v%d; this build reads up to v%d) — upgrade strata",
			path, s.Version, formatVersion)
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	return s, nil
}

func (s State) Save(path string) error {
	s.Version = formatVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, b, 0o644)
}

// QueueHooks adds rels to the pending-hook queue (kept sorted, no dupes).
func (s *State) QueueHooks(rels []string) {
	set := map[string]bool{}
	for _, r := range s.PendingHooks {
		set[r] = true
	}
	for _, r := range rels {
		set[r] = true
	}
	s.PendingHooks = sortedSet(set)
}

// FinishHooks drops rels whose hooks no longer need to run.
func (s *State) FinishHooks(rels []string) {
	set := map[string]bool{}
	for _, r := range s.PendingHooks {
		set[r] = true
	}
	for _, r := range rels {
		delete(set, r)
	}
	s.PendingHooks = sortedSet(set)
}

func sortedSet(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// errLocked is what the per-OS lockFile returns when someone else holds it.
var errLocked = errors.New("locked")

// Lock takes an exclusive, non-blocking lock on statePath+".lock", so two
// strata processes can't interleave read-modify-write of the state file —
// the later save would silently drop the earlier one's updates. The OS
// releases the lock when the process exits, so a crash can't leave a stale
// lock behind. Callers should re-read state after locking.
func Lock(statePath string) (unlock func() error, err error) {
	lockPath := statePath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, errLocked) {
			return nil, fmt.Errorf("another strata is already running (lock held on %s)", lockPath)
		}
		return nil, fmt.Errorf("locking %s: %w", lockPath, err)
	}
	return f.Close, nil // closing the file releases the lock
}
