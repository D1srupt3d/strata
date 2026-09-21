package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"strata/internal/release"
	"strata/internal/release/releasetest"
)

// These run the real get.sh with sh, against a fake GitHub release. Because
// get.sh checks signatures with the real `ssh-keygen -Y verify`, they also
// prove the whole chain works with stock OpenSSH.

func needTools(t *testing.T, tools ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("get.sh is for macOS and Linux")
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

// runGetSh runs get.sh against rel with a throwaway HOME and install dir.
// Returns the install dir, HOME, combined output, and the run error.
// extraEnv entries override the defaults (the last value for a key wins).
func runGetSh(t *testing.T, s releasetest.Signer, rel releasetest.Release, extraEnv ...string) (binDir, home, out string, err error) {
	t.Helper()
	home = t.TempDir()
	binDir = filepath.Join(home, "bin")
	cmd := exec.Command("sh", "get.sh")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"SHELL=/bin/zsh",
		"STRATA_RELEASE_API="+releasetest.Serve(t, rel),
		"STRATA_BIN_DIR="+binDir,
		"STRATA_GET_TRUSTED_KEY="+s.AuthorizedKey,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	b, err := cmd.CombinedOutput()
	return binDir, home, string(b), err
}

func hostRelease(t *testing.T, s releasetest.Signer) releasetest.Release {
	t.Helper()
	return releasetest.NewRelease(t, s, release.Namespace, "2026.9.1", runtime.GOOS, runtime.GOARCH, fakeStrata("2026.9.1"))
}

// The smoke test only checked that the binary ran; it must also report the
// release's exact version, the same check `strata upgrade` makes.
func TestGetShRefusesABinaryReportingTheWrongVersion(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	s := releasetest.NewSigner(t)
	rel := releasetest.NewRelease(t, s, release.Namespace, "2026.9.1", runtime.GOOS, runtime.GOARCH, fakeStrata("2026.9.10"))
	binDir, _, out, err := runGetSh(t, s, rel)
	if err == nil {
		t.Fatalf("get.sh installed a binary that reports the wrong version:\n%s", out)
	}
	if exists(filepath.Join(binDir, "strata")) {
		t.Error("a binary was installed despite the version mismatch")
	}
}

// ssh-keygen's own reason for a failed check was thrown away, so an OpenSSH
// too old to verify signatures (before 8.1) looked like a forged release.
func TestGetShExplainsAnSSHKeygenThatCantVerify(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	fake := t.TempDir()
	writeFile(t, filepath.Join(fake, "ssh-keygen"), "#!/bin/sh\necho 'ssh-keygen: unknown option -- Y' >&2\nexit 1\n")
	if err := os.Chmod(filepath.Join(fake, "ssh-keygen"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := releasetest.NewSigner(t)
	_, _, out, err := runGetSh(t, s, hostRelease(t, s), "PATH="+fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err == nil {
		t.Fatalf("get.sh passed with an ssh-keygen that can't verify:\n%s", out)
	}
	if !strings.Contains(out, "unknown option") || !strings.Contains(out, "OpenSSH 8.1") {
		t.Errorf("get.sh doesn't show ssh-keygen's reason and the OpenSSH version hint:\n%s", out)
	}
}

func TestGetShInstallsAVerifiedRelease(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	s := releasetest.NewSigner(t)
	binDir, home, out, err := runGetSh(t, s, hostRelease(t, s))
	if err != nil {
		t.Fatalf("get.sh failed: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(binDir, "strata")); got != string(fakeStrata("2026.9.1")) {
		t.Errorf("installed binary = %q, want the release binary", got)
	}
	if prof := readFile(t, filepath.Join(home, ".zprofile")); !strings.Contains(prof, binDir) {
		t.Errorf(".zprofile doesn't put %s on PATH:\n%s", binDir, prof)
	}
	if exists(filepath.Join(home, ".zshrc")) {
		t.Error("get.sh created .zshrc - a file strata usually manages")
	}
}

func TestGetShRefusesATamperedRelease(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	s := releasetest.NewSigner(t)
	rel := hostRelease(t, s)
	rel.Assets[rel.ArchiveName()] = append(rel.Assets[rel.ArchiveName()], 0)
	binDir, _, out, err := runGetSh(t, s, rel)
	if err == nil {
		t.Fatalf("get.sh installed a tampered release:\n%s", out)
	}
	if exists(filepath.Join(binDir, "strata")) {
		t.Error("a binary was installed despite the checksum mismatch")
	}
}

func TestGetShRefusesAnUnsignedRelease(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	s := releasetest.NewSigner(t)
	rel := hostRelease(t, s)
	delete(rel.Assets, "checksums.txt.sig")
	binDir, _, out, err := runGetSh(t, s, rel)
	if err == nil {
		t.Fatalf("get.sh installed an unsigned release:\n%s", out)
	}
	if exists(filepath.Join(binDir, "strata")) {
		t.Error("a binary was installed without a signature")
	}
}

func TestGetShRefusesASignatureFromAnotherKey(t *testing.T) {
	needTools(t, "sh", "curl", "tar", "ssh-keygen")
	s, other := releasetest.NewSigner(t), releasetest.NewSigner(t)
	rel := hostRelease(t, s)
	rel.Assets["checksums.txt.sig"] = other.Sign(rel.Assets["checksums.txt"], release.Namespace)
	binDir, _, out, err := runGetSh(t, s, rel)
	if err == nil {
		t.Fatalf("get.sh accepted a signature from an untrusted key:\n%s", out)
	}
	if exists(filepath.Join(binDir, "strata")) {
		t.Error("a binary was installed with an untrusted signature")
	}
}

// get.sh can't read the repo when piped from curl, so it carries its own
// copy of the release public key. That copy must never drift from the one
// strata embeds.
func TestGetShKeyMatchesEmbeddedReleaseKey(t *testing.T) {
	script := readFile(t, "get.sh")
	m := regexp.MustCompile(`STRATA_GET_TRUSTED_KEY:-(ssh-ed25519 [A-Za-z0-9+/=]+)`).FindStringSubmatch(script)
	if m == nil {
		t.Fatal("get.sh has no default STRATA_GET_TRUSTED_KEY")
	}
	want := strings.Join(strings.Fields(release.TrustedKeys[0])[:2], " ")
	if m[1] != want {
		t.Errorf("get.sh key = %s\nembedded key = %s", m[1], want)
	}
}
