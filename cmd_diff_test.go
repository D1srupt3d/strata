package main

import (
	"strings"
	"testing"
)

// diff says which copy holds the newer edit, so a drifted file can't be
// mistaken for a pending update.
func TestDiffNamesTheNewerSide(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n")

	out, err := run(t, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- home/.zshrc  ") || !strings.Contains(out, "(drifted: $HOME has the newer edit)") {
		t.Fatalf("diff header doesn't name the newer side:\n%s", out)
	}
}
