package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupInstall lays down a fake strata footprint (binary, machine.toml,
// state.json, and an rc file with the installer's PATH line) under a temp
// home, and points the env overrides at them. Returns the paths.
func setupInstall(t *testing.T) (home, bin, machine, state, rc string) {
	t.Helper()
	tmp := t.TempDir()
	home = filepath.Join(tmp, "home")
	bin = filepath.Join(home, ".local", "bin", "strata")
	machine = filepath.Join(home, ".config", "strata", "machine.toml")
	state = filepath.Join(home, ".local", "state", "strata", "state.json")
	rc = filepath.Join(home, ".zshrc")

	for _, f := range []string{bin, machine, state} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Mirror exactly what install.sh appends.
	rcBody := "export EDITOR=vim\n\n# added by strata install.sh\nexport PATH=\"$HOME/.local/bin:$PATH\"\n"
	if err := os.WriteFile(rc, []byte(rcBody), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("STRATA_HOME", home)
	t.Setenv("STRATA_BIN", bin)
	t.Setenv("STRATA_CONFIG", machine)
	t.Setenv("STRATA_STATE", state)
	return
}

func TestUninstallDryRunRemovesNothing(t *testing.T) {
	_, bin, machine, state, rc := setupInstall(t)

	out, err := run(t, "uninstall", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dry run") {
		t.Fatalf("expected dry-run notice, got:\n%s", out)
	}
	for _, f := range []string{bin, machine, state, rc} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("dry-run deleted %s: %v", f, err)
		}
	}
}

func TestUninstallYesRemovesEverything(t *testing.T) {
	home, bin, machine, state, rc := setupInstall(t)

	out, err := run(t, "uninstall", "--yes")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}

	for _, f := range []string{bin, machine, state} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("expected %s gone, stat err = %v", f, err)
		}
	}
	// Empty strata dirs should be cleaned up too.
	for _, d := range []string{
		filepath.Join(home, ".config", "strata"),
		filepath.Join(home, ".local", "state", "strata"),
	} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("expected empty dir %s removed, stat err = %v", d, err)
		}
	}

	// rc file: keep the user's line, drop the installer's marker + export.
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("rc read: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "export EDITOR=vim") {
		t.Fatalf("uninstall clobbered the user's rc content:\n%s", body)
	}
	if strings.Contains(body, rcMarker) || strings.Contains(body, ".local/bin") {
		t.Fatalf("installer PATH line not fully removed:\n%s", body)
	}
}

func TestUninstallNothingToRemove(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0o755)
	t.Setenv("STRATA_HOME", home)
	t.Setenv("STRATA_BIN", filepath.Join(home, "nope"))
	t.Setenv("STRATA_CONFIG", filepath.Join(home, "nope.toml"))
	t.Setenv("STRATA_STATE", filepath.Join(home, "nope.json"))

	out, err := run(t, "uninstall", "--yes")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nothing to remove") {
		t.Fatalf("expected 'nothing to remove', got:\n%s", out)
	}
}

func TestUninstallDeclineAborts(t *testing.T) {
	_, bin, _, _, _ := setupInstall(t)

	root := newRootCmd()
	var buf strings.Builder
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"uninstall"})
	if err := root.Execute(); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "aborted") {
		t.Fatalf("expected abort, got:\n%s", buf.String())
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("decline should keep the binary: %v", err)
	}
}
