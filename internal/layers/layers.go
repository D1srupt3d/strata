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

// ValidName reports whether name can be a layer: one folder directly inside
// the repo. "../x" would walk a folder outside the repo as a layer, and
// "a/b" would reach into another layer.
func ValidName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%q is not a layer name - a layer is a folder directly inside the repo", name)
	}
	return nil
}

// CheckRoles verifies that every role layer names a folder in the repo. OS
// layers are optional (most repos have no windows/), but a role layer is one
// you typed: a missing folder is a typo, and skipping it would make every
// file it provides read "removed" - so apply would delete them. A repo
// folder that doesn't exist at all is the documented exception (README
// "Order matters"): it reads as an empty repo, so there is nothing to check.
func CheckRoles(repoDir string, roles []string) error {
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return nil
	}
	for _, r := range roles {
		if err := ValidName(r); err != nil {
			return err
		}
	}
	missing, hints, err := missingFolders(repoDir, roles)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		quoted := make([]string, len(missing))
		for i, m := range missing {
			quoted[i] = fmt.Sprintf("%q", m)
		}
		return fmt.Errorf("role layer %s has no folder in %s - typo?%s", strings.Join(quoted, ", "), repoDir, caseHint(hints))
	}
	return nil
}

// missingFolders returns the names that aren't a folder in repoDir spelled
// exactly that way, plus a hint for each that differs from an entry only in
// case. It matches the repo's listing instead of asking os.Stat about
// repoDir/name: macOS and Windows file systems ignore case, so Stat("Work")
// finds work/ there. A layer name is also a lookup key ([layer_vars.work])
// where "Work" is not "work", so the listing is the only answer every OS
// agrees on. A match must still be a folder to os.Stat, as CheckRoles has
// always required.
func missingFolders(repoDir string, names []string) (missing, hints []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil
	}
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the repo folder: %w", err)
	}
	for _, name := range names {
		found, near := false, ""
		for _, e := range entries {
			if e.Name() == name {
				info, statErr := os.Stat(filepath.Join(repoDir, name))
				found = statErr == nil && info.IsDir()
			} else if strings.EqualFold(e.Name(), name) {
				near = e.Name()
			}
		}
		if found {
			continue
		}
		missing = append(missing, name)
		if near != "" {
			hints = append(hints, fmt.Sprintf("%q is spelled %q", name, near))
		}
	}
	return missing, hints, nil
}

// caseHint turns missingFolders' hints into a suffix for an error message.
func caseHint(hints []string) string {
	if len(hints) == 0 {
		return ""
	}
	return " (" + strings.Join(hints, ", ") + ": layer names are case-sensitive)"
}

// builtinOS are the OS layer names OSLayers returns that are fixed rather
// than read from /etc/os-release. A [layer_vars] section may name one even
// when its folder doesn't exist: git can't store an empty folder, and a
// placeholder like .gitkeep inside a layer would be deployed to ~/.gitkeep.
// Distro names are open-ended, so without their folder they can't be told
// apart from typos.
var builtinOS = map[string]bool{"mac": true, "linux": true, "windows": true}

// CheckVarSections verifies the names of dots.toml's [layer_vars.<name>]
// sections. Each must be a built-in OS layer or a folder in the repo,
// spelled exactly: a typo'd section would never apply, and machines would
// quietly get the defaults. base is refused, because its values are just
// [vars]. Like CheckRoles, a repo folder that doesn't exist at all has
// nothing to check.
func CheckVarSections(repoDir string, names []string) error {
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return nil
	}
	var check []string
	for _, n := range names {
		switch {
		case n == "base":
			return fmt.Errorf("[layer_vars.base]: base's values are the defaults - put them in [vars]")
		case builtinOS[n]:
			continue
		}
		if err := ValidName(n); err != nil {
			return fmt.Errorf("[layer_vars.%s]: %w", n, err)
		}
		check = append(check, n)
	}
	missing, hints, err := missingFolders(repoDir, check)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		secs := make([]string, len(missing))
		for i, m := range missing {
			secs[i] = "[layer_vars." + m + "]"
		}
		return fmt.Errorf("no layer folder in %s for %s - typo?%s", repoDir, strings.Join(secs, ", "), caseHint(hints))
	}
	return nil
}

// DefaultIgnore are patterns no repo ever wants managed. These files are
// written *into* layer dirs by the OS file browser, not by the user - Finder
// drops .DS_Store the moment the repo window is opened - so they would
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

// allPatterns is the effective ignore set: built-ins first, then repo config.
func allPatterns(ignore []string) []string {
	return append(append([]string{}, DefaultIgnore...), ignore...)
}

// Ignored reports whether rel is excluded. Exported because the engine must
// ask the same question about paths that exist only in state.json - a file
// recorded by an earlier apply and ignored since is not "gone from the
// layers", it is simply no longer ours, and must not be deleted from $HOME.
func Ignored(rel string, ignore []string) (bool, error) {
	return shouldIgnore(rel, allPatterns(ignore))
}

// Resolve walks each existing layer dir in order and returns
// rel path (forward slashes) → absolute winning source path. Files matching
// DefaultIgnore or one of the caller's ignore patterns are skipped entirely,
// so they never become managed - and so never surface as unmanaged either.
func Resolve(repoDir string, order []string, ignore []string) (map[string]string, error) {
	patterns := allPatterns(ignore)
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
