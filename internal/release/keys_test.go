package release

import "testing"

// The embedded release key must parse and be exactly the key whose
// fingerprint was recorded when it was created — this catches a truncated or
// wrong key pasted into release_key.pub before a release ever relies on it.
func TestReleaseKeyFingerprint(t *testing.T) {
	if len(TrustedKeys) != 1 {
		t.Fatalf("TrustedKeys = %d keys, want 1", len(TrustedKeys))
	}
	fp, err := Fingerprint(TrustedKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := "SHA256:nMQXiQxd18neATjd15cvS8DQ5ihMQXbrgBa6xLKedNY"; fp != want {
		t.Errorf("release key fingerprint = %s, want %s", fp, want)
	}
}

func TestFingerprintRejectsNonEd25519(t *testing.T) {
	for _, bad := range []string{"", "ssh-rsa AAAAB3NzaC1yc2E=", "ssh-ed25519 !!!notbase64"} {
		if _, err := Fingerprint(bad); err == nil {
			t.Errorf("Fingerprint(%q): want error", bad)
		}
	}
}
