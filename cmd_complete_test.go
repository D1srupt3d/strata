package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// complete runs Cobra's hidden __complete command, which is what the shell
// calls on every Tab, and returns the suggestions and the directive line.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	out, err := run(t, append([]string{"__complete"}, args...)...)
	if err != nil {
		t.Fatalf("__complete %v: %v\n%s", args, err, out)
	}
	var got []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, ":") {
			return got, line
		}
		got = append(got, line)
	}
	t.Fatalf("no directive line in:\n%s", out)
	return nil, ""
}

// edit and rm complete to managed files, filtered by what's typed so far.
// A file only in $HOME isn't offered: neither command can act on it.
// ":4" is ShellCompDirectiveNoFileComp - no fallback to the shell's own
// file names, which would offer unmanaged files.
func TestCompleteManagedFiles(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.repo("base/.zprofile"), "x\n")
	writeFile(t, s.repo("work/.gitconfig"), "x\n")
	writeFile(t, s.home(".bashrc"), "not managed\n")

	for _, cmd := range []string{"edit", "rm"} {
		got, dir := complete(t, cmd, ".z")
		if want := []string{".zprofile", ".zshrc"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s .z<Tab> = %q, want %q", cmd, got, want)
		}
		if dir != ":4" {
			t.Errorf("%s directive = %s, want :4 (no file fallback)", cmd, dir)
		}
		if got, _ := complete(t, cmd, ""); len(got) != 3 {
			t.Errorf("%s <Tab> = %q, want the 3 managed files", cmd, got)
		}
	}
}

// Both commands take exactly one file, so a second Tab offers nothing.
func TestCompleteOnlyFirstArg(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	if got, _ := complete(t, "edit", ".zshrc", ""); len(got) != 0 {
		t.Errorf("second argument completed to %q, want nothing", got)
	}
}

// A removed file has no layer left to edit or delete it from.
func TestCompleteSkipsRemovedFiles(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.repo("base/.vimrc"), "x\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	removeFile(t, s.repo("base/.vimrc"))
	if got, _ := complete(t, "rm", ""); !reflect.DeepEqual(got, []string{".zshrc"}) {
		t.Errorf("rm <Tab> = %q, want only .zshrc", got)
	}
}

// Without a machine.toml there's nothing to offer, and Tab must stay quiet
// rather than print an error into the user's command line.
func TestCompleteWithoutSetupIsQuiet(t *testing.T) {
	s := sandbox(t)
	removeFile(t, s.Machine)
	got, dir := complete(t, "edit", "")
	if len(got) != 0 || dir != ":4" {
		t.Errorf("unconfigured: got %q %s, want nothing and :4", got, dir)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// diff takes several files, so each Tab offers the managed files not
// already on the line.
func TestCompleteDiffSeveralFiles(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.repo("base/.vimrc"), "x\n")
	got, dir := complete(t, "diff", ".zshrc", "")
	if !reflect.DeepEqual(got, []string{".vimrc"}) || dir != ":4" {
		t.Errorf("diff .zshrc <Tab> = %q %s, want [.vimrc] :4", got, dir)
	}
}
