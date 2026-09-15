// Package fsutil provides content hashing and crash-safe file writes.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// WriteFileAtomic writes data to path via a temp file + fsync + rename, so
// neither a crash nor a power cut leaves a half-written or empty file: the
// data reaches disk before the rename makes it visible. Creates missing
// parent directories, as private as the file: 0700 when mode gives group
// and others nothing (~/.ssh for a 600 config), else 0755; directories that
// already exist keep their mode. Like any rename-into-place, it replaces a
// symlink at path with a regular file — callers that must preserve links
// check first.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	dirMode := os.FileMode(0o755)
	if mode&0o077 == 0 {
		dirMode = 0o700
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".strata-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir flushes the directory entry so a completed rename survives a power
// cut. Best effort: some platforms (Windows) can't open a directory to sync.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}
