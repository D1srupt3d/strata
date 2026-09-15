package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addFixture builds a repo where work/ overrides base/.gitconfig, plus an
// unrelated substituted file with an undefined var, so planning fails. The
// $HOME copy of .gitconfig holds edits the user wants to absorb.
func addFixture(t *testing.T) (repo string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo = filepath.Join(tmp, "repo")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(repo, "dots.toml"), "substitute = [\".broken\"]\n")
	write(filepath.Join(repo, "base", ".gitconfig"), "personal\n")
	write(filepath.Join(repo, "work", ".gitconfig"), "work\n")
	write(filepath.Join(repo, "base", ".broken"), "{{missing}}\n")
	write(filepath.Join(home, ".gitconfig"), "edited work\n")
	write(filepath.Join(tmp, "machine.toml"),
		"repo = \""+filepath.ToSlash(repo)+"\"\nlayers = [\"work\"]\n")
	t.Setenv("STRATA_HOME", home)
	t.Setenv("STRATA_CONFIG", filepath.Join(tmp, "machine.toml"))
	t.Setenv("STRATA_STATE", filepath.Join(tmp, "state.json"))
	return repo
}

// Without --layer, add must know which layer wins. If planning fails it
// cannot know, and guessing base/ would push a work-only file (e.g. the work
// git identity) to every machine. It must refuse and leave the repo alone.
func TestAddRefusesToGuessLayerWhenPlanFails(t *testing.T) {
	repo := addFixture(t)

	_, err := run(t, "add", ".gitconfig")
	if err == nil {
		t.Fatal("add succeeded despite a broken plan; want an error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should surface the underlying cause (undefined var 'missing'), got: %v", err)
	}
	if got := readFile(t, filepath.Join(repo, "base", ".gitconfig")); got != "personal\n" {
		t.Errorf("base/.gitconfig was overwritten: %q", got)
	}
	if got := readFile(t, filepath.Join(repo, "work", ".gitconfig")); got != "work\n" {
		t.Errorf("work/.gitconfig changed: %q", got)
	}
}

// --layer names the target explicitly, so add needs no plan and must keep
// working even while an unrelated file breaks planning — it's the escape
// hatch the refusal above points users to.
func TestAddWithExplicitLayerWorksWhenPlanFails(t *testing.T) {
	repo := addFixture(t)

	if _, err := run(t, "add", ".gitconfig", "--layer", "work"); err != nil {
		t.Fatalf("add --layer work: %v", err)
	}
	if got := readFile(t, filepath.Join(repo, "work", ".gitconfig")); got != "edited work\n" {
		t.Errorf("work/.gitconfig = %q, want the absorbed $HOME edit", got)
	}
	if got := readFile(t, filepath.Join(repo, "base", ".gitconfig")); got != "personal\n" {
		t.Errorf("base/.gitconfig changed: %q", got)
	}
}
