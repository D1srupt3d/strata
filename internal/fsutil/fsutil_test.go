package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHashStable(t *testing.T) {
	if Hash([]byte("hi")) != Hash([]byte("hi")) {
		t.Fatal("hash not deterministic")
	}
	if Hash([]byte("hi")) == Hash([]byte("ho")) {
		t.Fatal("different content, same hash")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", ".zshrc")
	if err := WriteFileAtomic(target, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "export A=1\n" {
		t.Fatalf("content = %q, err = %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(target)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
	// no temp litter
	entries, _ := os.ReadDir(filepath.Dir(target))
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %v", entries)
	}
}
