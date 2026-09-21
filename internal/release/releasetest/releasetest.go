// Package releasetest fakes a signed GitHub release for tests: a throwaway
// signing key, a signer that produces the same armored signatures as
// `ssh-keygen -Y sign`, release-shaped archives, and an httptest server
// that answers like GitHub's releases/latest API. Test-only, like
// net/http/httptest - production code never imports it.
package releasetest

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// Signer holds a throwaway ed25519 key.
type Signer struct {
	priv    ed25519.PrivateKey
	pubBlob []byte
	// AuthorizedKey is the public key as one authorized_keys line - what a
	// trusted-key list holds.
	AuthorizedKey string
}

func NewSigner(t testing.TB) Signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blob := appendString(appendString(nil, []byte("ssh-ed25519")), pub)
	return Signer{
		priv:          priv,
		pubBlob:       blob,
		AuthorizedKey: "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " strata-test",
	}
}

// Sign returns an armored signature of msg in namespace, byte-compatible
// with `ssh-keygen -Y sign -n <namespace>` (PROTOCOL.sshsig, sha512).
func (s Signer) Sign(msg []byte, namespace string) []byte {
	digest := sha512.Sum512(msg)
	signed := []byte("SSHSIG")
	for _, f := range [][]byte{[]byte(namespace), nil, []byte("sha512"), digest[:]} {
		signed = appendString(signed, f)
	}
	sigBlob := appendString(appendString(nil, []byte("ssh-ed25519")), ed25519.Sign(s.priv, signed))

	blob := binary.BigEndian.AppendUint32([]byte("SSHSIG"), 1)
	for _, f := range [][]byte{s.pubBlob, []byte(namespace), nil, []byte("sha512"), sigBlob} {
		blob = appendString(blob, f)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "SSH SIGNATURE", Bytes: blob})
}

func appendString(dst, s []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(s)))
	return append(dst, s...)
}

// Archive packs body as the release binary for goos, the way GoReleaser
// does: strata (or strata.exe) at the root next to a README, tar.gz - or
// zip on Windows. It returns the archive and its extension.
func Archive(t testing.TB, goos string, body []byte) (data []byte, ext string) {
	t.Helper()
	if goos == "windows" {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for name, content := range map[string][]byte{"README.md": []byte("docs"), "strata.exe": body} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(content); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes(), "zip"
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{"README.md", []byte("docs")}, {"strata", body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), "tar.gz"
}

// Release is a fake GitHub release: a tag and its downloadable assets.
// Tests edit Assets (tamper, delete) before calling Serve.
type Release struct {
	Tag    string
	Assets map[string][]byte
}

// NewRelease builds a complete signed release of version for goos/goarch
// whose binary is body: the archive, checksums.txt, and checksums.txt.sig
// signed by s in namespace.
func NewRelease(t testing.TB, s Signer, namespace, version, goos, goarch string, body []byte) Release {
	t.Helper()
	archive, ext := Archive(t, goos, body)
	name := fmt.Sprintf("strata_%s_%s_%s.%s", version, goos, goarch, ext)
	sum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	return Release{
		Tag: "v" + version,
		Assets: map[string][]byte{
			name:                archive,
			"checksums.txt":     checksums,
			"checksums.txt.sig": s.Sign(checksums, namespace),
		},
	}
}

// ArchiveName returns the name of the release's binary archive.
func (r Release) ArchiveName() string {
	for name := range r.Assets {
		if strings.HasPrefix(name, "strata_") {
			return name
		}
	}
	return ""
}

// Serve starts an httptest server for rel and returns the URL of its
// releases/latest endpoint. The JSON mirrors GitHub's (pretty-printed, same
// field names), and assets download from the same server.
func Serve(t testing.TB, rel Release) string {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	var assets []asset
	names := make([]string, 0, len(rel.Assets))
	for name := range rel.Assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		assets = append(assets, asset{name, srv.URL + "/download/" + name})
	}
	body, err := json.MarshalIndent(map[string]any{
		"tag_name":   rel.Tag,
		"draft":      false,
		"prerelease": false,
		"assets":     assets,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/repos/D1srupt3d/strata/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		data, ok := rel.Assets[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	})
	return srv.URL + "/repos/D1srupt3d/strata/releases/latest"
}
