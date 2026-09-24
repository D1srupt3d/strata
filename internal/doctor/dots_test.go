package doctor

import (
	"strings"
	"testing"
)

func TestDotsChecksSkipWhenConfigDidNotLoad(t *testing.T) {
	e := newEnv(t)
	mustRemove(t, e.in.MachinePath)
	wantSev(t, Run(e.in), "dots.toml", "checks", Skip)
}

// An entry naming a file no layer has is silently ignored by apply.
func TestEntriesNamingNoFileWarn(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "base/.zshrc", "x\n")
	e.repoFile(t, "dots.toml", `substitute = [".zshrc", ".nope"]
[hooks]
".zshrc" = "echo ok"
".gone" = "echo never"
[permissions]
".zshrc" = "600"
".ssh/*" = "600"
`)
	got := Run(e.in)
	wantSev(t, got, "dots.toml", `substitute ".nope"`, Warn)
	wantSev(t, got, "dots.toml", `hook ".gone"`, Warn)
	wantSev(t, got, "dots.toml", `permission ".ssh/*"`, Warn)
	for _, subject := range []string{`substitute ".zshrc"`, `hook ".zshrc"`, `permission ".zshrc"`} {
		absent(t, got, "dots.toml", subject)
	}
}

// "Any layer" is every folder in the repo, not this machine's stack: a hook
// for a mac-only file does its job on the Mac, even when doctor runs on Linux.
func TestOtherOSFileIsNotANoOp(t *testing.T) {
	e := newEnv(t)
	e.in.GOOS = "linux"
	e.repoFile(t, "mac/.config/aerospace.toml", "x\n")
	e.repoFile(t, "dots.toml", "substitute = [\".config/aerospace.toml\"]\n[hooks]\n\".config/aerospace.toml\" = \"aerospace reload-config\"\n")
	got := Run(e.in)
	wantSev(t, got, "dots.toml", "substitute", OK)
	wantSev(t, got, "dots.toml", "hooks", OK)
}

// A broken ignore glob makes the file walk unreliable, so the checks built
// on it are skipped rather than reporting bogus no-ops.
func TestInvalidIgnorePatternIsErrorAndSkipsFileChecks(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "dots.toml", "ignore = [\"[unclosed\"]\n[hooks]\n\".nope\" = \"echo\"\n")
	got := Run(e.in)
	wantSev(t, got, "dots.toml", `ignore "[unclosed"`, Error)
	wantSev(t, got, "dots.toml", "substitute, hooks, permissions, vars", Skip)
	absent(t, got, "dots.toml", `hook ".nope"`)
}

func TestInvalidPermissionPatternIsError(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "base/.zshrc", "x\n")
	e.repoFile(t, "dots.toml", "[permissions]\n\"[x\" = \"600\"\n")
	got := Run(e.in)
	wantSev(t, got, "dots.toml", `permission "[x"`, Error)
	wantSev(t, got, "dots.toml", "file modes", Skip)
}

// Equal-length patterns that disagree make apply fail on that file.
func TestEquallySpecificPermissionsThatDisagreeAreError(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "base/.ssh/config", "x\n")
	e.repoFile(t, "dots.toml", "[permissions]\n\".ssh/co*\" = \"600\"\n\"*/config\" = \"644\"\n") // both 8 chars
	f := wantSev(t, Run(e.in), "dots.toml", `mode of ".ssh/config"`, Error)
	if !strings.Contains(f.Detail, ".ssh/co*") || !strings.Contains(f.Detail, "*/config") {
		t.Errorf("detail = %q, want it to name both patterns", f.Detail)
	}
}

// Apply stops at the first undefined var; doctor lists them all, for the
// file this machine actually gets (work's copy beats base's).
func TestEveryUndefinedVarIsListed(t *testing.T) {
	e := newEnv(t, "work")
	e.repoFile(t, "base/.gitconfig", "{{name}}\n")
	e.repoFile(t, "work/.gitconfig", "{{name}} {{email}} {{host}} {{email}}\n")
	e.repoFile(t, "dots.toml", "substitute = [\".gitconfig\"]\n[vars]\nname = \"me\"\n")
	f := wantSev(t, Run(e.in), "dots.toml", `vars in ".gitconfig"`, Error)
	if !strings.Contains(f.Detail, "email, host") {
		t.Errorf("detail = %q, want every undefined var: email, host", f.Detail)
	}
	if !strings.Contains(f.Fix, "[layer_vars.") {
		t.Errorf("fix = %q, want it to mention [layer_vars.<layer>] as a place to define vars", f.Fix)
	}
}

// Another OS's copy is substituted on that OS, with that machine's vars.
func TestOtherOSUndefinedVarIsNotReported(t *testing.T) {
	e := newEnv(t)
	e.in.GOOS = "linux"
	e.repoFile(t, "mac/.gitconfig", "{{mac_only}}\n")
	e.repoFile(t, "dots.toml", "substitute = [\".gitconfig\"]\n")
	wantSev(t, Run(e.in), "dots.toml", "vars", OK)
}

// A role layer like "../x" would resolve outside the repo, so the vars check
// (which resolves this machine's stack) must not run.
func TestVarsSkipWhenARoleLayerIsBad(t *testing.T) {
	e := newEnv(t)
	e.machineToml(t, `"../x"`)
	wantSev(t, Run(e.in), "dots.toml", "vars", Skip)
}
