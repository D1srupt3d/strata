package release

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"strata/internal/release/releasetest"
)

// Rotating the release key means one release that trusts two keys at once:
// release_key.pub holds one key per line (comments and blank lines ignored),
// and a signature from any listed key verifies.
func TestTrustedKeysSupportRotation(t *testing.T) {
	retiring := releasetest.NewSigner(t).AuthorizedKey
	keys := keyLines("# retiring key\n" + retiring + "\n\n" + testKey(t) + "\n")
	if len(keys) != 2 {
		t.Fatalf("keyLines found %d keys, want 2: %q", len(keys), keys)
	}
	if err := VerifySSHSig(fixture(t, "message.txt"), fixture(t, "message.sha512.sig"), Namespace, keys); err != nil {
		t.Errorf("a signature by the second trusted key was rejected: %v", err)
	}
}

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

// VerifySSHSig parses bytes downloaded from the internet, so no input may
// make it panic, and nothing but the one genuinely signed message may ever
// verify. `go test` runs the seeds; explore further with
//
//	go test -fuzz=FuzzVerifySSHSig ./internal/release
func FuzzVerifySSHSig(f *testing.F) {
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		return b
	}
	msg := read("message.txt")
	key := strings.TrimSpace(string(read("test_key.pub")))
	for _, sig := range []string{"message.sha512.sig", "message.sha256.sig", "message.wrong-namespace.sig"} {
		f.Add(msg, read(sig))
	}
	f.Add([]byte("x"), []byte("-----BEGIN SSH SIGNATURE-----\nU1NIU0lHAAAAAQ==\n-----END SSH SIGNATURE-----\n"))
	f.Fuzz(func(t *testing.T, m, sig []byte) {
		if VerifySSHSig(m, sig, Namespace, []string{key}) == nil && !bytes.Equal(m, msg) {
			t.Fatalf("verified a message that was never signed: %q", m)
		}
	})
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
