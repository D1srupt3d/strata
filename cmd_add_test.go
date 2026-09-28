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

// --layer with a name that is neither a folder in the repo nor one of this
// machine's layers is a typo; it used to create a brand-new layer folder.
func TestAddRefusesUnknownLayer(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.home(".new"), "new\n")
	_, err := run(t, "add", ".new", "--layer", "wrok")
	if err == nil || !strings.Contains(err.Error(), "wrok") {
		t.Fatalf("add --layer wrok: err = %v, want one naming the layer", err)
	}
	if exists(s.repo("wrok")) {
		t.Error("add created a wrok/ layer folder")
	}
}

// A role layer listed in machine.toml may not have a folder yet; add is how
// its first file gets there.
func TestAddCanStartThisMachinesRoleLayer(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.home(".new"), "new\n")
	if out, err := run(t, "add", ".new", "--layer", "work"); err != nil {
		t.Fatalf("add --layer work: %v\n%s", err, out)
	}
	if got := readFile(t, s.repo("work/.new")); got != "new\n" {
		t.Errorf("work/.new = %q", got)
	}
}

// Adding into a layer this machine doesn't use (another OS, another role)
// must not make strata manage the file here: recording it in state made the
// next apply see it as "removed" and delete it from $HOME.
func TestAddToAnotherMachinesLayerLeavesHomeAlone(t *testing.T) {
	s := sandbox(t) // no role layers
	writeFile(t, s.repo("work/.x"), "a work-only file\n")
	writeFile(t, s.home(".new"), "new\n")
	out, err := run(t, "add", ".new", "--layer", "work")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if got := readFile(t, s.repo("work/.new")); got != "new\n" {
		t.Errorf("work/.new = %q", got)
	}
	if !containsLine(out, "work", "this machine") {
		t.Errorf("no note that work isn't one of this machine's layers:\n%s", out)
	}
	if out, err := run(t, "apply"); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if !exists(s.home(".new")) {
		t.Fatal("apply deleted $HOME/.new")
	}
}

// Adding into a layer that a later layer overrides here: the $HOME copy used
// to be recorded as applied, so the next apply "updated" it back to the
// winning layer's version. The edit must stay protected (drifted), with a
// warning that says which layer wins.
func TestAddIntoAShadowedLayerKeepsTheHomeEdit(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("base/.cfg"), "base\n")
	writeFile(t, s.repo("work/.cfg"), "work\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".cfg"), "my edit\n")
	out, err := run(t, "add", ".cfg", "--layer", "base")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if got := readFile(t, s.repo("base/.cfg")); got != "my edit\n" {
		t.Errorf("base/.cfg = %q, want the $HOME edit", got)
	}
	if !containsLine(out, "work", "wins") {
		t.Errorf("no warning that work/ wins on this machine:\n%s", out)
	}
	_, _ = run(t, "apply") // refuses: the edit reads as drifted
	if got := readFile(t, s.home(".cfg")); got != "my edit\n" {
		t.Errorf("apply replaced the $HOME edit with %q", got)
	}
}

// --layer names the target explicitly, so add needs no plan and must keep
// working even while an unrelated file breaks planning - it's the escape
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

// Several files in one call each get adopted and recorded, so a follow-up
// status is clean - the same result as adding them one by one.
func TestAddSeveralFiles(t *testing.T) {
	s := sandbox(t)
	for _, f := range []string{".a", ".b", ".config/c"} {
		writeFile(t, s.home(f), f+"\n")
	}
	out, err := run(t, "add", ".a", "~/.b", s.home(".config/c"))
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	for _, f := range []string{".a", ".b", ".config/c"} {
		if got := readFile(t, s.repo("base/"+f)); got != f+"\n" {
			t.Errorf("base/%s = %q", f, got)
		}
		if !containsLine(out, "added", f) {
			t.Errorf("no 'added %s' line:\n%s", f, out)
		}
	}
	if out, err := run(t, "status"); err != nil {
		t.Errorf("status after add: %v, want clean\n%s", err, out)
	}
}

// One bad path must stop the whole add before anything is written - a typo
// in the third argument shouldn't leave the first two half-adopted.
func TestAddSeveralFilesIsAllOrNothing(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.home(".a"), "a\n")
	for _, bad := range []string{".missing", filepath.Join(s.Root, "outside")} {
		writeFile(t, filepath.Join(s.Root, "outside"), "x\n")
		if _, err := run(t, "add", ".a", bad); err == nil {
			t.Errorf("add .a %s succeeded; want an error", bad)
		}
		if exists(s.repo("base/.a")) {
			t.Fatalf("add .a %s wrote base/.a before failing", bad)
		}
	}
}

// Without --layer each file goes to the layer that wins for it, so one call
// can absorb a work-only edit and adopt a new file into base/.
func TestAddSeveralFilesEachToItsWinningLayer(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("work/.gitconfig"), "work\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".gitconfig"), "edited work\n")
	writeFile(t, s.home(".new"), "new\n")
	if out, err := run(t, "add", ".gitconfig", ".new"); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if got := readFile(t, s.repo("work/.gitconfig")); got != "edited work\n" {
		t.Errorf("work/.gitconfig = %q", got)
	}
	if exists(s.repo("base/.gitconfig")) {
		t.Error("the work-only .gitconfig leaked into base/")
	}
	if got := readFile(t, s.repo("base/.new")); got != "new\n" {
		t.Errorf("base/.new = %q", got)
	}
	if out, err := run(t, "status"); err != nil {
		t.Errorf("status after add: %v, want clean\n%s", err, out)
	}
}

// --layer applies to every file in the call.
func TestAddSeveralFilesWithLayer(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.home(".a"), "a\n")
	writeFile(t, s.home(".b"), "b\n")
	if out, err := run(t, "add", ".a", ".b", "--layer", "work"); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	for _, f := range []string{".a", ".b"} {
		if !exists(s.repo("work/" + f)) {
			t.Errorf("work/%s missing", f)
		}
		if exists(s.repo("base/" + f)) {
			t.Errorf("base/%s written despite --layer work", f)
		}
	}
}

// The same file named two ways is added once, not twice.
func TestAddSameFileTwiceAddsOnce(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.home(".a"), "a\n")
	out, err := run(t, "add", ".a", "~/.a")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if n := strings.Count(out, "added"); n != 1 {
		t.Errorf("%d 'added' lines, want 1:\n%s", n, out)
	}
}
