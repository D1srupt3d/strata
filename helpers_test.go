package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Shared scaffolding for the CLI tests. Every test runs in its own
// t.TempDir() sandbox wired up through the STRATA_* env vars, so nothing
// ever touches the real $HOME. Helpers fail the test on any setup error -
// a fixture that silently didn't get written makes failures unreadable.

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sandboxEnv is one test's isolated repo + home + config + state.
type sandboxEnv struct {
	Root, Home, Repo, Machine, State string
}

func (s sandboxEnv) repo(rel string) string { return filepath.Join(s.Repo, filepath.FromSlash(rel)) }
func (s sandboxEnv) home(rel string) string { return filepath.Join(s.Home, filepath.FromSlash(rel)) }

// sandbox creates an empty repo/ and home/, points strata at them, and
// writes a machine.toml selecting the given role layers.
func sandbox(t *testing.T, roleLayers ...string) sandboxEnv {
	t.Helper()
	root := t.TempDir()
	s := sandboxEnv{
		Root:    root,
		Home:    filepath.Join(root, "home"),
		Repo:    filepath.Join(root, "repo"),
		Machine: filepath.Join(root, "machine.toml"),
		State:   filepath.Join(root, "state.json"),
	}
	for _, d := range []string{s.Home, s.Repo} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("STRATA_HOME", s.Home)
	t.Setenv("STRATA_CONFIG", s.Machine)
	t.Setenv("STRATA_STATE", s.State)
	quoted := make([]string, len(roleLayers))
	for i, l := range roleLayers {
		quoted[i] = strconv.Quote(l)
	}
	writeFile(t, s.Machine, fmt.Sprintf("repo = %q\nlayers = [%s]\n",
		filepath.ToSlash(s.Repo), strings.Join(quoted, ", ")))
	return s
}

// runIn is run (e2e_test.go) with stdin, for commands that prompt.
func runIn(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// isolateGit makes git inside tests ignore the developer's own config - no
// commit signing (which would prompt a password manager), no global hooks -
// and gives commits a fixed identity.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	empty := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, empty, "")
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "strata-test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.invalid")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
