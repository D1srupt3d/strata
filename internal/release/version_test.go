package release

import "testing"

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]Version{
		"2026.9.0":   {2026, 9, 0},
		"v2026.9.1":  {2026, 9, 1}, // tags carry a v; binaries don't
		"2026.10.12": {2026, 10, 12},
	} {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	// Source builds (git describe) and junk are not release versions.
	for _, bad := range []string{"2026.9.0-3-gabc123", "2026.9.0-dev", "2026.9", "v", "", "2026.09.x"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q): want error", bad)
		}
	}
}

// Comparison is numeric per field. As plain strings "2026.10.0" sorts before
// "2026.9.9", which would make October's release look older than September's.
func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2026.9.0", "2026.9.1", true},
		{"2026.9.9", "2026.10.0", true},
		{"2026.12.3", "2027.1.0", true},
		{"2026.9.1", "2026.9.1", false},
		{"2026.10.0", "2026.9.9", false},
	}
	for _, c := range cases {
		a, _ := ParseVersion(c.a)
		b, _ := ParseVersion(c.b)
		if got := a.Less(b); got != c.want {
			t.Errorf("%s < %s = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionString(t *testing.T) {
	if s := (Version{2026, 9, 1}).String(); s != "2026.9.1" {
		t.Errorf("String() = %q, want 2026.9.1", s)
	}
}
