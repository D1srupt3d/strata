package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHash(t *testing.T) {
	// SHA-256 of "hi", as `printf hi | shasum -a 256` prints it.
	if got := Hash([]byte("hi")); got != "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4" {
		t.Fatalf("Hash(hi) = %s", got)
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

// Folders WriteFileAtomic creates are as private as the file that needed
// them: ~/.ssh/config at 600 used to get a ~/.ssh anyone could list (755),
// and gpg warns about a ~/.gnupg like that. Folders that already exist keep
// their mode - strata never tightens or loosens those.
func TestWriteFileAtomicMakesNewFoldersAsPrivateAsTheFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes on Windows")
	}
	home := t.TempDir()
	if err := WriteFileAtomic(filepath.Join(home, ".ssh", "keys", "config"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".ssh", ".ssh/keys"} {
		info, err := os.Stat(filepath.Join(home, filepath.FromSlash(d)))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s = %v, want no group/other access", d, perm)
		}
	}

	shared := filepath.Join(home, "shared")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o755); err != nil { // undo any umask
		t.Fatal(err)
	}
	if err := WriteFileAtomic(filepath.Join(shared, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(shared); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("existing folder changed: %v (err %v), want 0755", info.Mode().Perm(), err)
	}
}
