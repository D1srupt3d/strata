package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures come from the REAL ssh-keygen (testdata/gen.sh), so these tests
// prove the verifier accepts exactly what release CI will produce.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testKey(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(string(fixture(t, "test_key.pub")))
}

func TestVerifySSHSigAcceptsRealSSHKeygenSignatures(t *testing.T) {
	msg := fixture(t, "message.txt")
	for _, sig := range []string{"message.sha512.sig", "message.sha256.sig"} {
		if err := VerifySSHSig(msg, fixture(t, sig), Namespace, []string{testKey(t)}); err != nil {
			t.Errorf("%s: %v", sig, err)
		}
	}
}

func TestVerifySSHSigRejects(t *testing.T) {
	msg := fixture(t, "message.txt")
	sig := fixture(t, "message.sha512.sig")
	key := []string{testKey(t)}

	// Flip one character deep inside the base64 body.
	corrupted := []byte(string(sig))
	i := len(corrupted) / 2
	if corrupted[i] == 'A' {
		corrupted[i] = 'B'
	} else {
		corrupted[i] = 'A'
	}

	cases := map[string]struct {
		msg, sig  []byte
		namespace string
		trusted   []string
	}{
		"tampered message":            {append(append([]byte{}, msg...), 'x'), sig, Namespace, key},
		"different namespace wanted":  {msg, sig, "other", key},
		"signed in another namespace": {msg, fixture(t, "message.wrong-namespace.sig"), Namespace, key},
		"signer not trusted":          {msg, sig, Namespace, TrustedKeys}, // the real release key didn't sign this
		"no trusted keys":             {msg, sig, Namespace, nil},
		"not armored":                 {msg, []byte("definitely not a signature"), Namespace, key},
		"truncated":                   {msg, sig[:len(sig)/2], Namespace, key},
		"corrupted":                   {msg, corrupted, Namespace, key},
	}
	for name, c := range cases {
		if err := VerifySSHSig(c.msg, c.sig, c.namespace, c.trusted); err == nil {
			t.Errorf("%s: verified, want an error", name)
		}
	}
}
