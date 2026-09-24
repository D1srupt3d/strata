package layers

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		t.Helper()
		p := filepath.Join(repo, layer, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
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

// OS junk files land in layer dirs on their own - Finder writes .DS_Store the
// moment you open the repo - so they are never dotfiles, at any depth.
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

// mkDirs creates each folder under repo.
func mkDirs(t *testing.T, repo string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(repo, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// macOS and Windows file systems ignore case, so os.Stat("Work") found
// work/ there and the name passed, while Linux rejected the same
// machine.toml. A layer name is also a lookup key ([layer_vars.work]), so
// the check must match the folder's exact spelling, on every OS.
func TestCheckRolesIsCaseExact(t *testing.T) {
	repo := t.TempDir()
	mkDirs(t, repo, "base", "work")
	if err := CheckRoles(repo, []string{"work"}); err != nil {
		t.Fatalf("exact spelling: %v", err)
	}
	err := CheckRoles(repo, []string{"Work"})
	if err == nil {
		t.Fatal(`CheckRoles accepted "Work" for the folder work/`)
	}
	if !strings.Contains(err.Error(), `"Work" is spelled "work"`) {
		t.Errorf("err = %v, want it to name the folder's real spelling", err)
	}
}

// A [layer_vars] section must name a layer this repo can have: a folder,
// spelled exactly, or a built-in OS layer. A typo'd section would never
// apply, and machines would quietly get the defaults.
func TestCheckVarSections(t *testing.T) {
	repo := t.TempDir()
	mkDirs(t, repo, "base", "work")
	for _, names := range [][]string{
		nil,
		{"work"},
		{"mac", "linux", "windows"}, // built-in OS layers need no folder
	} {
		if err := CheckVarSections(repo, names); err != nil {
			t.Errorf("CheckVarSections(%q) = %v, want nil", names, err)
		}
	}
	for _, tt := range []struct {
		names []string
		want  []string // substrings the error must contain
	}{
		{[]string{"base"}, []string{"[layer_vars.base]", "[vars]"}},
		{[]string{"wrok"}, []string{"[layer_vars.wrok]", "typo?"}},
		{[]string{"arch"}, []string{"[layer_vars.arch]"}}, // a distro needs its folder
		{[]string{"Work"}, []string{"[layer_vars.Work]", `"Work" is spelled "work"`}},
		{[]string{"a/b"}, []string{"[layer_vars.a/b]", "not a layer name"}},
		{[]string{"Work", "wrok"}, []string{"[layer_vars.Work]", "[layer_vars.wrok]"}}, // every one listed
	} {
		err := CheckVarSections(repo, tt.names)
		if err == nil {
			t.Errorf("CheckVarSections(%q) = nil, want an error", tt.names)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("CheckVarSections(%q) = %v, want it to mention %s", tt.names, err, w)
			}
		}
	}
}

// Like CheckRoles: a repo folder that doesn't exist at all reads as an empty
// repo (README "Order matters"), so there is nothing to check.
func TestCheckVarSectionsMissingRepo(t *testing.T) {
	if err := CheckVarSections(filepath.Join(t.TempDir(), "gone"), []string{"wrok"}); err != nil {
		t.Errorf("missing repo: %v, want nil", err)
	}
}

// The exemption must track OSLayers. A new fixed OS layer that isn't exempt
// would need a folder for its vars, and an exempt name OSLayers never
// returns would let a typo through.
func TestBuiltinOSMatchesOSLayers(t *testing.T) {
	got := map[string]bool{}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, n := range OSLayers(goos, "") {
			got[n] = true
		}
	}
	if !reflect.DeepEqual(got, builtinOS) {
		t.Errorf("OSLayers returns %v, builtinOS is %v", got, builtinOS)
	}
}
