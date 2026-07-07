// Package layers detects which layers apply on this machine and resolves,
// for every managed relative path, which layer's file wins.
package layers

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// Resolve walks each existing layer dir in order and returns
// rel path (forward slashes) → absolute winning source path.
func Resolve(repoDir string, order []string) (map[string]string, error) {
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
			out[filepath.ToSlash(rel)] = path
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
