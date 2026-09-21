package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

// tarGz and zipOf build release-shaped archives in memory.
func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	a := tarGz(t, map[string]string{"README.md": "docs", "strata": "BINARY"})
	got, err := ExtractBinary(a, "tar.gz", "strata")
	if err != nil || string(got) != "BINARY" {
		t.Fatalf("ExtractBinary = %q, %v; want BINARY", got, err)
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	a := zipOf(t, map[string]string{"README.md": "docs", "strata.exe": "EXE"})
	got, err := ExtractBinary(a, "zip", "strata.exe")
	if err != nil || string(got) != "EXE" {
		t.Fatalf("ExtractBinary = %q, %v; want EXE", got, err)
	}
}

// Only an entry named exactly the binary, at the archive root, counts. A
// "../strata" or "sub/strata" is never taken - and nothing is ever written
// to disk from archive paths, so traversal can't escape anywhere.
func TestExtractBinaryOnlyTakesTheRootEntry(t *testing.T) {
	a := tarGz(t, map[string]string{"../strata": "evil", "sub/strata": "nested", "README.md": "docs"})
	if got, err := ExtractBinary(a, "tar.gz", "strata"); err == nil {
		t.Fatalf("took %q from a non-root entry; want an error", got)
	}
}

func TestExtractBinaryRejectsUnknownFormat(t *testing.T) {
	if _, err := ExtractBinary([]byte("x"), "rar", "strata"); err == nil {
		t.Fatal("want error for unknown archive format")
	}
}
