package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"strata/internal/release"
	"strata/internal/release/releasetest"
)

// fakeStrata stands in for a release binary: a script that prints its
// version the way `strata --version` does.
func fakeStrata(version string) []byte {
	return []byte("#!/bin/sh\necho 'strata version " + version + "'\n")
}

// setVar overrides a package-level var for one test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// upgradeSandbox serves a signed fake release of latest, installs "OLD" as
// the binary strata would replace (via STRATA_BIN), and sets the running
// build's version and channel. Returns the binary's path.
func upgradeSandbox(t *testing.T, current, latest, ch string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake release binaries are sh scripts")
	}
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, release.Namespace, latest, runtime.GOOS, runtime.GOARCH, fakeStrata(latest))
	t.Setenv("STRATA_RELEASE_API", releasetest.Serve(t, rel))
	bin := filepath.Join(t.TempDir(), "strata")
	writeFile(t, bin, "OLD")
	t.Setenv("STRATA_BIN", bin)
	setVar(t, &version, current)
	setVar(t, &channel, ch)
	setVar(t, &upgradeKeys, []string{s.AuthorizedKey})
	return bin
}

func TestUpgradeCommandReplacesReleaseBinary(t *testing.T) {
	bin := upgradeSandbox(t, "2026.9.0", "2026.9.1", "release")
	out, err := run(t, "upgrade")
	if err != nil {
		t.Fatalf("upgrade: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2026.9.0 → 2026.9.1") {
		t.Errorf("output doesn't report the upgrade:\n%s", out)
	}
	if got := readFile(t, bin); got != string(fakeStrata("2026.9.1")) {
		t.Errorf("binary not replaced: %q", got)
	}
}

// "update" is an alias - the word people reach for first.
func TestUpdateIsAnAliasForUpgrade(t *testing.T) {
	bin := upgradeSandbox(t, "2026.9.0", "2026.9.1", "release")
	if out, err := run(t, "update"); err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	if got := readFile(t, bin); got != string(fakeStrata("2026.9.1")) {
		t.Errorf("binary not replaced: %q", got)
	}
}

// A binary built from source is its owner's to manage; upgrade must never
// swap it for a release build.
func TestUpgradeRefusesSourceBuilds(t *testing.T) {
	bin := upgradeSandbox(t, "2026.9.0", "2026.9.1", "source")
	_, err := run(t, "upgrade", "--force")
	if err == nil {
		t.Fatal("upgraded a source build")
	}
	if !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "install.sh") {
		t.Errorf("error should say it's a source build and how to update it: %v", err)
	}
	if got := readFile(t, bin); got != "OLD" {
		t.Errorf("source build was modified: %q", got)
	}
}

func TestUpgradeRefusesHomebrewInstalls(t *testing.T) {
	upgradeSandbox(t, "2026.9.0", "2026.9.1", "release")
	bin := filepath.Join(t.TempDir(), "Cellar", "strata", "2026.9.0", "bin", "strata")
	writeFile(t, bin, "OLD")
	t.Setenv("STRATA_BIN", bin)

	_, err := run(t, "upgrade")
	if err == nil || !strings.Contains(err.Error(), "brew upgrade strata") {
		t.Fatalf("err = %v, want it to defer to 'brew upgrade strata'", err)
	}
	if got := readFile(t, bin); got != "OLD" {
		t.Errorf("Homebrew-owned binary was modified: %q", got)
	}
}

func TestIsHomebrewPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/opt/homebrew/Cellar/strata/2026.9.1/bin/strata":              true,
		"/usr/local/Cellar/strata/2026.9.1/bin/strata":                 true,
		"/home/linuxbrew/.linuxbrew/Cellar/strata/2026.9.1/bin/strata": true,
		"/opt/homebrew/bin/strata":                                     true,
		"/Users/you/.local/bin/strata":                                 false,
		"/usr/local/bin/strata":                                        false,
	} {
		if got := isHomebrewPath(path); got != want {
			t.Errorf("isHomebrewPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// --check is a question: exit 1 when an update exists (like `status` when
// something needs attention), 0 when current - and it never installs.
func TestUpgradeCheckExitCodes(t *testing.T) {
	bin := upgradeSandbox(t, "2026.9.0", "2026.9.1", "release")
	out, err := run(t, "upgrade", "--check")
	if code, msg := exitStatus(err); code != 1 || msg != "" {
		t.Fatalf("--check with an update: exit %d %q, want 1 and no error line\n%s", code, msg, out)
	}
	if !strings.Contains(out, "2026.9.1") {
		t.Errorf("--check doesn't name the new version:\n%s", out)
	}
	if got := readFile(t, bin); got != "OLD" {
		t.Errorf("--check modified the binary: %q", got)
	}

	upgradeSandbox(t, "2026.9.1", "2026.9.1", "release")
	if out, err := run(t, "upgrade", "--check"); err != nil {
		t.Errorf("--check when current: %v\n%s", err, out)
	}
}

// A source build can still ask what the latest release is.
func TestUpgradeCheckReportsForSourceBuilds(t *testing.T) {
	upgradeSandbox(t, "2026.9.0-3-gabc123", "2026.9.1", "source")
	out, err := run(t, "upgrade", "--check")
	if err != nil {
		t.Fatalf("--check on a source build: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2026.9.1") || !strings.Contains(out, "source") {
		t.Errorf("want the latest release and a source-build note:\n%s", out)
	}
}

// The stale binary Windows leaves behind is cleaned up on the next upgrade.
func TestUpgradeRemovesLeftoverOldBinary(t *testing.T) {
	bin := upgradeSandbox(t, "2026.9.1", "2026.9.1", "release")
	writeFile(t, bin+".old", "stale")
	if _, err := run(t, "upgrade"); err != nil {
		t.Fatal(err)
	}
	if exists(bin + ".old") {
		t.Error("leftover .old binary not removed")
	}
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
}
