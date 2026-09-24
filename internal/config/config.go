// Package config loads the repo-level dots.toml and per-machine machine.toml,
// and merges them into the one view the engine consumes (see Merge).
package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"strata/internal/layers"
)

type RepoConfig struct {
	Substitute  []string                     `toml:"substitute"`
	Ignore      []string                     `toml:"ignore"`
	Vars        map[string]string            `toml:"vars"`
	LayerVars   map[string]map[string]string `toml:"layer_vars"` // layer → its vars
	Permissions map[string]string            `toml:"permissions"`
	Hooks       map[string]string            `toml:"hooks"`
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
	Vars        map[string]string // [vars], then this machine's [layer_vars], then machine.toml
	VarFrom     map[string]string // var name → where its value came from, e.g. "dots.toml [layer_vars.work]"
	VarSections []string          // every [layer_vars.<layer>] name, sorted; engine.CheckLayers checks them
	Substitute  []string
	Ignore      []string // globs never managed, on top of layers.DefaultIgnore
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
	md, err := toml.DecodeFile(path, &rc)
	if err != nil {
		return rc, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rc, rejectUnknown(path, md)
}

func LoadMachineConfig(path string) (MachineConfig, error) {
	var mc MachineConfig
	md, err := toml.DecodeFile(path, &mc)
	if err != nil {
		return mc, fmt.Errorf("parsing %s (run 'strata init' first?): %w", path, err)
	}
	mc.Repo = ExpandTilde(mc.Repo)
	if err := rejectUnknown(path, md); err != nil {
		return mc, err
	}
	// A relative repo would resolve against whichever folder strata runs
	// from, so the same machine would read "clean" in one folder and
	// "removed" in the next - and apply from the wrong one would delete.
	switch {
	case mc.Repo == "":
		return mc, fmt.Errorf("%s: repo is not set - it must be the full path to your dotfiles repo (set it, or rerun 'strata init --repo <path>')", path)
	case !filepath.IsAbs(mc.Repo):
		return mc, fmt.Errorf("%s: repo = %q is a relative path - use the full path (or ~/...), otherwise it depends on which folder you run strata from", path, mc.Repo)
	}
	return mc, nil
}

// rejectUnknown fails on keys the config structs don't define. The TOML
// decoder silently drops them, so a typo like [hook] for [hooks] would
// otherwise just never run - the kind of quiet no-op strata refuses to allow.
func rejectUnknown(path string, md toml.MetaData) error {
	seen := map[string]bool{}
	var bad []string
	for _, key := range md.Undecoded() {
		if top := key[0]; !seen[top] {
			seen[top] = true
			bad = append(bad, top)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("%s: unknown key(s) %s - typo?", path, strings.Join(bad, ", "))
}

func ExpandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}

// Merge stacks vars the way layers stack files: dots.toml [vars], then each
// of this machine's layers' [layer_vars.<layer>] in layer order, then
// machine.toml [vars]; later wins. Where each value came from is decided
// here, once, so every consumer (the TUI, doctor, init's note) agrees with
// what apply substitutes. goos/osRelease pick the layers (layers.Order) and
// are parameters so tests can simulate any platform; it also means Vars are
// this platform's values. A section for a layer this machine doesn't use is
// skipped here, and engine.CheckLayers rejects names that match no layer.
func Merge(rc RepoConfig, mc MachineConfig, goos, osRelease string) Config {
	vars, from := map[string]string{}, map[string]string{}
	for k, v := range rc.Vars {
		vars[k], from[k] = v, "dots.toml"
	}
	for _, layer := range layers.Order(mc.Layers, goos, osRelease) {
		for k, v := range rc.LayerVars[layer] {
			vars[k], from[k] = v, "dots.toml [layer_vars."+layer+"]"
		}
	}
	for k, v := range mc.Vars {
		vars[k], from[k] = v, "machine.toml"
	}
	return Config{
		RepoDir:     mc.Repo,
		RoleLayers:  mc.Layers,
		Vars:        vars,
		VarFrom:     from,
		VarSections: slices.Sorted(maps.Keys(rc.LayerVars)),
		Substitute:  rc.Substitute,
		Ignore:      rc.Ignore,
		Permissions: rc.Permissions,
		Hooks:       rc.Hooks,
	}
}
