package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAllOK(t *testing.T) {
	e := newEnv(t, "work")
	got := Run(e.in)
	for _, subject := range []string{"machine.toml", "repo", "git", "dots.toml", `layer "work"`} {
		wantSev(t, got, "config", subject, OK)
	}
	// No dots.toml is valid: a repo without config uses the defaults.
	if f := get(t, got, "config", "dots.toml"); !strings.Contains(f.Detail, "defaults") {
		t.Errorf("missing dots.toml detail = %q, want it to mention defaults", f.Detail)
	}
}

// Without machine.toml nothing else in config can be found, so those checks
// are skipped by name rather than silently missing.
func TestMissingMachineTomlIsErrorAndSkipsTheRest(t *testing.T) {
	e := newEnv(t)
	mustRemove(t, e.in.MachinePath)
	got := Run(e.in)
	if f := wantSev(t, got, "config", "machine.toml", Error); !strings.Contains(f.Fix, "strata init") {
		t.Errorf("fix = %q, want it to suggest 'strata init'", f.Fix)
	}
	wantSev(t, got, "config", "repo, git, dots.toml, layers", Skip)
}

// A machine.toml that exists but is broken needs fixing, not re-initing.
func TestBrokenMachineTomlIsError(t *testing.T) {
	e := newEnv(t)
	mustWrite(t, e.in.MachinePath, "repo = \n")
	if f := wantSev(t, Run(e.in), "config", "machine.toml", Error); strings.Contains(f.Fix, "init") {
		t.Errorf("fix = %q; a broken file should be fixed, not re-inited", f.Fix)
	}
}

// Every command reads a missing repo as an empty one, so every managed file
// reads as removed. Doctor must call that out.
func TestMissingRepoIsError(t *testing.T) {
	e := newEnv(t)
	mustRemoveAll(t, e.repo)
	got := Run(e.in)
	if f := wantSev(t, got, "config", "repo", Error); !strings.Contains(f.Detail, "removed") {
		t.Errorf("detail = %q, want it to explain files would read as removed", f.Detail)
	}
	wantSev(t, got, "config", "git, dots.toml, layers", Skip)
}

// A repo path that exists but isn't a readable folder makes every command
// that reads it fail outright, not read it as empty — unlike a missing repo.
func TestRepoThatIsAFileIsError(t *testing.T) {
	e := newEnv(t)
	mustRemoveAll(t, e.repo)
	mustWrite(t, e.repo, "not a folder")
	f := wantSev(t, Run(e.in), "config", "repo", Error)
	if strings.Contains(f.Detail, "removed") {
		t.Errorf("detail = %q, should not claim managed files would read as removed", f.Detail)
	}
}

func TestRepoWithoutGitWarns(t *testing.T) {
	e := newEnv(t)
	mustRemoveAll(t, filepath.Join(e.repo, ".git"))
	wantSev(t, Run(e.in), "config", "git", Warn)
}

// strata init and sync both work when the dotfiles repo is a subfolder of a
// git repo; doctor must not tell someone to 'git init' inside it, which
// would create a nested repo.
func TestRepoInsideParentGitRepoIsOK(t *testing.T) {
	e := newEnv(t)
	mustRemoveAll(t, filepath.Join(e.repo, ".git"))
	mustMkdir(t, filepath.Join(filepath.Dir(e.repo), ".git"))
	wantSev(t, Run(e.in), "config", "git", OK)
}

func TestUnparseableDotsTomlIsError(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "dots.toml", "[hook]\n\".zshrc\" = \"echo hi\"\n") // typo: [hook]
	if f := wantSev(t, Run(e.in), "config", "dots.toml", Error); !strings.Contains(f.Detail, "hook") {
		t.Errorf("detail = %q, want it to name the unknown key", f.Detail)
	}
}

// Apply stops at the first bad layer; doctor lists every one.
func TestEveryBadRoleLayerIsListed(t *testing.T) {
	e := newEnv(t, "work")
	e.machineToml(t, `"work", "wrok", "../x"`)
	got := Run(e.in)
	wantSev(t, got, "config", `layer "work"`, OK)
	wantSev(t, got, "config", `layer "wrok"`, Error)
	wantSev(t, got, "config", `layer "../x"`, Error)
}
