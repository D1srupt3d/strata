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

// A failed write must leave the old file exactly as it was and no
// .strata-tmp-* file behind: a half-finished apply can't corrupt or litter
// $HOME. These are the failures a real machine can produce; a failing
// write, sync or close mid-file would need a fake filesystem.

// noTempLitter fails if a temp file from WriteFileAtomic survived in dir.
func noTempLitter(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, ".strata-tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The parent folder can't be created because a file is in the way.
func TestWriteFileAtomicFailsWhenParentIsAFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, ".config")
	if err := os.WriteFile(blocker, []byte("a file, not a folder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(filepath.Join(blocker, "app", "conf"), []byte("new"), 0o644); err == nil {
		t.Fatal("want an error when a file sits where the folder should be")
	}
	if got := readString(t, blocker); got != "a file, not a folder\n" {
		t.Errorf("the file in the way changed: %q", got)
	}
}

// The temp file can't be created: the folder isn't writable.
func TestWriteFileAtomicFailsInReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX folder permissions on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes to read-only folders anyway")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) }) // so TempDir can be removed
	if err := WriteFileAtomic(target, []byte("new\n"), 0o644); err == nil {
		t.Fatal("want an error in a read-only folder")
	}
	if got := readString(t, target); got != "old\n" {
		t.Errorf("old file changed to %q", got)
	}
	noTempLitter(t, dir)
}

// The final rename fails: a folder is where the file should go. By then the
// temp file is fully written, so this is the case that tests the cleanup.
func TestWriteFileAtomicFailsWhenTargetIsAFolder(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".vimrc")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(target, "keep")
	if err := os.WriteFile(inside, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("new\n"), 0o644); err == nil {
		t.Fatal("want an error when the target is a folder")
	}
	if got := readString(t, inside); got != "keep\n" {
		t.Errorf("folder contents changed: %q", got)
	}
	noTempLitter(t, dir)
}
