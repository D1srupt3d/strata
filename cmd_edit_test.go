package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeEditor writes a script that records its arguments and replaces the
// file it was given (its last argument) with "edited\n".
func fakeEditor(t *testing.T) (path, argLog string) {
	t.Helper()
	dir := t.TempDir()
	argLog = filepath.Join(dir, "args")
	path = filepath.Join(dir, "ed")
	script := "#!/bin/sh\n" +
		"echo \"$@\" > '" + argLog + "'\n" +
		"for last; do :; done\n" +
		"printf 'edited\\n' > \"$last\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argLog
}

func editSandbox(t *testing.T) sandboxEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake editor is a sh script")
	}
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "old\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	return s
}

// GUI editors need a wait flag ("code --wait"), so $EDITOR may hold
// arguments; strata used to treat the whole string as the program name.
// Answering "y" at the prompt must then apply the edit.
func TestEditRunsEditorWithArguments(t *testing.T) {
	s := editSandbox(t)
	ed, argLog := fakeEditor(t)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", ed+" --wait")

	if out, err := runIn(t, "y\n", "edit", ".zshrc"); err != nil {
		t.Fatalf("edit: %v\n%s", err, out)
	}
	if got := readFile(t, argLog); !strings.Contains(got, "--wait") {
		t.Errorf("editor got args %q; want --wait passed through", got)
	}
	if got := readFile(t, s.home(".zshrc")); got != "edited\n" {
		t.Errorf("answering y should apply the edit; $HOME .zshrc = %q", got)
	}
}

// $VISUAL wins over $EDITOR — the order git and most tools use.
func TestEditPrefersVisualOverEditor(t *testing.T) {
	s := editSandbox(t)
	ed, _ := fakeEditor(t)
	t.Setenv("VISUAL", ed)
	t.Setenv("EDITOR", "false") // would fail the edit if it were used

	if out, err := runIn(t, "n\n", "edit", ".zshrc"); err != nil {
		t.Fatalf("edit: %v\n%s", err, out)
	}
	if got := readFile(t, s.repo("base/.zshrc")); got != "edited\n" {
		t.Errorf("$VISUAL editor didn't run; source = %q", got)
	}
}
