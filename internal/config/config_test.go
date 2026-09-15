package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A typo'd section like [hook] (for [hooks]) used to decode into nothing, so
// the hooks silently never ran. Unknown keys must fail loudly and name the
// offender.
func TestUnknownRepoConfigKeyIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "dots.toml"), "[hook]\n\".zshrc\" = \"echo hi\"\n")
	_, err := LoadRepoConfig(dir)
	if err == nil {
		t.Fatal("dots.toml with unknown section [hook] loaded without error")
	}
	if !strings.Contains(err.Error(), "hook") {
		t.Errorf("error should name the unknown key 'hook': %v", err)
	}
}

func TestUnknownMachineConfigKeyIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machine.toml")
	write(t, path, "repo = \"/tmp/x\"\nlayer = [\"work\"]\n") // typo: layer
	_, err := LoadMachineConfig(path)
	if err == nil {
		t.Fatal("machine.toml with unknown key 'layer' loaded without error")
	}
	if !strings.Contains(err.Error(), "layer") {
		t.Errorf("error should name the unknown key 'layer': %v", err)
	}
}

// A machine.toml without repo, or with a relative one, made layers resolve
// against whatever folder strata ran from: status said clean inside the repo
// and "removed" everywhere else — and apply from the wrong folder deleted.
func TestMachineConfigRepoMustBeAFullPath(t *testing.T) {
	for name, content := range map[string]string{
		"missing":  "layers = []\n",
		"empty":    "repo = \"\"\n",
		"relative": "repo = \"dotfiles\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "machine.toml")
			write(t, path, content)
			if _, err := LoadMachineConfig(path); err == nil || !strings.Contains(err.Error(), "repo") {
				t.Errorf("err = %v, want an error about repo", err)
			}
		})
	}
}

func TestLoadAndMerge(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "repo", "dots.toml"), `
substitute = [".gitconfig"]
[vars]
email = "personal@example.com"
name = "Luke"
[permissions]
".ssh/**" = "600"
[hooks]
".Brewfile" = "brew bundle"
`)
	write(t, filepath.Join(dir, "machine.toml"), `
repo = "`+filepath.ToSlash(filepath.Join(dir, "repo"))+`"
layers = ["work"]
[vars]
email = "work@example.com"
`)
	m, err := LoadMachineConfig(filepath.Join(dir, "machine.toml"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := LoadRepoConfig(m.Repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Merge(r, m)
	if cfg.Vars["email"] != "work@example.com" { // machine overrides repo
		t.Errorf("email = %q", cfg.Vars["email"])
	}
	if cfg.Vars["name"] != "Luke" { // repo default survives
		t.Errorf("name = %q", cfg.Vars["name"])
	}
	if cfg.Permissions[".ssh/**"] != "600" || cfg.Hooks[".Brewfile"] != "brew bundle" {
		t.Error("permissions/hooks not loaded")
	}
	if len(cfg.RoleLayers) != 1 || cfg.RoleLayers[0] != "work" {
		t.Errorf("layers = %v", cfg.RoleLayers)
	}
}

// Where each var's value came from is decided once, here, alongside the
// value itself — so the TUI (and later, layer-scoped vars) can't disagree
// with what apply actually substitutes.
func TestMergeRecordsVarProvenance(t *testing.T) {
	rc := RepoConfig{Vars: map[string]string{"email": "p@example.com", "name": "Luke"}}
	mc := MachineConfig{Vars: map[string]string{"email": "w@example.com", "host": "mbp"}}
	cfg := Merge(rc, mc)
	want := map[string]string{"email": "machine.toml", "name": "dots.toml", "host": "machine.toml"}
	if !reflect.DeepEqual(cfg.VarFrom, want) {
		t.Errorf("VarFrom = %v, want %v", cfg.VarFrom, want)
	}
}

func TestIgnoreLoadsFromRepoConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "repo", "dots.toml"), `
ignore = [".claude/settings.json", "**/*.log"]
`)
	write(t, filepath.Join(dir, "machine.toml"), `
repo = "`+filepath.ToSlash(filepath.Join(dir, "repo"))+`"
`)
	m, err := LoadMachineConfig(filepath.Join(dir, "machine.toml"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := LoadRepoConfig(m.Repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Merge(r, m)
	want := []string{".claude/settings.json", "**/*.log"}
	if !reflect.DeepEqual(cfg.Ignore, want) {
		t.Errorf("Ignore = %v, want %v", cfg.Ignore, want)
	}
}

func TestMissingRepoConfigIsOK(t *testing.T) {
	r, err := LoadRepoConfig(t.TempDir()) // no dots.toml
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Vars) != 0 {
		t.Error("expected empty defaults")
	}
}

func TestTildeExpansion(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "machine.toml"), "repo = \"~/dotfiles\"\n")
	m, err := LoadMachineConfig(filepath.Join(dir, "machine.toml"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if m.Repo != filepath.Join(home, "dotfiles") {
		t.Errorf("repo = %q", m.Repo)
	}
}
