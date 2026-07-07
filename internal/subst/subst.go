// Package subst replaces {{name}} tokens in opted-in files. Undefined tokens
// are a hard error so a broken file is never written.
package subst

import (
	"fmt"
	"regexp"
	"strings"
)

var tokenRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// Tokens returns the unique variable names referenced in content, in order
// of first appearance.
func Tokens(content []byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range tokenRe.FindAllSubmatch(content, -1) {
		name := string(m[1])
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func Apply(content []byte, vars map[string]string) ([]byte, error) {
	var missing []string
	out := tokenRe.ReplaceAllFunc(content, func(m []byte) []byte {
		name := string(tokenRe.FindSubmatch(m)[1])
		v, ok := vars[name]
		if !ok {
			missing = append(missing, name)
			return m
		}
		return []byte(v)
	})
	if len(missing) > 0 {
		return nil, fmt.Errorf("undefined variables: %s", strings.Join(missing, ", "))
	}
	return out, nil
}
