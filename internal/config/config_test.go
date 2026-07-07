package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
