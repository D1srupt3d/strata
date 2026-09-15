package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
)

// SSH signatures (`ssh-keygen -Y sign`) follow OpenSSH's PROTOCOL.sshsig: a
// PEM-armored blob holding the signer's public key, the namespace, the hash
// algorithm, and a signature over
//
//	"SSHSIG" ‖ string(namespace) ‖ string(reserved) ‖ string(hash_alg) ‖ string(H(msg))
//
// where string(x) is a big-endian uint32 length followed by x. Only ed25519
// keys are accepted — that is what the release key is.

const sshsigMagic = "SSHSIG"

// VerifySSHSig checks an armored `ssh-keygen -Y sign` signature over msg. It
// passes only if the signature is well-formed, was made in namespace, and
// was made by one of the trusted keys (authorized_keys lines). Every other
// outcome is an error saying what was wrong.
func VerifySSHSig(msg, armored []byte, namespace string, trusted []string) error {
	block, _ := pem.Decode(armored)
	if block == nil || block.Type != "SSH SIGNATURE" {
		return errors.New("not an SSH signature")
	}
	b, ok := bytes.CutPrefix(block.Bytes, []byte(sshsigMagic))
	if !ok {
		return errors.New("not an SSH signature (bad header)")
	}
	if len(b) < 4 || binary.BigEndian.Uint32(b) != 1 {
		return errors.New("unsupported SSH signature version")
	}
	b = b[4:]
	var pubBlob, ns, reserved, hashAlg, sigBlob []byte
	for _, field := range []*[]byte{&pubBlob, &ns, &reserved, &hashAlg, &sigBlob} {
		if *field, b, ok = readString(b); !ok {
			return errors.New("truncated SSH signature")
		}
	}
	if len(b) != 0 {
		return errors.New("trailing data after SSH signature")
	}
	if string(ns) != namespace {
		return fmt.Errorf("signature was made for %q, not %q", ns, namespace)
	}

	var digest []byte
	switch string(hashAlg) {
	case "sha512":
		sum := sha512.Sum512(msg)
		digest = sum[:]
	case "sha256":
		sum := sha256.Sum256(msg)
		digest = sum[:]
	default:
		return fmt.Errorf("unsupported signature hash %q", hashAlg)
	}

	key, err := trustedKey(pubBlob, trusted)
	if err != nil {
		return err
	}
	algo, rest, ok := readString(sigBlob)
	if !ok || string(algo) != "ssh-ed25519" {
		return errors.New("signature is not ssh-ed25519")
	}
	raw, rest, ok := readString(rest)
	if !ok || len(rest) != 0 || len(raw) != ed25519.SignatureSize {
		return errors.New("malformed ssh-ed25519 signature")
	}

	signed := []byte(sshsigMagic)
	for _, field := range [][]byte{ns, reserved, hashAlg, digest} {
		signed = appendString(signed, field)
	}
	if !ed25519.Verify(key, signed, raw) {
		return errors.New("signature does not match the data")
	}
	return nil
}

// trustedKey returns the key whose wire blob is pubBlob, if it is trusted.
func trustedKey(pubBlob []byte, trusted []string) (ed25519.PublicKey, error) {
	for _, line := range trusted {
		blob, key, err := parseEd25519(line)
		if err != nil {
			return nil, fmt.Errorf("trusted key list: %w", err)
		}
		if bytes.Equal(blob, pubBlob) {
			return key, nil
		}
	}
	return nil, fmt.Errorf("signed by a key strata doesn't trust (%s)", blobFingerprint(pubBlob))
}

// readString reads one SSH wire-format string from b.
func readString(b []byte) (s, rest []byte, ok bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := binary.BigEndian.Uint32(b)
	if uint64(len(b)-4) < uint64(n) {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}

func appendString(dst, s []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(s)))
	return append(dst, s...)
}
