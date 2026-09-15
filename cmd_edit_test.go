package main

import (
	"os"
	"path/filepath"
	"reflect"
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

// Windows has no sh to split the editor setting, and splitting on spaces
// broke any editor under "C:\Program Files\". Double quotes group, and a
// setting that is itself an existing file is used whole. Pure string work,
// so it runs on every OS.
func TestWindowsEditorArgs(t *testing.T) {
	spaced := filepath.Join(t.TempDir(), "Program Files", "ed.exe")
	writeFile(t, spaced, "")
	cases := []struct {
		editor string
		want   []string
	}{
		{"notepad", []string{"notepad", "f"}},
		{"code --wait", []string{"code", "--wait", "f"}},
		{`"C:\Program Files\Notepad++\notepad++.exe" -multiInst`, []string{`C:\Program Files\Notepad++\notepad++.exe`, "-multiInst", "f"}},
		{spaced, []string{spaced, "f"}},
	}
	for _, c := range cases {
		if got := windowsEditorArgs(c.editor, "f"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("windowsEditorArgs(%q) = %q, want %q", c.editor, got, c.want)
		}
	}
}
