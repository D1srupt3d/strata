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

	got, err := Resolve(repo, []string{"base", "mac", "work", "nonexistent-layer"})
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
