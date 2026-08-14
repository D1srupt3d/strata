// Package layers detects which layers apply on this machine and resolves,
// for every managed relative path, which layer's file wins.
package layers

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// OSLayers maps GOOS (+ /etc/os-release content on Linux) to layer names.
func OSLayers(goos, osRelease string) []string {
	switch goos {
	case "darwin":
		return []string{"mac"}
	case "windows":
		return []string{"windows"}
	case "linux":
		out := []string{"linux"}
		if id := parseOSReleaseID(osRelease); id != "" {
			out = append(out, id)
		}
		return out
	}
	return nil
}

func parseOSReleaseID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "ID="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// ReadOSRelease returns /etc/os-release content, or "" off-Linux / on error.
func ReadOSRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return string(b)
}

// Order returns the full layer stack: base, then OS layers, then role layers.
func Order(roleLayers []string, goos, osRelease string) []string {
	out := []string{"base"}
	out = append(out, OSLayers(goos, osRelease)...)
	out = append(out, roleLayers...)
	return out
}

// DefaultIgnore are patterns no repo ever wants managed. These files are
// written *into* layer dirs by the OS file browser, not by the user — Finder
// drops .DS_Store the moment the repo window is opened — so they would
// otherwise resolve as dotfiles and be copied to every machine. Always
// applied, on every OS: a mac-authored .DS_Store is just as meaningless on
// Linux, and a repo is often edited from more than one platform.
var DefaultIgnore = []string{
	"**/.DS_Store",
	"**/._*",
	"**/.Spotlight-V100",
	"**/Thumbs.db",
	"**/desktop.ini",
}

// shouldIgnore reports whether rel matches any pattern. A malformed pattern is
// an error rather than a silent non-match, matching the fail-loud rule that
// governs substitution: a typo must not quietly manage a file you excluded.
func shouldIgnore(rel string, patterns []string) (bool, error) {
	for _, pat := range patterns {
		ok, err := doublestar.Match(pat, rel)
		if err != nil {
			return false, fmt.Errorf("ignore pattern %q: %w", pat, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// Resolve walks each existing layer dir in order and returns
// rel path (forward slashes) → absolute winning source path. Files matching
// DefaultIgnore or one of the caller's ignore patterns are skipped entirely,
// so they never become managed — and so never surface as unmanaged either.
func Resolve(repoDir string, order []string, ignore []string) (map[string]string, error) {
	patterns := append(append([]string{}, DefaultIgnore...), ignore...)
	out := map[string]string{}
	for _, layer := range order {
		layerDir := filepath.Join(repoDir, layer)
		info, err := os.Stat(layerDir)
		if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
			continue // layers are optional folders
		} else if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(layerDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(layerDir, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			skip, err := shouldIgnore(relSlash, patterns)
			if err != nil {
				return err
			}
			if skip {
				return nil
			}
			out[relSlash] = path
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
