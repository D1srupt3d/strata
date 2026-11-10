package doctor

import (
	"strings"
	"testing"
)

// sshConfig is a 10-line file whose agent line is the only per-OS part.
func sshConfig(agent string) string {
	return "# personal identity\nHost github-personal\n\tHostName github.com\n\tUser git\n" +
		"\tIdentityFile ~/.ssh/me.pub\n\tIdentitiesOnly yes\n\nHost *\n\tAddKeysToAgent yes\n" +
		"\tIdentityAgent " + agent + "\n"
}

// A file copied into two layers to change one line is what [layer_vars]
// replaces: doctor names both layers and how many lines differ.
func TestNearDuplicateCopiesWarn(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "linux/.ssh/config", sshConfig("~/.1password/agent.sock"))
	e.repoFile(t, "mac/.ssh/config", sshConfig("~/Library/agent.sock"))
	f := wantSev(t, Run(e.in), "layers", `".ssh/config" in linux and mac`, Warn)
	if !strings.Contains(f.Detail, "differ in 1 line") {
		t.Errorf("detail = %q, want it to say the copies differ in 1 line", f.Detail)
	}
	if !strings.Contains(f.Fix, "layer_vars") || !strings.Contains(f.Fix, "substitute") {
		t.Errorf("fix = %q, want it to point at layer_vars and substitute", f.Fix)
	}
}

// Identical copies: the later layer wins with the same bytes, so one copy
// does nothing but drift apart from the other over time.
func TestIdenticalCopiesWarn(t *testing.T) {
	e := newEnv(t, "work")
	e.repoFile(t, "base/.vimrc", "set number\nsyntax on\n")
	e.repoFile(t, "work/.vimrc", "set number\nsyntax on\n")
	f := wantSev(t, Run(e.in), "layers", `".vimrc" in base and work`, Warn)
	if !strings.Contains(f.Detail, "identical") {
		t.Errorf("detail = %q, want it to say the copies are identical", f.Detail)
	}
}

// Real per-layer differences (a different key list, a work identity) are
// what layers are for: no warning, and the check says it ran.
func TestDifferentCopiesAreFine(t *testing.T) {
	e := newEnv(t, "work")
	e.repoFile(t, "base/.gitconfig", "[user]\n\tname = me\n\temail = me@home\n[core]\n\teditor = vim\n")
	e.repoFile(t, "work/.gitconfig", "[user]\n\tname = Me Work\n\temail = me@work\n[core]\n\teditor = code\n")
	got := Run(e.in)
	absent(t, got, "layers", `".gitconfig" in base and work`)
	wantSev(t, got, "layers", "copies", OK)
}

// Short files where the changed lines are most of the file aren't "the
// same file with a line changed", and binary files have no lines at all.
func TestTinyAndBinaryCopiesAreSkipped(t *testing.T) {
	e := newEnv(t, "work")
	e.repoFile(t, "base/.theme", "dark\nbig\nfont\n")
	e.repoFile(t, "work/.theme", "light\nsmall\nfont\n")
	e.repoFile(t, "base/bg.jpg", "\x00JFIF\nsame\nsame\nsame\nsame\n1\n")
	e.repoFile(t, "work/bg.jpg", "\x00JFIF\nsame\nsame\nsame\nsame\n2\n")
	got := Run(e.in)
	absent(t, got, "layers", `".theme" in base and work`)
	absent(t, got, "layers", `"bg.jpg" in base and work`)
}

func TestLayerChecksSkipWhenConfigDidNotLoad(t *testing.T) {
	e := newEnv(t)
	mustRemove(t, e.in.MachinePath)
	wantSev(t, Run(e.in), "layers", "copies", Skip)
}
