// Package perms decides the file mode for an applied file.
package perms

import (
	"fmt"
	"os"
	"strconv"

	"github.com/bmatcuk/doublestar/v4"
)

// RuleFor returns the mode string of the longest matching explicit rule,
// or ok=false when no rule matches (i.e. defaults apply).
func RuleFor(rel string, rules map[string]string) (string, bool) {
	bestLen := -1
	best := ""
	for pattern, modeStr := range rules {
		if ok, err := doublestar.Match(pattern, rel); err == nil && ok && len(pattern) > bestLen {
			bestLen, best = len(pattern), modeStr
		}
	}
	return best, bestLen >= 0
}

// ModeFor picks: longest matching glob rule > exec-bit heuristic > 0644.
func ModeFor(rel string, sourceMode os.FileMode, rules map[string]string) (os.FileMode, error) {
	bestLen := -1
	var bestMode os.FileMode
	for pattern, modeStr := range rules {
		ok, err := doublestar.Match(pattern, rel)
		if err != nil {
			return 0, fmt.Errorf("permission pattern %q: %w", pattern, err)
		}
		if !ok || len(pattern) <= bestLen {
			continue
		}
		n, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return 0, fmt.Errorf("permission %q = %q: not octal", pattern, modeStr)
		}
		bestLen, bestMode = len(pattern), os.FileMode(n)
	}
	if bestLen >= 0 {
		return bestMode, nil
	}
	if sourceMode&0o111 != 0 {
		return 0o755, nil
	}
	return 0o644, nil
}
