package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// initRepo seeds a repo where work/ overrides base/.gitconfig, so which
// layers init selected is visible in the applied file.
func initRepo(t *testing.T) sandboxEnv {
	t.Helper()
	s := sandbox(t)
	writeFile(t, s.repo("base/.gitconfig"), "personal\n")
	writeFile(t, s.repo("work/.gitconfig"), "work\n")
	return s
}

func TestInitWithFlagsWritesMachineTomlAndApplies(t *testing.T) {
	s := initRepo(t)
	if _, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "work"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.home(".gitconfig")); got != "work\n" {
		t.Errorf("$HOME .gitconfig = %q, want the work layer's", got)
	}
	if mc := readFile(t, s.Machine); !strings.Contains(mc, `layers = ["work"]`) {
		t.Errorf("machine.toml missing the chosen layer:\n%s", mc)
	}
}

// The documented `--layers ""` means "no role layers, don't ask". It used to
// be indistinguishable from omitting the flag, so it prompted anyway.
func TestInitEmptyLayersFlagSkipsPrompt(t *testing.T) {
	s := initRepo(t)
	out, err := runIn(t, "work\n", "init", "--repo", s.Repo, "--layers", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "role layers") {
		t.Errorf(`--layers "" still prompted:\n%s`, out)
	}
	if mc := readFile(t, s.Machine); !strings.Contains(mc, "layers = []") {
		t.Errorf("machine.toml should select no layers:\n%s", mc)
	}
	if got := readFile(t, s.home(".gitconfig")); got != "personal\n" {
		t.Errorf("$HOME .gitconfig = %q, want base's (no role layers)", got)
	}
}

// A plain folder (e.g. a downloaded zip) works for apply, but `sync` runs
// git pull and will fail there — say so up front.
func TestInitWarnsWhenRepoIsNotAGitRepo(t *testing.T) {
	s := initRepo(t)
	out, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not a git repository") || !strings.Contains(out, "sync") {
		t.Errorf("expected a warning that sync won't work, got:\n%s", out)
	}
}

func TestInitDoesNotWarnForAGitRepo(t *testing.T) {
	isolateGit(t)
	s := initRepo(t)
	git(t, s.Repo, "init", "-q")
	out, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "not a git repository") {
		t.Errorf("warned about a real git repo:\n%s", out)
	}
}

// init writes no [vars], so every var runs on its dots.toml default — which
// silently gave a work machine the personal values. Say which ones, and
// where to override them.
func TestInitRemindsAboutVarsOnDefaults(t *testing.T) {
	s := initRepo(t)
	writeFile(t, s.repo("dots.toml"), "[vars]\npalette = \"everforest\"\n")
	out, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "palette") || !strings.Contains(out, "machine.toml") {
		t.Errorf("expected a reminder naming 'palette' and machine.toml, got:\n%s", out)
	}
}

// Clone mode: init <url> clones, then sets up exactly like --repo.
func TestInitClonesFromURL(t *testing.T) {
	isolateGit(t)
	s := sandbox(t)
	origin := filepath.Join(s.Root, "origin")
	writeFile(t, filepath.Join(origin, "base", ".zshrc"), "from origin\n")
	git(t, origin, "init", "-q", "-b", "main")
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-q", "-m", "init")

	dest := filepath.Join(s.Root, "cloned")
	if out, err := runIn(t, "", "init", origin, "--dir", dest, "--layers", ""); err != nil {
		t.Fatalf("init <url>: %v\n%s", err, out)
	}
	if !exists(filepath.Join(dest, ".git")) {
		t.Error("repo was not cloned")
	}
	if got := readFile(t, s.home(".zshrc")); got != "from origin\n" {
		t.Errorf("$HOME .zshrc = %q, want the cloned content", got)
	}
}

func TestInitPromptsForLayersWhenFlagOmitted(t *testing.T) {
	s := initRepo(t)
	out, err := runIn(t, "work\n", "init", "--repo", s.Repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "role layers") {
		t.Errorf("expected a role-layer prompt, got:\n%s", out)
	}
	if got := readFile(t, s.home(".gitconfig")); got != "work\n" {
		t.Errorf("$HOME .gitconfig = %q, want the work layer's", got)
	}
}
