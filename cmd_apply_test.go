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
// is clean by then — otherwise one failed `brew bundle` is never retried.
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
