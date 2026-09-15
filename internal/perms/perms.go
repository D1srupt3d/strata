// Package perms decides the file mode for an applied file.
package perms

import (
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/bmatcuk/doublestar/v4"
)

// match is a rule that applies to a path.
type match struct {
	pattern string
	mode    os.FileMode
}

// bestRule finds the most specific (longest) pattern matching rel. Equally
// long matches must agree on the mode: otherwise the winner would depend on
// Go's random map order, and the same config could yield 600 on one run and
// 644 on the next — so a disagreement is a config error.
func bestRule(rel string, rules map[string]string) (match, bool, error) {
	var best []match
	for pattern, modeStr := range rules {
		ok, err := doublestar.Match(pattern, rel)
		if err != nil {
			return match{}, false, fmt.Errorf("permission pattern %q: %w", pattern, err)
		}
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return match{}, false, fmt.Errorf("permission %q = %q: not octal", pattern, modeStr)
		}
		// Go keeps setuid/setgid/sticky outside a plain mode's permission
		// bits, so "4755" would be written as 755 and then never match.
		if n > 0o777 {
			return match{}, false, fmt.Errorf("permission %q = %q: only the rwx bits (000–777) are supported, not setuid, setgid or sticky", pattern, modeStr)
		}
		m := match{pattern, os.FileMode(n)}
		switch {
		case len(best) == 0 || len(pattern) > len(best[0].pattern):
			best = []match{m}
		case len(pattern) == len(best[0].pattern):
			best = append(best, m)
		}
	}
	if len(best) == 0 {
		return match{}, false, nil
	}
	sort.Slice(best, func(i, j int) bool { return best[i].pattern < best[j].pattern })
	for _, m := range best[1:] {
		if m.mode != best[0].mode {
			return match{}, false, fmt.Errorf(
				"permission patterns %q (%o) and %q (%o) both match %s equally specifically; make one more specific",
				best[0].pattern, best[0].mode, m.pattern, m.mode, rel)
		}
	}
	return best[0], true, nil
}

// RuleFor returns the mode string (as written in dots.toml) of the rule that
// applies to rel, or ok=false when no rule matches (i.e. defaults apply).
func RuleFor(rel string, rules map[string]string) (string, bool, error) {
	m, ok, err := bestRule(rel, rules)
	if err != nil || !ok {
		return "", false, err
	}
	return rules[m.pattern], true, nil
}

// ModeFor picks: most specific glob rule > exec-bit heuristic > 0644.
// explicit reports whether a [permissions] rule decided it — only then is
// the mode a policy to enforce on existing files, rather than a default for
// new ones.
func ModeFor(rel string, sourceMode os.FileMode, rules map[string]string) (mode os.FileMode, explicit bool, err error) {
	m, ok, err := bestRule(rel, rules)
	if err != nil {
		return 0, false, err
	}
	if ok {
		return m.mode, true, nil
	}
	if sourceMode&0o111 != 0 {
		return 0o755, false, nil
	}
	return 0o644, false, nil
}
