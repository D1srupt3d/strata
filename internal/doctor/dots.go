package doctor

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"strata/internal/layers"
	"strata/internal/perms"
	"strata/internal/subst"
)

const globFix = "fix the pattern: look for an unclosed [ or {"

// checkDotsToml finds dots.toml settings that apply rejects, and ones that
// silently do nothing because they name a file no layer provides.
func checkDotsToml(r *report, in Inputs, l loaded) {
	r.group = "dots.toml"
	switch {
	case !l.machine:
		r.add(Skip, "checks", "need a readable machine.toml", "")
		return
	case !l.repo:
		r.add(Skip, "checks", "need the repo folder", "")
		return
	case !l.dots:
		r.add(Skip, "checks", "need dots.toml to parse", "")
		return
	}
	cfg := l.cfg

	// doublestar.Match only notices a bad pattern when matching reaches the
	// broken part, so apply can pass today and fail after a new file lands.
	mark := len(r.findings)
	for _, pat := range cfg.Ignore {
		if !doublestar.ValidatePattern(pat) {
			r.add(Error, fmt.Sprintf("ignore %q", pat), "not a valid glob pattern", globFix)
		}
	}
	if len(r.findings) > mark {
		r.add(Skip, "substitute, hooks, permissions, vars", "need every ignore pattern to be valid", "")
		return
	}
	r.add(OK, "ignore", "every pattern is valid", "")

	all, err := everyLayerFile(cfg.RepoDir, cfg.Ignore)
	if err != nil {
		r.add(Error, "files", "reading the repo: "+err.Error(), "check the repo folder's permissions")
		r.add(Skip, "substitute, hooks, permissions, vars", "need the repo's folders to be readable", "")
		return
	}

	mark = len(r.findings)
	for _, rel := range cfg.Substitute {
		if !all[rel] {
			r.add(Warn, fmt.Sprintf("substitute %q", rel), "no layer provides this file, so the entry does nothing",
				`fix the path: it is relative to your home folder, like ".gitconfig"`)
		}
	}
	r.okIfClean(mark, "substitute", "every entry names a file in a layer")

	mark = len(r.findings)
	for _, rel := range slices.Sorted(maps.Keys(cfg.Hooks)) {
		if !all[rel] {
			r.add(Warn, fmt.Sprintf("hook %q", rel), "no layer provides this file, so the hook never runs",
				`fix the path: it is relative to your home folder, like ".zshrc"`)
		}
	}
	r.okIfClean(mark, "hooks", "every hook's file is in a layer")

	checkPermissions(r, cfg.Permissions, all)
	checkVars(r, in, l)
}

// everyLayerFile returns the rels provided by any folder in the repo, each
// folder resolved on its own (like the TUI's columns). "Any layer" means
// every folder, not this machine's stack: a hook for a mac-only file is not
// a no-op just because doctor runs on Linux.
func everyLayerFile(repoDir string, ignore []string) (map[string]bool, error) {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil, err
	}
	all := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue // .git and friends are not layers
		}
		files, err := layers.Resolve(repoDir, []string{e.Name()}, ignore)
		if err != nil {
			return nil, err
		}
		for rel := range files {
			all[rel] = true
		}
	}
	return all, nil
}

// checkPermissions warns on rules that match no file, and reports what apply
// rejects: a pattern that isn't a glob, or a file the rules disagree on.
func checkPermissions(r *report, rules map[string]string, all map[string]bool) {
	mark := len(r.findings)
	valid := true
	for _, pat := range slices.Sorted(maps.Keys(rules)) {
		subject := fmt.Sprintf("permission %q", pat)
		if !doublestar.ValidatePattern(pat) {
			valid = false
			r.add(Error, subject, "not a valid glob pattern", globFix)
			continue
		}
		if !matchesAny(pat, all) {
			r.add(Warn, subject, "matches no file in any layer, so the rule does nothing",
				`fix the pattern: it matches paths relative to your home folder, like ".ssh/*"`)
		}
	}
	if !valid {
		r.add(Skip, "file modes", "need every permission pattern to be valid", "")
		return
	}
	// perms.ModeFor is what apply calls, so its errors are exactly apply's.
	// A bad mode string repeats for every file it matches: report it once.
	seen := map[string]bool{}
	for _, rel := range slices.Sorted(maps.Keys(all)) {
		_, _, err := perms.ModeFor(rel, 0, rules)
		if err == nil || seen[err.Error()] {
			continue
		}
		seen[err.Error()] = true
		r.add(Error, fmt.Sprintf("mode of %q", rel), err.Error(), "fix the [permissions] rules the error names")
	}
	r.okIfClean(mark, "permissions", "every rule matches a file, and no two disagree")
}

func matchesAny(pattern string, files map[string]bool) bool {
	for rel := range files {
		if ok, _ := doublestar.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

// checkVars lists the undefined {{vars}} in each substituted file this
// machine gets. Apply stops at the first such file; doctor names them all.
// Only this machine's winners count: another OS's copy is substituted
// there, with that machine's vars.
func checkVars(r *report, in Inputs, l loaded) {
	if !l.roles {
		r.add(Skip, "vars", "need every role layer to be valid", "")
		return
	}
	cfg := l.cfg
	order := layers.Order(cfg.RoleLayers, in.GOOS, in.OSRelease)
	sources, err := layers.Resolve(cfg.RepoDir, order, cfg.Ignore)
	if err != nil {
		r.add(Error, "vars", "resolving this machine's layers: "+err.Error(), "check the repo folder's permissions")
		return
	}
	mark := len(r.findings)
	for _, rel := range cfg.Substitute {
		src, ok := sources[rel]
		if !ok {
			continue // not on this machine; a file in no layer at all is warned above
		}
		subject := fmt.Sprintf("vars in %q", rel)
		content, err := os.ReadFile(src)
		if err != nil {
			r.add(Error, subject, err.Error(), "check the file's permissions")
			continue
		}
		if _, err := subst.Apply(content, cfg.Vars); err != nil {
			r.add(Error, subject, err.Error(), "define them under [vars] in dots.toml or machine.toml")
		}
	}
	r.okIfClean(mark, "vars", "every substituted file on this machine has its vars defined")
}
