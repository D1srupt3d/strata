package release

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeTemp writes the new binary as an executable temp file in target's
// directory - the same filesystem, so the final rename is atomic - flushed
// to disk before anything else happens.
func writeTemp(target string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".strata-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("can't write next to %s (is its directory writable?): %w", target, err)
	}
	name := f.Name()
	fail := func(err error) (string, error) {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Chmod(0o755); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// install moves the verified temp binary over target. On Unix, renaming
// over a running binary is safe: the running process keeps its open copy.
// Windows won't overwrite a running .exe, so the current one is first moved
// aside to target+".old" (and moved back if the swap fails); CleanupOld
// removes it on a later run.
func install(tmp, target, goos string) error {
	if goos != "windows" {
		if err := os.Rename(tmp, target); err != nil {
			return fmt.Errorf("installing the new binary: %w", err)
		}
		return nil
	}
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("moving the current binary aside: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Rename(old, target)
		return fmt.Errorf("installing the new binary: %w", err)
	}
	return nil
}

// CleanupOld removes the binary a Windows upgrade moved aside. Best effort:
// if it's still running, a later upgrade retries.
func CleanupOld(target string) { _ = os.Remove(target + ".old") }
