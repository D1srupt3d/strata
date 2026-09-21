package main

import (
	"fmt"
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
// git pull and will fail there - say so up front.
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

// init writes no [vars], so every var runs on its dots.toml default - which
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

// A local folder without --dir is the repo itself. It used to be handed to
// git clone, so `strata init dotfiles/` pointed the machine at a copy in
// ~/dotfiles instead of the repo the user was working in.
func TestInitUsesLocalFolderInPlace(t *testing.T) {
	s := initRepo(t)
	if out, err := runIn(t, "", "init", s.Repo, "--layers", "work"); err != nil {
		t.Fatalf("init <folder>: %v\n%s", err, out)
	}
	if mc := readFile(t, s.Machine); !strings.Contains(mc, fmt.Sprintf("repo = %q", filepath.ToSlash(s.Repo))) {
		t.Errorf("machine.toml does not point at the folder given:\n%s", mc)
	}
	if exists(s.home("dotfiles")) {
		t.Error("init cloned the local folder into ~/dotfiles")
	}
	if got := readFile(t, s.home(".gitconfig")); got != "work\n" {
		t.Errorf("$HOME .gitconfig = %q, want the work layer's", got)
	}
}

// A role layer with no folder is a typo; init must catch it before writing
// machine.toml, not leave a config that every later command rejects.
func TestInitRejectsRoleLayerWithoutAFolder(t *testing.T) {
	s := initRepo(t)
	before := readFile(t, s.Machine)
	_, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "wrok")
	if err == nil || !strings.Contains(err.Error(), "wrok") {
		t.Fatalf("init --layers wrok: err = %v, want one naming the missing layer", err)
	}
	if got := readFile(t, s.Machine); got != before {
		t.Errorf("machine.toml was rewritten despite the error:\n%s", got)
	}
}

// Re-running init (after moving the repo, say) rewrote machine.toml from
// scratch: the machine's [vars] overrides vanished and apply quietly put the
// dots.toml defaults back into $HOME.
func TestInitKeepsMachineVarOverrides(t *testing.T) {
	s := initRepo(t)
	writeFile(t, s.repo("base/.greet"), "hi {{name}}\n")
	writeFile(t, s.repo("dots.toml"), "substitute = [\".greet\"]\n[vars]\nname = \"default\"\n")
	writeFile(t, s.Machine, fmt.Sprintf("repo = %q\nlayers = []\n[vars]\nname = \"override\"\n", filepath.ToSlash(s.Repo)))

	out, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".greet")); got != "hi override\n" {
		t.Errorf("$HOME .greet = %q, want the machine override applied", got)
	}
	if mc := readFile(t, s.Machine); !strings.Contains(mc, "override") {
		t.Errorf("machine.toml lost its [vars] override:\n%s", mc)
	}
	if strings.Contains(out, `name = "default"`) {
		t.Errorf("init says name runs on its default, but machine.toml overrides it:\n%s", out)
	}
}

// A machine.toml init can't parse is refused, not replaced: it may hold the
// only copy of this machine's overrides.
func TestInitRefusesToReplaceAnUnparseableMachineToml(t *testing.T) {
	s := initRepo(t)
	writeFile(t, s.Machine, "this is = = not toml\n")
	if _, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", ""); err == nil {
		t.Fatal("init replaced a machine.toml it couldn't read")
	}
	if got := readFile(t, s.Machine); got != "this is = = not toml\n" {
		t.Errorf("machine.toml was changed:\n%s", got)
	}
}

// init is also how you repair a machine.toml the other commands reject (a
// relative repo, a stale key), so a parseable one is always replaced.
func TestInitRepairsAnInvalidButParseableMachineToml(t *testing.T) {
	s := initRepo(t)
	writeFile(t, s.Machine, "repo = \"dotfiles\"\nlayer = [\"work\"]\n")
	if out, err := runIn(t, "", "init", "--repo", s.Repo, "--layers", "work"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if got := readFile(t, s.home(".gitconfig")); got != "work\n" {
		t.Errorf("$HOME .gitconfig = %q, want the work layer's", got)
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
