package state

import (
	"path/filepath"
	"testing"
)

func TestRoundTripAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	s, err := Load(path) // missing file → empty state, no error
	if err != nil || len(s.Files) != 0 {
		t.Fatalf("Load missing = %v, %v", s, err)
	}
	s.Files[".zshrc"] = "abc123"
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil || s2.Files[".zshrc"] != "abc123" {
		t.Fatalf("round trip failed: %v, %v", s2, err)
	}
}
