package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	home, repo := filepath.Join(tmp, "home"), filepath.Join(tmp, "repo")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(rel, content string) {
		writeFile(t, filepath.Join(repo, filepath.FromSlash(rel)), content)
	}
	mk("dots.toml", "substitute = [\".gitconfig\"]\n[vars]\nemail = \"default@example.com\"\n[permissions]\n\".ssh/**\" = \"600\"\n")
	mk("base/.zshrc", "export EDITOR=vim\n")
	mk("base/.gitconfig", "[user]\n\temail = {{email}}\n")
	mk("base/.ssh/config", "Host *\n")
	t.Setenv("STRATA_HOME", home)
	t.Setenv("STRATA_CONFIG", filepath.Join(tmp, "machine.toml"))
	t.Setenv("STRATA_STATE", filepath.Join(tmp, "state.json"))
	writeFile(t, filepath.Join(tmp, "machine.toml"),
		"repo = \""+filepath.ToSlash(repo)+"\"\nlayers = []\n[vars]\nemail = \"work@cfs.energy\"\n")

	// 1. fresh apply: substitution, permissions
	out, err := run(t, "apply")
	if err != nil || !strings.Contains(out, ".zshrc") {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	git, _ := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if !strings.Contains(string(git), "work@cfs.energy") {
		t.Fatalf("substitution: %s", git)
	}
	info, err := os.Stat(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatalf("ssh config stat: %v", err)
	}
	// Windows has no POSIX modes: Chmod only toggles a read-only bit and
	// Perm() reports 0666/0444, so the mode assertion is POSIX-only.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("ssh config mode: %v", info.Mode())
	}

	// 2. drift → status → refuse → add absorbs
	writeFile(t, filepath.Join(home, ".zshrc"), "export EDITOR=nvim\n")
	out, _ = run(t, "status")
	if !strings.Contains(out, "drifted") {
		t.Fatalf("status: %s", out)
	}
	if _, err := run(t, "apply"); err == nil {
		t.Fatal("apply should refuse drifted file")
	}
	if _, err := run(t, "add", ".zshrc"); err != nil {
		t.Fatal(err)
	}
	repoZshrc, _ := os.ReadFile(filepath.Join(repo, "base", ".zshrc"))
	if !strings.Contains(string(repoZshrc), "nvim") {
		t.Fatal("add did not absorb home edit")
	}
	out, _ = run(t, "status")
	if !strings.Contains(out, "clean") {
		t.Fatalf("after add: %s", out)
	}
}

// Vars stack like files: a machine with the work layer gets the work value,
// and a machine.toml override on top is an ordinary update, not drift.
func TestLayerVarsEndToEnd(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("dots.toml"), "substitute = [\".greet\"]\n[vars]\nname = \"default\"\n[layer_vars.work]\nname = \"work\"\n")
	writeFile(t, s.repo("base/.greet"), "hi {{name}}\n")
	writeFile(t, s.repo("work/.workrc"), "work\n") // a role layer needs its folder
	if out, err := run(t, "apply"); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".greet")); got != "hi work\n" {
		t.Fatalf("$HOME .greet = %q, want the work layer's value", got)
	}

	writeFile(t, s.Machine, fmt.Sprintf("repo = %q\nlayers = [\"work\"]\n[vars]\nname = \"mine\"\n", filepath.ToSlash(s.Repo)))
	out, _ := run(t, "status") // exits 1: something needs attention
	// status prints "%-9s %s", so an update reads "update    .greet".
	if !strings.Contains(out, "update    .greet") {
		t.Fatalf("status after a machine.toml override, want .greet as update:\n%s", out)
	}
	if out, err := run(t, "apply"); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".greet")); got != "hi mine\n" {
		t.Errorf("$HOME .greet = %q, want the machine.toml override", got)
	}
}
