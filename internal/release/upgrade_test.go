package release

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"strata/internal/release/releasetest"
)

// fakeBinary stands in for a real strata build: a script that prints its
// version exactly like `strata --version` does.
func fakeBinary(version string) []byte {
	return []byte("#!/bin/sh\necho 'strata version " + version + "'\n")
}

func mustVersion(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// upgradeEnv is one test's installed binary plus a fake release server.
type upgradeEnv struct {
	opts   Options
	target string
}

// newEnv installs "OLD" at a temp target (version current) and serves rel.
// The fake binaries are sh scripts, so the flow runs on Unix hosts only.
func newEnv(t *testing.T, signer releasetest.Signer, rel releasetest.Release, goos, current string) upgradeEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake release binaries are sh scripts")
	}
	target := filepath.Join(t.TempDir(), "strata")
	if err := os.WriteFile(target, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	return upgradeEnv{
		target: target,
		opts: Options{
			APIURL:  releasetest.Serve(t, rel),
			GOOS:    goos,
			GOARCH:  "amd64",
			Current: mustVersion(t, current),
			Target:  target,
			Trusted: []string{signer.AuthorizedKey},
		},
	}
}

func (e upgradeEnv) targetContent(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(e.target)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// dirEntries lists the target's directory, to catch leftover temp files.
func (e upgradeEnv) dirEntries(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Dir(e.target))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ent := range ents {
		names = append(names, ent.Name())
	}
	return names
}

func TestUpgradeInstallsSignedNewerRelease(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	env := newEnv(t, s, rel, "linux", "2026.9.0")

	res, err := Upgrade(context.Background(), env.opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Upgraded || res.From.String() != "2026.9.0" || res.To.String() != "2026.9.1" {
		t.Errorf("result = %+v, want upgraded 2026.9.0 → 2026.9.1", res)
	}
	if got := env.targetContent(t); got != string(fakeBinary("2026.9.1")) {
		t.Errorf("target not replaced with the release binary: %q", got)
	}
	if fi, err := os.Stat(env.target); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("installed binary mode = %v (err %v), want 0755", fi.Mode().Perm(), err)
	}
	if got := env.dirEntries(t); len(got) != 1 {
		t.Errorf("leftover files next to the binary: %v", got)
	}
}

func TestUpgradeUpToDateIsANoop(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	env := newEnv(t, s, rel, "linux", "2026.9.1")

	res, err := Upgrade(context.Background(), env.opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upgraded {
		t.Error("reported an upgrade while already on the latest release")
	}
	if got := env.targetContent(t); got != "OLD" {
		t.Errorf("target changed while up to date: %q", got)
	}
}

func TestUpgradeForceReinstallsSameVersion(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	env := newEnv(t, s, rel, "linux", "2026.9.1")
	env.opts.Force = true

	if res, err := Upgrade(context.Background(), env.opts); err != nil || !res.Upgraded {
		t.Fatalf("Upgrade(--force) = %+v, %v; want a reinstall", res, err)
	}
	if got := env.targetContent(t); got != string(fakeBinary("2026.9.1")) {
		t.Errorf("--force didn't reinstall: %q", got)
	}
}

// A "latest" older than what's installed means a rollback - a yanked
// release, or someone re-serving an old signed release. Never install it,
// --force or not.
func TestUpgradeRefusesDowngradeEvenWithForce(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.8.0", "linux", "amd64", fakeBinary("2026.8.0"))
	env := newEnv(t, s, rel, "linux", "2026.9.1")
	env.opts.Force = true

	if _, err := Upgrade(context.Background(), env.opts); err == nil {
		t.Fatal("installed an older release")
	}
	if got := env.targetContent(t); got != "OLD" {
		t.Errorf("target changed on a refused downgrade: %q", got)
	}
}

// Every way a release can be wrong must stop the upgrade before the
// installed binary is touched, and leave no temp files behind.
func TestUpgradeFailsClosed(t *testing.T) {
	s := releasetest.NewSigner(t)
	other := releasetest.NewSigner(t)
	good := func(t *testing.T) releasetest.Release {
		return releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	}
	cases := map[string]func(t *testing.T) releasetest.Release{
		"unsigned release": func(t *testing.T) releasetest.Release {
			r := good(t)
			delete(r.Assets, "checksums.txt.sig")
			return r
		},
		"signed by an untrusted key": func(t *testing.T) releasetest.Release {
			r := good(t)
			r.Assets["checksums.txt.sig"] = other.Sign(r.Assets["checksums.txt"], Namespace)
			return r
		},
		"signed for another namespace": func(t *testing.T) releasetest.Release {
			r := good(t)
			r.Assets["checksums.txt.sig"] = s.Sign(r.Assets["checksums.txt"], "not-strata")
			return r
		},
		"archive tampered": func(t *testing.T) releasetest.Release {
			r := good(t)
			r.Assets[r.ArchiveName()] = append(r.Assets[r.ArchiveName()], 0)
			return r
		},
		"checksums tampered": func(t *testing.T) releasetest.Release {
			r := good(t)
			r.Assets["checksums.txt"] = append(r.Assets["checksums.txt"], []byte("0000  evil\n")...)
			return r
		},
		"no build for this platform": func(t *testing.T) releasetest.Release {
			r := good(t)
			delete(r.Assets, r.ArchiveName())
			return r
		},
		"archive without a strata binary": func(t *testing.T) releasetest.Release {
			r := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", nil)
			name := r.ArchiveName()
			noBin := tarGz(t, map[string]string{"README.md": "docs"})
			r.Assets[name] = noBin
			// Re-sign honestly so the failure is the missing binary, not a checksum.
			checksums := []byte(sha256Hex(noBin) + "  " + name + "\n")
			r.Assets["checksums.txt"] = checksums
			r.Assets["checksums.txt.sig"] = s.Sign(checksums, Namespace)
			return r
		},
		"binary fails the smoke test": func(t *testing.T) releasetest.Release {
			return releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("1999.1.0"))
		},
		// "version 2026.9.1" is a prefix of "version 2026.9.10": only an exact
		// version counts.
		"binary reports a longer version": func(t *testing.T) releasetest.Release {
			return releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.10"))
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			env := newEnv(t, s, mk(t), "linux", "2026.9.0")
			if _, err := Upgrade(context.Background(), env.opts); err == nil {
				t.Fatal("upgrade succeeded; want it refused")
			}
			if got := env.targetContent(t); got != "OLD" {
				t.Errorf("installed binary was modified: %q", got)
			}
			if got := env.dirEntries(t); len(got) != 1 {
				t.Errorf("leftover files next to the binary: %v", got)
			}
		})
	}
}

// Windows won't overwrite a running .exe: the old one is moved aside to
// .old, the new one takes its place, and CleanupOld removes the leftover on
// a later run. goos is a parameter, so this runs on any Unix host.
func TestUpgradeWindowsMovesOldBinaryAside(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "windows", "amd64", fakeBinary("2026.9.1"))
	env := newEnv(t, s, rel, "windows", "2026.9.0")

	if _, err := Upgrade(context.Background(), env.opts); err != nil {
		t.Fatal(err)
	}
	if got := env.targetContent(t); got != string(fakeBinary("2026.9.1")) {
		t.Errorf("target not replaced: %q", got)
	}
	old, err := os.ReadFile(env.target + ".old")
	if err != nil || string(old) != "OLD" {
		t.Fatalf("old binary not moved aside to .old (%q, %v)", old, err)
	}
	CleanupOld(env.target)
	if _, err := os.Stat(env.target + ".old"); !os.IsNotExist(err) {
		t.Error("CleanupOld left the .old binary behind")
	}
}

func TestLatestReportsWhetherNewer(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	for current, wantNewer := range map[string]bool{"2026.9.0": true, "2026.9.1": false} {
		env := newEnv(t, s, rel, "linux", current)
		latest, newer, err := Latest(context.Background(), env.opts)
		if err != nil || latest.String() != "2026.9.1" || newer != wantNewer {
			t.Errorf("Latest(current %s) = %v, %v, %v; want 2026.9.1, %v", current, latest, newer, err, wantNewer)
		}
	}
}

func TestUpgradeReportsHTTPErrors(t *testing.T) {
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, Namespace, "2026.9.1", "linux", "amd64", fakeBinary("2026.9.1"))
	env := newEnv(t, s, rel, "linux", "2026.9.0")
	env.opts.APIURL += "-nope" // 404
	_, err := Upgrade(context.Background(), env.opts)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want it to report the 404", err)
	}
}
