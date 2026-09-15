package releasetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fake releases are only meaningful if their signatures are real. This
// has the REAL `ssh-keygen -Y verify` check Signer's output, so every test
// built on releasetest is exercising genuine OpenSSH-compatible signatures.
func TestSignerMatchesSSHKeygen(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	s := NewSigner(t)
	dir := t.TempDir()
	msg := []byte("abc123  strata_2026.9.1_darwin_arm64.tar.gz\n")
	sigPath := filepath.Join(dir, "checksums.txt.sig")
	allowed := filepath.Join(dir, "allowed_signers")
	if err := os.WriteFile(sigPath, s.Sign(msg, "strata-release"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allowed, []byte(`strata-test namespaces="strata-release" `+s.AuthorizedKey+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	verify := func(namespace string, data []byte) error {
		cmd := exec.Command("ssh-keygen", "-Y", "verify", "-f", allowed, "-I", "strata-test", "-n", namespace, "-s", sigPath)
		cmd.Stdin = strings.NewReader(string(data))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return &exec.ExitError{Stderr: out}
		}
		return nil
	}
	if err := verify("strata-release", msg); err != nil {
		t.Fatalf("ssh-keygen rejected Signer's signature: %s", err.(*exec.ExitError).Stderr)
	}
	// And it isn't just accepting anything.
	if err := verify("strata-release", append(msg, 'x')); err == nil {
		t.Error("ssh-keygen accepted a signature over different data")
	}
}
