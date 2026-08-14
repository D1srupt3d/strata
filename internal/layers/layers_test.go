package layers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOSLayers(t *testing.T) {
	tests := []struct {
		goos, osRelease string
		want            []string
	}{
		{"darwin", "", []string{"mac"}},
		{"windows", "", []string{"windows"}},
		{"linux", "NAME=\"Arch Linux\"\nID=arch\n", []string{"linux", "arch"}},
		{"linux", "ID=\"ubuntu\"\n", []string{"linux", "ubuntu"}},
		{"linux", "", []string{"linux"}},
	}
	for _, tt := range tests {
		if got := OSLayers(tt.goos, tt.osRelease); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("OSLayers(%q) = %v, want %v", tt.goos, got, tt.want)
		}
	}
}

func TestOrder(t *testing.T) {
	got := Order([]string{"work"}, "darwin", "")
	want := []string{"base", "mac", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Order = %v, want %v", got, want)
	}
}

func TestResolveLaterLayerWins(t *testing.T) {
	repo := t.TempDir()
	mk := func(layer, rel, content string) {
		p := filepath.Join(repo, layer, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("base", ".zshrc", "base")
	mk("base", ".gitconfig", "base-git")
	mk("work", ".gitconfig", "work-git")
	mk("mac", ".config/nvim/init.lua", "lua")

	got, err := Resolve(repo, []string{"base", "mac", "work", "nonexistent-layer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("resolved %d files: %v", len(got), got)
	}
	if got[".gitconfig"] != filepath.Join(repo, "work", ".gitconfig") {
		t.Errorf("work layer should win: %v", got[".gitconfig"])
	}
	if _, ok := got[".config/nvim/init.lua"]; !ok {
		t.Error("nested path missing (keys must use forward slashes)")
	}
}

// mkLayer writes content into <repo>/<layer>/<rel>, creating parents.
func mkLayer(t *testing.T, repo, layer, rel, content string) {
	t.Helper()
	p := filepath.Join(repo, layer, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// OS junk files land in layer dirs on their own — Finder writes .DS_Store the
// moment you open the repo — so they are never dotfiles, at any depth.
func TestResolveSkipsDefaultJunk(t *testing.T) {
	repo := t.TempDir()
	mkLayer(t, repo, "base", ".zshrc", "z")
	mkLayer(t, repo, "base", ".DS_Store", "junk")
	mkLayer(t, repo, "base", ".config/nvim/.DS_Store", "junk")
	mkLayer(t, repo, "base", ".config/._resource", "junk")
	mkLayer(t, repo, "base", "Thumbs.db", "junk")

	got, err := Resolve(repo, []string{"base"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for rel := range got {
		if rel != ".zshrc" {
			t.Errorf("junk file was resolved: %q", rel)
		}
	}
	if _, ok := got[".zshrc"]; !ok {
		t.Error("real dotfile .zshrc was dropped")
	}
}

func TestResolveSkipsConfiguredIgnores(t *testing.T) {
	repo := t.TempDir()
	mkLayer(t, repo, "base", ".zshrc", "z")
	mkLayer(t, repo, "base", ".claude/settings.json", "app state")
	mkLayer(t, repo, "base", ".config/app/debug.log", "noise")

	got, err := Resolve(repo, []string{"base"}, []string{".claude/settings.json", "**/*.log"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[".claude/settings.json"]; ok {
		t.Error("exact-path ignore did not take effect")
	}
	if _, ok := got[".config/app/debug.log"]; ok {
		t.Error("glob ignore did not take effect")
	}
	if _, ok := got[".zshrc"]; !ok {
		t.Error("real dotfile .zshrc was dropped")
	}
}

func TestResolveReportsBadIgnorePattern(t *testing.T) {
	repo := t.TempDir()
	mkLayer(t, repo, "base", ".zshrc", "z")

	if _, err := Resolve(repo, []string{"base"}, []string{"["}); err == nil {
		t.Error("malformed ignore pattern should fail loud, not silently match nothing")
	}
}
