package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"strings"
)

// Namespace every release signature must carry. ssh-keygen stamps it into
// the signed data, so a signature this key made for anything else (a git
// commit, some other file) can never pass as a release signature.
const Namespace = "strata-release"

//go:embed release_key.pub
var releaseKeyFile string

// TrustedKeys are the release-signing public keys, one authorized_keys line
// each, embedded from release_key.pub. Rotation: ship one release signed by
// the old key that trusts both, then sign with the new key and drop the old
// one later. Software already installed can't un-trust a stolen key; the
// recovery path is reinstalling with get.sh.
var TrustedKeys = keyLines(releaseKeyFile)

func keyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// parseEd25519 decodes an authorized_keys line ("ssh-ed25519 <base64>
// [comment]") into its SSH wire blob and the raw 32-byte key.
func parseEd25519(line string) (blob []byte, key ed25519.PublicKey, err error) {
	f := strings.Fields(line)
	if len(f) < 2 || f[0] != "ssh-ed25519" {
		return nil, nil, errors.New("not an ssh-ed25519 public key")
	}
	blob, err = base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		return nil, nil, errors.New("public key is not valid base64")
	}
	algo, rest, ok := readString(blob)
	if !ok || string(algo) != "ssh-ed25519" {
		return nil, nil, errors.New("malformed ssh-ed25519 public key")
	}
	k, rest, ok := readString(rest)
	if !ok || len(rest) != 0 || len(k) != ed25519.PublicKeySize {
		return nil, nil, errors.New("malformed ssh-ed25519 public key")
	}
	return blob, ed25519.PublicKey(k), nil
}

// Fingerprint is the SHA256 fingerprint `ssh-keygen -l` prints for a key.
func Fingerprint(line string) (string, error) {
	blob, _, err := parseEd25519(line)
	if err != nil {
		return "", err
	}
	return blobFingerprint(blob), nil
}

func blobFingerprint(blob []byte) string {
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}
