package engine

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"strata/internal/config"
	"strata/internal/fsutil"
	"strata/internal/state"
)

// mustWrite writes a fixture file (creating parents), failing the test on
// error - a fixture that silently didn't get written makes the real failure
// unreadable.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// fixture builds a repo with base/work layers and returns (cfg, home).
func fixture(t *testing.T) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(rel, content string) {
		mustWrite(t, filepath.Join(repo, filepath.FromSlash(rel)), content)
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

// Ignoring a file must mean "forget it", not "delete it". A path that was
// managed before and is now ignored - .DS_Store that strata used to copy, or a
// settings.json the user decided isn't a real dotfile - must be dropped from
// the plan entirely, leaving the $HOME copy alone. Treating it as Removed
// would make adding one ignore line silently delete live config.
func TestIgnoredFileIsForgottenNotDeleted(t *testing.T) {
	cfg, home := fixture(t)
	cfg.Ignore = []string{".claude/settings.json"}

	// Both files exist in $HOME and were recorded by an earlier apply.
	for _, rel := range []string{".DS_Store", ".claude/settings.json"} {
		mustWrite(t, filepath.Join(home, filepath.FromSlash(rel)), "live content")
	}
	st := state.State{Files: map[string]string{
		".DS_Store":             fsutil.Hash([]byte("live content")),
		".claude/settings.json": fsutil.Hash([]byte("live content")),
	}}

	items := plan(t, cfg, home, st)
	if it, ok := items[".DS_Store"]; ok {
		t.Errorf("built-in ignored file planned as %v, want absent", it.Status)
	}
	if it, ok := items[".claude/settings.json"]; ok {
		t.Errorf("configured ignored file planned as %v, want absent", it.Status)
	}

	all, err := Plan(cfg, home, st, "darwin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(all, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".DS_Store", ".claude/settings.json"} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); err != nil {
			t.Errorf("apply deleted ignored file %s from $HOME: %v", rel, err)
		}
	}
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
		mustWrite(t, filepath.Join(home, filepath.FromSlash(rel)), string(it.Desired))
		st.Files[rel] = fsutil.Hash(it.Desired)
	}
	items = plan(t, cfg, home, st)
	if items[".zshrc"].Status != Clean {
		t.Fatalf("want Clean, got %v", items[".zshrc"].Status)
	}

	// Repo changes, home untouched → Update.
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"), "new zshrc\n")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Update {
		t.Fatalf("want Update, got %v", s)
	}

	// Home edited too → Conflict.
	mustWrite(t, filepath.Join(home, ".zshrc"), "home edit\n")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Conflict {
		t.Fatalf("want Conflict, got %v", s)
	}

	// Repo back to matching state hash, home still edited → Drifted.
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"), "base zshrc\n")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Drifted {
		t.Fatalf("want Drifted, got %v", s)
	}

	// Untracked existing file that differs → Unmanaged.
	delete(st.Files, ".zshrc")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Unmanaged {
		t.Fatalf("want Unmanaged, got %v", s)
	}
}

// A role layer named in machine.toml with no folder in the repo is a typo,
// not an empty layer: skipping it made every file it provided read Removed,
// and the next apply deleted them from $HOME.
func TestMissingRoleLayerFolderIsAnError(t *testing.T) {
	cfg, home := fixture(t)
	cfg.RoleLayers = []string{"wrok"}
	_, err := Plan(cfg, home, state.State{Files: map[string]string{}}, "darwin", "")
	if err == nil || !strings.Contains(err.Error(), `"wrok"`) {
		t.Fatalf("Plan with a typo'd role layer: err = %v, want one naming \"wrok\"", err)
	}
}

// A layer is a folder directly inside the repo. "../home" would have walked
// a directory outside the repo as if it were a layer.
func TestRoleLayerMustBeAFolderName(t *testing.T) {
	cfg, home := fixture(t) // home is a sibling of the repo
	cfg.RoleLayers = []string{"../home"}
	if _, err := Plan(cfg, home, state.State{Files: map[string]string{}}, "darwin", ""); err == nil {
		t.Fatal("Plan accepted a role layer outside the repo")
	}
}

// A [layer_vars] section naming no layer would silently never apply, and the
// machine would quietly get the defaults. Plan refuses it before any var is
// used.
func TestTypoLayerVarsSectionIsAnError(t *testing.T) {
	cfg, home := fixture(t)
	cfg.VarSections = []string{"wrok"}
	_, err := Plan(cfg, home, state.State{Files: map[string]string{}}, "darwin", "")
	if err == nil || !strings.Contains(err.Error(), "[layer_vars.wrok]") || !strings.Contains(err.Error(), "dots.toml") {
		t.Fatalf("Plan with [layer_vars.wrok]: err = %v, want one naming the section and dots.toml", err)
	}
}

// The documented exception: a repo folder that doesn't exist at all (moved or
// deleted) still reads as an empty repo, so every managed file is Removed -
// README "Order matters". Only a missing layer inside a real repo is a typo.
func TestMissingRepoStillPlansRemovals(t *testing.T) {
	cfg, home := fixture(t)
	cfg.RepoDir = filepath.Join(t.TempDir(), "moved-away")
	mustWrite(t, filepath.Join(home, ".zshrc"), "x")
	st := state.State{Files: map[string]string{".zshrc": fsutil.Hash([]byte("x"))}}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Removed {
		t.Fatalf("want Removed, got %v", s)
	}
}

// state.json is strata's own file, but it's plain JSON on disk: a path in it
// that climbs out of $HOME (hand-edited, corrupted) must never be deleted.
func TestStatePathOutsideHomeIsRefused(t *testing.T) {
	cfg, home := fixture(t)
	outside := filepath.Join(filepath.Dir(home), "outside")
	mustWrite(t, outside, "x")
	st := state.State{Files: map[string]string{"../outside": fsutil.Hash([]byte("x"))}}
	items, err := Plan(cfg, home, st, "darwin", "")
	if err == nil {
		_, err = Apply(items, home, &st, false, nil)
	}
	if err == nil {
		t.Error("a state entry outside $HOME was planned and applied without error")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("strata deleted a file outside $HOME: %v", statErr)
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
	res, err := Apply(items, home, &st, false, nil)
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
	mustWrite(t, filepath.Join(home, ".zshrc"), "edited\n")
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"), "repo change\n")
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err == nil {
		t.Fatal("expected refusal on conflict")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(b) != "edited\n" {
		t.Fatal("blocked apply must not write anything")
	}

	// --force wins.
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, true, nil); err != nil {
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
	hooks := map[string]string{".Brewfile": "touch '" + marker + "'"}
	if _, err := RunHooks(hooks, []string{".zshrc"}, dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("hook ran for wrong file")
	}
	if _, err := RunHooks(hooks, []string{".Brewfile"}, dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("hook did not run")
	}
}

// Hooks run in the directory they're given ($HOME), not wherever strata was
// started from - so relative paths in hook commands mean the same thing on
// every run.
func TestRunHooksRunsInGivenDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-based test")
	}
	dir := t.TempDir()
	where := filepath.Join(t.TempDir(), "where")
	hooks := map[string]string{".x": "pwd > '" + where + "'"}
	if _, err := RunHooks(hooks, []string{".x"}, dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(where)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(b)))
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("hook ran in %q, want %q", got, want)
	}
}

// One failing hook must not stop the others, and the caller must learn
// exactly which ones are done, so only the failure stays pending for retry.
func TestRunHooksRunsAllAndReportsDone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-based test")
	}
	dir := t.TempDir()
	cRan := filepath.Join(dir, "c-ran")
	hooks := map[string]string{".a": "true", ".b": "exit 3", ".c": "touch '" + cRan + "'"}
	done, err := RunHooks(hooks, []string{".c", ".b", ".a", ".nohook"}, dir, io.Discard)
	if err == nil || !strings.Contains(err.Error(), ".b") {
		t.Errorf("err = %v, want a failure naming .b", err)
	}
	// A rel with no hook configured (e.g. removed from dots.toml since) is
	// trivially done.
	if want := []string{".a", ".c", ".nohook"}; !reflect.DeepEqual(done, want) {
		t.Errorf("done = %v, want %v", done, want)
	}
	if _, err := os.Stat(cRan); err != nil {
		t.Error("hook for .c was skipped because .b failed")
	}
}

// A [permissions] rule added after a file was applied must still reach it.
// Content matching isn't enough: .zshrc at 0644 is not "clean" under a 600
// rule. The fix is a chmod, not a rewrite - so it must not count as a
// written file, or hooks for unchanged content would fire.
func TestPermissionRuleReachesUnchangedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX file modes on Windows")
	}
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, err := Plan(cfg, home, st, "darwin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}

	cfg.Permissions = map[string]string{".zshrc": "600"}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Chmod {
		t.Fatalf(".zshrc status = %v, want chmod", s)
	}
	items, err = Plan(cfg, home, st, "darwin", "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Apply(items, home, &st, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after apply = %o, want 600", got)
	}
	if len(res.Written) != 0 {
		t.Errorf("a chmod is not a content write; Written = %v", res.Written)
	}
	if len(res.Chmodded) != 1 || res.Chmodded[0] != ".zshrc" {
		t.Errorf("Chmodded = %v, want [.zshrc]", res.Chmodded)
	}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Clean {
		t.Errorf("after apply: %v, want clean", s)
	}
}

// Without an explicit rule strata must not "fix" a mode the user tightened
// by hand: the 0644 default is for new files, not a policy to enforce.
func TestNoRuleNeverLoosensAMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX file modes on Windows")
	}
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(home, ".zshrc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Clean {
		t.Fatalf("status = %v, want clean (no rule, so a tighter mode stays)", s)
	}
}

// A file made executable in the repo must become executable in $HOME, even
// though its content didn't change.
func TestExecBitReachesUnchangedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX file modes on Windows")
	}
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(cfg.RepoDir, "base", ".zshrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Chmod {
		t.Fatalf("status = %v, want chmod", s)
	}
}

// Windows has no POSIX modes (Perm() reports 0666/0444), so comparing them
// would flag every ruled file forever. goos, not the host, decides.
func TestModesAreNotComparedOnWindows(t *testing.T) {
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	cfg.Permissions = map[string]string{".zshrc": "600"}
	items, err := Plan(cfg, home, st, "windows", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Rel == ".zshrc" && it.Status != Clean {
			t.Fatalf("goos=windows: status = %v, want clean", it.Status)
		}
	}
}

// A $HOME dotfile that is a symlink (into Dropbox, another tool's dir, ...)
// is a setup strata didn't make. Rename-into-place would silently replace
// the link with a regular file, so apply must refuse unless forced.
func TestSymlinkInHomeIsNotReplacedWithoutForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	// Swap ~/.zshrc for a symlink to an identical file elsewhere.
	target := filepath.Join(t.TempDir(), "real-zshrc")
	link := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("base zshrc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	// The repo moves on: without the link, this would be a plain update.
	if err := os.WriteFile(filepath.Join(cfg.RepoDir, "base", ".zshrc"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err == nil {
		t.Fatal("apply replaced a symlinked dotfile without --force")
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("~/.zshrc is no longer a symlink (err=%v)", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "base zshrc\n" {
		t.Errorf("symlink target was modified: %q", b)
	}

	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, true, nil); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("--force should replace the link with a regular file (err=%v)", err)
	}
}

func TestRemoval(t *testing.T) {
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}

	// File leaves the work layer but still exists in base → base wins again:
	// a plain update, NOT a removal.
	mustRemove(t, filepath.Join(cfg.RepoDir, "work", ".gitconfig"))
	if s := plan(t, cfg, home, st)[".gitconfig"].Status; s != Update {
		t.Fatalf(".gitconfig want Update, got %v", s)
	}

	// Sole provider deleted from the repo → Removed; apply --prune deletes
	// from home and drops the state entry.
	mustRemove(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"))
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Removed {
		t.Fatalf(".zshrc want Removed, got %v", s)
	}
	items, _ = Plan(cfg, home, st, "darwin", "")
	res, err := Apply(items, home, &st, false, PruneAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 1 || res.Deleted[0] != ".zshrc" {
		t.Fatalf("Deleted = %v", res.Deleted)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal(".zshrc should be gone from home")
	}
	if _, ok := st.Files[".zshrc"]; ok {
		t.Fatal("state entry should be gone")
	}

	// Removed from repo but locally edited → --prune refuses; --prune
	// --force deletes.
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".tmux.conf"), "set -g mouse on\n")
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	mustRemove(t, filepath.Join(cfg.RepoDir, "base", ".tmux.conf"))
	mustWrite(t, filepath.Join(home, ".tmux.conf"), "my edit\n")
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, PruneAll); err == nil {
		t.Fatal("expected refusal: removed file was locally edited")
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); err != nil {
		t.Fatal("blocked apply must not delete")
	}
	items, _ = Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, true, PruneAll); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); !os.IsNotExist(err) {
		t.Fatal("--prune --force should delete")
	}

	// Stale state entry (file gone from home AND layers) cleans up silently.
	st.Files[".ghost"] = "deadbeef"
	items, _ = Plan(cfg, home, st, "darwin", "")
	res, err = Apply(items, home, &st, false, nil)
	if err != nil || len(res.Deleted) != 0 {
		t.Fatalf("ghost cleanup: %v %v", err, res.Deleted)
	}
	if _, ok := st.Files[".ghost"]; ok {
		t.Fatal("stale state entry should be dropped")
	}
}

// removedFixture applies the fixture, then deletes .zshrc from the repo: a
// file strata wrote that no layer provides anymore, as after a git switch to
// a branch that doesn't have it.
func removedFixture(t *testing.T) (config.Config, string, state.State) {
	t.Helper()
	cfg, home := fixture(t)
	st := state.State{Files: map[string]string{}}
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	mustRemove(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"))
	return cfg, home, st
}

// strata can't tell a file deleted on purpose from one that's only missing
// because the repo is on another branch, so plain apply never deletes. It
// keeps the state entry too: dropping it would orphan the file for good,
// the bug 2026.07.1 fixed.
func TestPlainApplyKeepsRemovedFile(t *testing.T) {
	cfg, home, st := removedFixture(t)
	items, _ := Plan(cfg, home, st, "darwin", "")
	res, err := Apply(items, home, &st, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".zshrc")); err != nil || string(got) != "base zshrc\n" {
		t.Fatalf("$HOME .zshrc = %q, %v; want it kept as is", got, err)
	}
	if len(res.Deleted) != 0 || !reflect.DeepEqual(res.Kept, []string{".zshrc"}) {
		t.Fatalf("Deleted = %v, Kept = %v; want nothing deleted, .zshrc kept", res.Deleted, res.Kept)
	}
	if _, ok := st.Files[".zshrc"]; !ok {
		t.Fatal("state entry dropped: .zshrc would never show as removed again")
	}
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Removed {
		t.Fatalf("next plan: .zshrc = %v, want still Removed", s)
	}
}

// The branch switch undone: the file comes back to the repo and gets the
// normal three-way comparison, which needs the entry plain apply kept.
func TestKeptRemovedFileComesBackTracked(t *testing.T) {
	cfg, home, st := removedFixture(t)
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"), "base zshrc\n")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Clean {
		t.Fatalf("same content back: .zshrc = %v, want Clean", s)
	}
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".zshrc"), "changed meanwhile\n")
	if s := plan(t, cfg, home, st)[".zshrc"].Status; s != Update {
		t.Fatalf("changed content back: .zshrc = %v, want Update", s)
	}
}

// Keeping a removed file means an edit to it has nothing to protect against,
// so it must not block the rest of a plain apply.
func TestEditedRemovedFileDoesNotBlockPlainApply(t *testing.T) {
	cfg, home, st := removedFixture(t)
	mustWrite(t, filepath.Join(home, ".zshrc"), "my edit\n")
	mustWrite(t, filepath.Join(cfg.RepoDir, "base", ".vimrc"), "set nu\n")
	items, _ := Plan(cfg, home, st, "darwin", "")
	for _, it := range items {
		if it.Blocked(st, nil) {
			t.Errorf("%s (%v) reads as blocked without --prune", it.Rel, it.Status)
		}
	}
	if _, err := Apply(items, home, &st, false, nil); err != nil {
		t.Fatalf("plain apply refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".vimrc")); err != nil {
		t.Fatal("new .vimrc not written")
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(got) != "my edit\n" {
		t.Fatalf("$HOME .zshrc = %q, want the edit kept", got)
	}
}

// --force means "take the repo's version", not "delete": run to settle an
// unrelated drift, it used to delete every removed file, edited ones too.
func TestForceAloneNeverDeletes(t *testing.T) {
	cfg, home, st := removedFixture(t)
	mustWrite(t, filepath.Join(home, ".zshrc"), "my edit\n")
	mustWrite(t, filepath.Join(home, ".gitconfig"), "drifted\n")
	items, _ := Plan(cfg, home, st, "darwin", "")
	if _, err := Apply(items, home, &st, true, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".gitconfig")); string(got) == "drifted\n" {
		t.Error("--force did not overwrite the drifted .gitconfig")
	}
	if got, err := os.ReadFile(filepath.Join(home, ".zshrc")); err != nil || string(got) != "my edit\n" {
		t.Fatalf("$HOME .zshrc = %q, %v; --force without --prune must not delete", got, err)
	}
}

// rm prunes only its own file: other removed files stay put.
func TestPruneDeletesOnlySelectedFiles(t *testing.T) {
	cfg, home, st := removedFixture(t)
	mustRemove(t, filepath.Join(cfg.RepoDir, "base", ".gitconfig"))
	mustRemove(t, filepath.Join(cfg.RepoDir, "work", ".gitconfig"))
	items, _ := Plan(cfg, home, st, "darwin", "")
	only := func(rel string) bool { return rel == ".zshrc" }
	res, err := Apply(items, home, &st, false, only)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Deleted, []string{".zshrc"}) || !reflect.DeepEqual(res.Kept, []string{".gitconfig"}) {
		t.Fatalf("Deleted = %v, Kept = %v; want .zshrc deleted, .gitconfig kept", res.Deleted, res.Kept)
	}
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); err != nil {
		t.Fatal(".gitconfig deleted although it wasn't selected")
	}
}

// Plan records each side's modified time so diff can show which copy was
// touched more recently. A side that doesn't exist has the zero time.
func TestPlanRecordsModTimes(t *testing.T) {
	cfg, home := fixture(t)
	src := filepath.Join(cfg.RepoDir, "base", ".zshrc")
	repoTime := time.Date(2026, 9, 27, 9, 10, 0, 0, time.Local)
	homeTime := time.Date(2026, 9, 28, 14, 2, 0, 0, time.Local)
	if err := os.Chtimes(src, repoTime, repoTime); err != nil {
		t.Fatal(err)
	}

	items := plan(t, cfg, home, state.State{Files: map[string]string{}})
	if it := items[".zshrc"]; !it.SourceModTime.Equal(repoTime) || !it.CurrentModTime.IsZero() {
		t.Fatalf("create: source %v, current %v; want %v and zero", it.SourceModTime, it.CurrentModTime, repoTime)
	}

	mustWrite(t, filepath.Join(home, ".zshrc"), "home edit\n")
	if err := os.Chtimes(filepath.Join(home, ".zshrc"), homeTime, homeTime); err != nil {
		t.Fatal(err)
	}
	if it := plan(t, cfg, home, state.State{Files: map[string]string{}})[".zshrc"]; !it.CurrentModTime.Equal(homeTime) {
		t.Fatalf("current mod time = %v, want %v", it.CurrentModTime, homeTime)
	}

	// Removed: no layer provides it, so only the $HOME side has a time.
	mustRemove(t, src)
	st := state.State{Files: map[string]string{".zshrc": fsutil.Hash([]byte("base zshrc\n"))}}
	if it := plan(t, cfg, home, st)[".zshrc"]; !it.SourceModTime.IsZero() || !it.CurrentModTime.Equal(homeTime) {
		t.Fatalf("removed: source %v, current %v", it.SourceModTime, it.CurrentModTime)
	}
}

func TestDiffHeaders(t *testing.T) {
	repoTime := time.Date(2026, 9, 27, 9, 10, 0, 0, time.Local)
	homeTime := time.Date(2026, 9, 28, 14, 2, 0, 0, time.Local)
	both := func(st FileStatus) Item {
		return Item{Rel: ".zshrc", Status: st, SourceModTime: repoTime, CurrentModTime: homeTime}
	}
	cases := []struct {
		it       Item
		from, to string
	}{
		{both(Drifted), "home/.zshrc  2026-09-28 14:02  (drifted: $HOME has the newer edit)", "repo/.zshrc  2026-09-27 09:10"},
		{both(Update), "home/.zshrc  2026-09-28 14:02  (update: the repo has the newer edit)", "repo/.zshrc  2026-09-27 09:10"},
		{both(Conflict), "home/.zshrc  2026-09-28 14:02  (conflict: both changed, the times show which is more recent)", "repo/.zshrc  2026-09-27 09:10"},
		{both(Unmanaged), "home/.zshrc  2026-09-28 14:02  (unmanaged)", "repo/.zshrc  2026-09-27 09:10"},
		{Item{Rel: ".zshrc", Status: Create, SourceModTime: repoTime}, "home/.zshrc  (create)", "repo/.zshrc  2026-09-27 09:10"},
		{Item{Rel: ".zshrc", Status: Removed, CurrentModTime: homeTime}, "home/.zshrc  2026-09-28 14:02  (removed)", "repo/.zshrc"},
	}
	for _, c := range cases {
		from, to := DiffHeaders(c.it)
		if from != c.from || to != c.to {
			t.Errorf("%v:\n got  %q / %q\n want %q / %q", c.it.Status, from, to, c.from, c.to)
		}
	}
}
