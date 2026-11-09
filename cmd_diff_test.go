package main

import (
	"strings"
	"testing"
)

// diff says which copy holds the newer edit, so a drifted file can't be
// mistaken for a pending update.
func TestDiffNamesTheNewerSide(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n")

	out, err := run(t, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- home/.zshrc  ") || !strings.Contains(out, "(drifted: $HOME has the newer edit)") {
		t.Fatalf("diff header doesn't name the newer side:\n%s", out)
	}
}

// Named files narrow the diff to just those files, like `git diff <file>`.
func TestDiffNamedFilesOnly(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	writeFile(t, s.repo("base/.vimrc"), "repo\n")
	writeFile(t, s.repo("base/.gitconfig"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n")
	writeFile(t, s.home(".vimrc"), "my edit\n")

	out, err := run(t, "diff", ".zshrc", "~/.gitconfig")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "home/.zshrc") {
		t.Errorf("diff .zshrc doesn't show .zshrc:\n%s", out)
	}
	if strings.Contains(out, ".vimrc") {
		t.Errorf("diff .zshrc also shows .vimrc, which wasn't named:\n%s", out)
	}
	// .gitconfig is clean: named but nothing to show, and no error.
	if strings.Contains(out, ".gitconfig") {
		t.Errorf("clean .gitconfig printed something:\n%s", out)
	}
}

// A file strata doesn't manage is an error, not a silent empty diff that
// reads as "no changes".
func TestDiffUnmanagedFileIsAnError(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	_, err := run(t, "diff", ".bashrc")
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("diff .bashrc: err = %v, want a \"not managed\" error", err)
	}
}

// Commands that take no files reject stray words instead of quietly acting
// on everything (`strata status .zshrc` used to report every file).
func TestNoArgCommandsRejectArguments(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	for _, cmd := range []string{"status", "apply", "sync"} {
		_, err := run(t, cmd, ".zshrc")
		if err == nil || !strings.Contains(err.Error(), cmd+" takes no file names") {
			t.Errorf("%s .zshrc: err = %v, want it rejected", cmd, err)
		}
	}
	if exists(s.home(".zshrc")) {
		t.Error("apply .zshrc wrote files despite the stray argument")
	}
}
