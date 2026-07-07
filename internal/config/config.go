// Package config loads the repo-level dots.toml and per-machine machine.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type RepoConfig struct {
	Substitute  []string          `toml:"substitute"`
	Vars        map[string]string `toml:"vars"`
	Permissions map[string]string `toml:"permissions"`
	Hooks       map[string]string `toml:"hooks"`
}

type MachineConfig struct {
	Repo   string            `toml:"repo"`
	Layers []string          `toml:"layers"`
	Vars   map[string]string `toml:"vars"`
}

// Config is the merged view the engine consumes.
type Config struct {
	RepoDir     string
	RoleLayers  []string
	Vars        map[string]string // repo defaults overridden by machine values
	Substitute  []string
	Permissions map[string]string
	Hooks       map[string]string
}

// LoadRepoConfig reads <repoDir>/dots.toml. A missing file yields zero-value
// defaults: dotfiles repos without config are valid.
func LoadRepoConfig(repoDir string) (RepoConfig, error) {
	var rc RepoConfig
	path := filepath.Join(repoDir, "dots.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return rc, nil
	}
	if _, err := toml.DecodeFile(path, &rc); err != nil {
		return rc, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rc, nil
}

func LoadMachineConfig(path string) (MachineConfig, error) {
	var mc MachineConfig
	if _, err := toml.DecodeFile(path, &mc); err != nil {
		return mc, fmt.Errorf("parsing %s (run 'strata init' first?): %w", path, err)
	}
	mc.Repo = ExpandTilde(mc.Repo)
	return mc, nil
}

func ExpandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}

func Merge(rc RepoConfig, mc MachineConfig) Config {
	vars := map[string]string{}
	for k, v := range rc.Vars {
		vars[k] = v
	}
	for k, v := range mc.Vars {
		vars[k] = v
	}
	return Config{
		RepoDir:     mc.Repo,
		RoleLayers:  mc.Layers,
		Vars:        vars,
		Substitute:  rc.Substitute,
		Permissions: rc.Permissions,
		Hooks:       rc.Hooks,
	}
}
