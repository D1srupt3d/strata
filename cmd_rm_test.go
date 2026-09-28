package main

import (
	"strings"
	"testing"
)

// rm deletes the file from the layer that wins, then applies - so a file no
// other layer provides disappears from $HOME as well.
func TestRmDeletesWinningSourceAndApplies(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.tmux.conf"), "set -g mouse on\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "rm", ".tmux.conf"); err != nil {
		t.Fatal(err)
	}
	if exists(s.repo("base/.tmux.conf")) {
		t.Error("source still in the repo")
	}
	if exists(s.home(".tmux.conf")) {
		t.Error("file still in $HOME")
	}
}

// Removing a role-layer override means the base copy wins again: the $HOME
// file is rewritten, not deleted.
func TestRmOfOverrideFallsBackToBase(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("base/.gitconfig"), "personal\n")
	writeFile(t, s.repo("work/.gitconfig"), "work\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "rm", ".gitconfig"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.home(".gitconfig")); got != "personal\n" {
		t.Errorf("$HOME .gitconfig = %q, want base's %q", got, "personal\n")
	}
}

// rm deletes the source and then applies. When that apply was going to
// refuse (another file is drifted), rm deleted the source anyway and stopped
// there, half done. It must refuse before touching anything.
func TestRmRefusesUpFrontWhenApplyWouldBeBlocked(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.tmux.conf"), "set -g mouse on\n")
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n") // drifted: apply would refuse

	_, err := run(t, "rm", ".tmux.conf")
	if err == nil || !strings.Contains(err.Error(), ".zshrc") {
		t.Fatalf("rm: err = %v, want a refusal naming the drifted .zshrc", err)
	}
	if !exists(s.repo("base/.tmux.conf")) {
		t.Error("rm deleted the source although the apply after it was going to refuse")
	}
}

// rm deletes the file it names, not every removed file: another file that
// left the repo (say, it's only on another branch) stays in $HOME.
func TestRmDeletesOnlyItsOwnFile(t *testing.T) {
	s := removedSandbox(t)
	if out, err := run(t, "rm", ".zshrc"); err != nil {
		t.Fatalf("rm: %v\n%s", err, out)
	}
	if exists(s.home(".zshrc")) {
		t.Error("rm left .zshrc in $HOME")
	}
	if !exists(s.home(".tmux.conf")) {
		t.Error("rm .zshrc also deleted the unrelated removed .tmux.conf")
	}
}

// An edited removed file is only blocked when something would delete it; it
// must not make rm of a different file refuse.
func TestRmIgnoresAnEditedRemovedFile(t *testing.T) {
	s := removedSandbox(t)
	writeFile(t, s.home(".tmux.conf"), "my edit\n")
	if out, err := run(t, "rm", ".zshrc"); err != nil {
		t.Fatalf("rm refused over an unrelated removed file: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".tmux.conf")); got != "my edit\n" {
		t.Errorf("$HOME .tmux.conf = %q, want the edit kept", got)
	}
}
