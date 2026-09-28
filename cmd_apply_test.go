package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// hookSandbox has one file whose hook succeeds only once $HOME/ready exists,
// and leaves $HOME/ran behind when it does. Both paths are relative, so the
// hook must run in $HOME to find them.
func hookSandbox(t *testing.T) sandboxEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook uses sh")
	}
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.repo("dots.toml"), "[hooks]\n\".zshrc\" = \"test -f ready && touch ran\"\n")
	return s
}

// A hook that fails must run again on the next apply, even though its file
// is clean by then - otherwise one failed `brew bundle` is never retried.
func TestFailedHookIsRetriedOnNextApply(t *testing.T) {
	s := hookSandbox(t)
	if _, err := run(t, "apply"); err == nil {
		t.Fatal("first apply: want the hook failure reported")
	}

	writeFile(t, s.home("ready"), "")
	if out, err := run(t, "apply"); err != nil {
		t.Fatalf("second apply: %v\n%s", err, out)
	}
	if !exists(s.home("ran")) {
		t.Fatal("the failed hook was not retried on the next apply")
	}

	// Once it has succeeded it is no longer pending.
	if err := os.Remove(s.home("ran")); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if exists(s.home("ran")) {
		t.Error("hook ran again after it had already succeeded")
	}
}

// A write that fails partway (unwritable folder, full disk) used to lose the
// hooks of the files already written: nothing was saved, so the next apply
// saw those files clean and never ran their hooks.
func TestHooksOfFilesWrittenBeforeAFailureStillRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook uses sh, and chmod can't make a Windows folder unwritable")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write into any folder")
	}
	s := sandbox(t)
	writeFile(t, s.repo("base/.a"), "a\n") // sorts before .locked/x, so it's written first
	writeFile(t, s.repo("base/.locked/x"), "x\n")
	writeFile(t, s.repo("dots.toml"), "[hooks]\n\".a\" = \"touch ran\"\n")
	locked := s.home(".locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // let TempDir clean up

	if _, err := run(t, "apply"); err == nil {
		t.Fatal("apply into an unwritable folder: want an error")
	}
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, "apply"); err != nil {
		t.Fatalf("apply after fixing the folder: %v\n%s", err, out)
	}
	if !exists(s.home("ran")) {
		t.Fatal("the hook for .a never ran, though .a was written")
	}
}

// containsLine reports whether any single output line contains all parts.
func containsLine(out string, parts ...string) bool {
	for _, line := range strings.Split(out, "\n") {
		all := true
		for _, p := range parts {
			all = all && strings.Contains(line, p)
		}
		if all {
			return true
		}
	}
	return false
}

// --dry-run must predict what apply will actually do. It used to print
// "would write drifted .zshrc" for a file apply then refused to touch.
func TestDryRunSeparatesBlockedFromWrites(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n")     // drifted: apply refuses
	writeFile(t, s.repo("base/.vimrc"), "set nu\n") // new: apply writes

	out, err := run(t, "apply", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if containsLine(out, "would write", ".zshrc") {
		t.Errorf("drifted .zshrc reported as writable:\n%s", out)
	}
	if !containsLine(out, "blocked", ".zshrc") {
		t.Errorf("dry run doesn't say .zshrc is blocked:\n%s", out)
	}
	if !containsLine(out, "would write", ".vimrc") {
		t.Errorf("dry run doesn't list the new .vimrc:\n%s", out)
	}
	// Apply is all-or-nothing: with a blocked file it writes nothing at all,
	// .vimrc included. The dry run must say so, not just list files.
	if !strings.Contains(out, "nothing is written") {
		t.Errorf("dry run doesn't warn that the blocked file stops the whole apply:\n%s", out)
	}
	if exists(s.home(".vimrc")) {
		t.Error("dry run wrote a file")
	}
}

// status must surface a pending hook rather than report everything clean.
func TestStatusReportsPendingHook(t *testing.T) {
	hookSandbox(t)
	if _, err := run(t, "apply"); err == nil {
		t.Fatal("apply: want the hook failure reported")
	}
	out, _ := run(t, "status")
	if !strings.Contains(out, "hook") || !strings.Contains(out, ".zshrc") {
		t.Errorf("status doesn't mention the pending .zshrc hook:\n%s", out)
	}
	if strings.Contains(out, "clean:") {
		t.Errorf("status claims clean while a hook is pending:\n%s", out)
	}
}

// The bug that made deletion opt-in: `strata add` a file on a branch, `git
// switch` to one without it, and a plain apply deleted the $HOME copy - the
// only copy, had the add not been committed yet. Plain apply keeps it, status
// keeps reporting it, and switching back makes it clean again.
func TestBranchSwitchKeepsFileUntilPrune(t *testing.T) {
	isolateGit(t)
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "zsh\n")
	git(t, s.Repo, "init", "-q", "-b", "main")
	git(t, s.Repo, "add", ".")
	git(t, s.Repo, "commit", "-q", "-m", "init")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	git(t, s.Repo, "switch", "-q", "-c", "feat")
	writeFile(t, s.home(".tmux.conf"), "set -g mouse on\n")
	if _, err := run(t, "add", ".tmux.conf"); err != nil {
		t.Fatal(err)
	}
	git(t, s.Repo, "add", ".")
	git(t, s.Repo, "commit", "-q", "-m", "tmux")

	git(t, s.Repo, "switch", "-q", "main")
	out, err := run(t, "apply")
	if err != nil {
		t.Fatalf("apply on main: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".tmux.conf")); got != "set -g mouse on\n" {
		t.Fatalf("$HOME .tmux.conf = %q after apply on a branch without it", got)
	}
	if !containsLine(out, "kept", ".tmux.conf", "--prune") {
		t.Errorf("apply doesn't say it kept .tmux.conf or how to delete it:\n%s", out)
	}
	out, err = run(t, "status")
	if err == nil || !containsLine(out, "removed", ".tmux.conf") {
		t.Errorf("status after apply: err = %v, out:\n%s\nwant .tmux.conf still reported as removed, exit 1", err, out)
	}

	git(t, s.Repo, "switch", "-q", "feat")
	if out, err := run(t, "status"); err != nil {
		t.Errorf("status back on feat: %v\n%s", err, out)
	}
}

// removedSandbox has two applied files, then deletes .tmux.conf from the repo.
func removedSandbox(t *testing.T) sandboxEnv {
	t.Helper()
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "zsh\n")
	writeFile(t, s.repo("base/.tmux.conf"), "set -g mouse on\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.repo("base/.tmux.conf")); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestApplyPruneDeletesRemovedFile(t *testing.T) {
	s := removedSandbox(t)
	out, err := run(t, "apply", "--prune")
	if err != nil {
		t.Fatalf("apply --prune: %v\n%s", err, out)
	}
	if exists(s.home(".tmux.conf")) {
		t.Error("apply --prune left .tmux.conf in $HOME")
	}
	if out, err := run(t, "status"); err != nil {
		t.Errorf("status after --prune: %v\n%s", err, out)
	}
}

// --prune keeps the old safety rule: an edit made since the last apply is
// only deleted with --force as well.
func TestApplyPruneRefusesEditedRemovedFile(t *testing.T) {
	s := removedSandbox(t)
	writeFile(t, s.home(".tmux.conf"), "my edit\n")
	// 'strata add' is the drift fix, and wrong here: it would put the file
	// back in the repo, undoing the delete. The hint must name the real
	// choices: keep it (no --prune) or delete it (--force).
	out, err := run(t, "apply", "--dry-run", "--prune")
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(out, "blocked", ".tmux.conf", "leave out --prune") || containsLine(out, ".tmux.conf", "strata add") {
		t.Errorf("dry run hint for the edited removed file:\n%s\nwant 'leave out --prune', not 'strata add'", out)
	}
	_, err = run(t, "apply", "--prune")
	if err == nil {
		t.Fatal("apply --prune deleted a file edited since the last apply")
	}
	if !containsLine(err.Error(), ".tmux.conf", "leave out --prune", "--force") {
		t.Errorf("refusal doesn't say how to keep or delete .tmux.conf:\n%v", err)
	}
	if !exists(s.home(".tmux.conf")) {
		t.Fatal("refused apply --prune deleted the file anyway")
	}
	if out, err := run(t, "apply", "--prune", "--force"); err != nil {
		t.Fatalf("apply --prune --force: %v\n%s", err, out)
	}
	if exists(s.home(".tmux.conf")) {
		t.Error("apply --prune --force left the edited file")
	}
}

func TestDryRunKeepsRemovedFilesWithoutPrune(t *testing.T) {
	s := removedSandbox(t)
	out, err := run(t, "apply", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if containsLine(out, "would remove", ".tmux.conf") || !containsLine(out, "would keep", ".tmux.conf", "--prune") {
		t.Errorf("dry run without --prune should keep .tmux.conf and name --prune:\n%s", out)
	}
	out, err = run(t, "apply", "--dry-run", "--prune")
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(out, "would remove", ".tmux.conf") {
		t.Errorf("dry run with --prune doesn't remove .tmux.conf:\n%s", out)
	}
	if !exists(s.home(".tmux.conf")) {
		t.Error("dry run deleted a file")
	}
}
