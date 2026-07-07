package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	os.MkdirAll(home, 0o755)
	mk := func(rel, content string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("dots.toml", "substitute = [\".gitconfig\"]\n[vars]\nemail = \"default@example.com\"\n[permissions]\n\".ssh/**\" = \"600\"\n")
	mk("base/.zshrc", "export EDITOR=vim\n")
	mk("base/.gitconfig", "[user]\n\temail = {{email}}\n")
	mk("base/.ssh/config", "Host *\n")
	t.Setenv("STRATA_HOME", home)
	t.Setenv("STRATA_CONFIG", filepath.Join(tmp, "machine.toml"))
	t.Setenv("STRATA_STATE", filepath.Join(tmp, "state.json"))
	os.WriteFile(filepath.Join(tmp, "machine.toml"),
		[]byte("repo = \""+filepath.ToSlash(repo)+"\"\nlayers = []\n[vars]\nemail = \"work@cfs.energy\"\n"), 0o644)

	// 1. fresh apply: substitution, permissions
	out, err := run(t, "apply")
	if err != nil || !strings.Contains(out, ".zshrc") {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	git, _ := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if !strings.Contains(string(git), "work@cfs.energy") {
		t.Fatalf("substitution: %s", git)
	}
	if info, err := os.Stat(filepath.Join(home, ".ssh", "config")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ssh config mode: %v, %v", info.Mode(), err)
	}

	// 2. drift → status → refuse → add absorbs
	os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export EDITOR=nvim\n"), 0o644)
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
