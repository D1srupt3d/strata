// Package release finds, verifies, and installs strata's signed GitHub
// releases for `strata upgrade`. Nothing here runs unless the user asks:
// strata never touches the network on its own.
package release

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a release version: CalVer YYYY.M.PATCH with an un-padded month.
type Version struct{ Year, Month, Patch int }

// ParseVersion reads a release version, with or without a tag's leading "v".
// Anything else - a source build's git-describe string such as
// 2026.9.0-3-gabc123, a -dev suffix - is not a release, and is an error.
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%q is not a release version (want YYYY.M.PATCH)", s)
	}
	var n [3]int
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return Version{}, fmt.Errorf("%q is not a release version (want YYYY.M.PATCH)", s)
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("%q: %w", s, err)
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2]}, nil
}

// Less compares field by field, numerically - as strings, "2026.10.0" would
// sort before "2026.9.9".
func (v Version) Less(o Version) bool {
	if v.Year != o.Year {
		return v.Year < o.Year
	}
	if v.Month != o.Month {
		return v.Month < o.Month
	}
	return v.Patch < o.Patch
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Year, v.Month, v.Patch) }
