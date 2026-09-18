package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// env is one test's isolated machine. newEnv builds a setup where every
// check passes; each test then breaks exactly the thing it checks.
type env struct {
	in   Inputs
	repo string
}

// newEnv creates, under t.TempDir(): a home folder, a git repo with base/
// and a folder per role layer, a machine.toml selecting those roles, a
// strata binary, and Inputs whose fake PATH finds that binary and git.
func newEnv(t *testing.T, roles ...string) env {
	t.Helper()
	root := t.TempDir()
	e := env{repo: filepath.Join(root, "repo")}
	bin := filepath.Join(root, "bin", "strata")
	e.in = Inputs{
		Home:        filepath.Join(root, "home"),
		MachinePath: filepath.Join(root, "machine.toml"),
		StatePath:   filepath.Join(root, "state.json"),
		Bin:         bin,
		GOOS:        "darwin",
		Version:     "2026.9.9",
		Channel:     "source",
		LookPath:    fakePath(map[string]string{"strata": bin, "git": "/usr/bin/git"}),
	}
	mustWrite(t, bin, "")
	mustMkdir(t, e.in.Home)
	mustMkdir(t, filepath.Join(e.repo, ".git"))
	mustMkdir(t, filepath.Join(e.repo, "base"))
	quoted := make([]string, len(roles))
	for i, role := range roles {
		mustMkdir(t, filepath.Join(e.repo, role))
		quoted[i] = strconv.Quote(role)
	}
	e.machineToml(t, strings.Join(quoted, ", "))
	return e
}

// machineToml (re)writes machine.toml with this env's repo and a raw
// layers list, e.g. `"work", "../x"`.
func (e env) machineToml(t *testing.T, layerList string) {
	t.Helper()
	mustWrite(t, e.in.MachinePath, fmt.Sprintf("repo = %q\nlayers = [%s]\n", filepath.ToSlash(e.repo), layerList))
}

func (e env) repoFile(t *testing.T, rel, content string) {
	t.Helper()
	mustWrite(t, filepath.Join(e.repo, filepath.FromSlash(rel)), content)
}

func (e env) homeFile(t *testing.T, rel, content string) {
	t.Helper()
	mustWrite(t, filepath.Join(e.in.Home, filepath.FromSlash(rel)), content)
}

func (e env) stateFile(t *testing.T, content string) {
	t.Helper()
	mustWrite(t, e.in.StatePath, content)
}

// fakePath returns a LookPath that finds only the given commands.
func fakePath(found map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if p, ok := found[name]; ok {
			return p, nil
		}
		return "", exec.ErrNotFound
	}
}

// get returns the finding in group with exactly this subject, failing the
// test (and printing every finding) when there is none.
func get(t *testing.T, got []Finding, group, subject string) Finding {
	t.Helper()
	for _, f := range got {
		if f.Group == group && f.Subject == subject {
			return f
		}
	}
	t.Fatalf("no %s finding with subject %s in:\n%s", group, subject, dump(got))
	return Finding{}
}

// wantSev is get plus a severity check. Every Warn and Error must carry a
// fix, so it checks that too.
func wantSev(t *testing.T, got []Finding, group, subject string, sev Severity) Finding {
	t.Helper()
	f := get(t, got, group, subject)
	if f.Sev != sev {
		t.Errorf("%s %s: severity %v, want %v\n%s", group, subject, f.Sev, sev, dump(got))
	}
	if (sev == Warn || sev == Error) && f.Fix == "" {
		t.Errorf("%s %s: %v finding has no fix", group, subject, sev)
	}
	return f
}

// absent fails if group has a finding with this subject.
func absent(t *testing.T, got []Finding, group, subject string) {
	t.Helper()
	for _, f := range got {
		if f.Group == group && f.Subject == subject {
			t.Errorf("unexpected %s finding %s:\n%s", group, subject, dump(got))
		}
	}
}

// problems returns the Warn and Error findings.
func problems(got []Finding) []Finding {
	var out []Finding
	for _, f := range got {
		if f.Sev == Warn || f.Sev == Error {
			out = append(out, f)
		}
	}
	return out
}

func dump(got []Finding) string {
	var b strings.Builder
	for _, f := range got {
		fmt.Fprintf(&b, "  [%s] %-5s %s: %s | fix: %s\n", f.Group, f.Sev, f.Subject, f.Detail, f.Fix)
	}
	return b.String()
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func mustRemoveAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
