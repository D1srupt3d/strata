package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

// sync = git pull + apply, and the apply must use the config as it is AFTER
// the pull: a hook (or var) added upstream has to take effect in that same
// sync, not the next one.
func TestSyncPullsThenAppliesWithPulledConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook uses sh")
	}
	isolateGit(t)
	s := sandbox(t)
	origin := filepath.Join(s.Root, "origin")
	writeFile(t, filepath.Join(origin, "base", ".zshrc"), "v1\n")
	git(t, origin, "init", "-q", "-b", "main")
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-q", "-m", "v1")
	git(t, s.Root, "clone", "-q", origin, s.Repo)
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}

	marker := s.home("synced")
	writeFile(t, filepath.Join(origin, "base", ".zshrc"), "v2\n")
	writeFile(t, filepath.Join(origin, "dots.toml"),
		fmt.Sprintf("[hooks]\n\".zshrc\" = %q\n", "touch '"+filepath.ToSlash(marker)+"'"))
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-q", "-m", "v2 + hook")

	if out, err := run(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".zshrc")); got != "v2\n" {
		t.Errorf("$HOME .zshrc = %q, want pulled v2", got)
	}
	if !exists(marker) {
		t.Error("hook added upstream did not run during the same sync")
	}
}
