package engine

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"strata/internal/config"
	"strata/internal/fsutil"
	"strata/internal/state"
)

// fixture builds a repo with base/work layers and returns (cfg, home).
func fixture(t *testing.T) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	mk := func(rel, content string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("base/.zshrc", "base zshrc\n")
	mk("base/.gitconfig", "email = {{email}}\n")
	mk("work/.gitconfig", "email = {{email}}\nwork = true\n")
	return config.Config{
		RepoDir:    repo,
		RoleLayers: []string{"work"},
		Vars:       map[string]string{"email": "w@cfs.energy"},
		Substitute: []string{".gitconfig"},
	}, home
}

func plan(t *testing.T, cfg config.Config, home string, st state.State) map[string]Item {
	t.Helper()
	items, err := Plan(cfg, home, st, "darwin", "")
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]Item{}
	for _, it := range items {
		byRel[it.Rel] = it
	}
	return byRel
}

func TestPlanStatuses(t *testing.T) {
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}

	// Fresh home: everything is Create; work layer + substitution applied.
	items := plan(t, cfg, home, st)
	if items[".zshrc"].Status != Create || items[".gitconfig"].Status != Create {
		t.Fatalf("fresh home: %+v", items)
	}
	if string(items[".gitconfig"].Desired) != "email = w@cfs.energy\nwork = true\n" {
		t.Fatalf("layering/substitution wrong: %q", items[".gitconfig"].Desired)
	}

	// Simulate applied state.
	for rel, it := range items {
		os.MkdirAll(filepath.Dir(filepath.Join(home, rel)), 0o755)
		os.WriteFile(filepath.Join(home, rel), it.Desired, 0o644)
		st.Files[rel] = fsutil.Hash(it.Desired)
	}
	items = plan(t, cfg, home, st)
	if items[".zshrc"].Status != Clean {
		t.Fatalf("want Clean, got %v", items[".zshrc"].Status)
	}

	// Repo changes, home untouched → Update.
	os.WriteFile(filepath.Join(cfg.RepoDir, "base", ".zshrc"), []byte("new zshrc\n"), 0o644)
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Update {
		t.Fatalf("want Update, got %v", s)
	}

	// Home edited too → Conflict.
	os.WriteFile(filepath.Join(home, ".zshrc"), []byte("home edit\n"), 0o644)
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Conflict {
		t.Fatalf("want Conflict, got %v", s)
	}

	// Repo back to matching state hash, home still edited → Drifted.
	os.WriteFile(filepath.Join(cfg.RepoDir, "base", ".zshrc"), []byte("base zshrc\n"), 0o644)
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Drifted {
		t.Fatalf("want Drifted, got %v", s)
	}

	// Untracked existing file that differs → Unmanaged.
	delete(st.Files, ".zshrc")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Unmanaged {
		t.Fatalf("want Unmanaged, got %v", s)
	}
}

func TestUndefinedVarFailsPlan(t *testing.T) {
	cfg, home := fixture(t)
	cfg.Vars = nil
	if _, err := Plan(cfg, home, state.State{Files: map[string]string{}}, "darwin", ""); err == nil {
		t.Fatal("expected substitution error")
	}
}

func TestApplyWritesAndRefuses(t *testing.T) {
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}

	items, _ := Plan(cfg, home, st, "darwin", "")
	res, err := Apply(items, home, &st, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 2 {
		t.Fatalf("written = %v", res.Written)
	}
	got, _ := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if string(got) != "email = w@cfs.energy\nwork = true\n" {
		t.Fatalf("applied content: %q", got)
	}
	if st.Files[".gitconfig"] == "" {
		t.Fatal("state not updated")
	}

	// Drift + repo change → Conflict blocks the whole apply.
	os.WriteFile(filepath.Join(home, ".zshrc"), []byte("edited\n"), 0o644)
	os.WriteFile(filepath.Join(cfg.RepoDir, "base", ".zshrc"), []byte("repo change\n"), 0o644)
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false); err == nil {
		t.Fatal("expected refusal on conflict")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(b) != "edited\n" {
		t.Fatal("blocked apply must not write anything")
	}

	// --force wins.
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(b) != "repo change\n" {
		t.Fatal("--force should overwrite")
	}
}

func TestRunHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-based test")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	hooks := map[string]string{".Brewfile": "touch " + marker}
	if err := RunHooks(hooks, []string{".zshrc"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("hook ran for wrong file")
	}
	if err := RunHooks(hooks, []string{".Brewfile"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("hook did not run")
	}
}
