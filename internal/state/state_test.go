package state

import (
	"os"
	"path/filepath"
	"reflect"
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

func writeState(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A state file from a newer strata may mean something this build doesn't
// understand; reading it anyway could misjudge drift and clobber edits.
func TestNewerStateVersionIsRejected(t *testing.T) {
	path := writeState(t, `{"version": 99, "files": {}}`)
	if _, err := Load(path); err == nil {
		t.Fatal("loaded a state file from a newer format without error")
	}
}

// Files written before the version field existed must keep working.
func TestLegacyStateWithoutVersionLoads(t *testing.T) {
	path := writeState(t, `{"files": {".zshrc": "abc123"}}`)
	s, err := Load(path)
	if err != nil || s.Files[".zshrc"] != "abc123" {
		t.Fatalf("Load legacy = %v, %v", s, err)
	}
}

// Pending hooks are the retry queue: they must survive a save/load, stay
// de-duplicated and sorted (hooks run in rel order), and drop when done.
func TestPendingHooksSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.QueueHooks([]string{".zshrc", ".Brewfile", ".zshrc"})
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".Brewfile", ".zshrc"}; !reflect.DeepEqual(s2.PendingHooks, want) {
		t.Fatalf("PendingHooks = %v, want %v", s2.PendingHooks, want)
	}
	s2.FinishHooks([]string{".Brewfile"})
	if want := []string{".zshrc"}; !reflect.DeepEqual(s2.PendingHooks, want) {
		t.Fatalf("after FinishHooks = %v, want %v", s2.PendingHooks, want)
	}
}

// Two strata processes writing state at once would lose one's updates; the
// lock makes the second back off (fail fast) instead.
func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(path); err == nil {
		t.Fatal("second Lock succeeded while the first was held")
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock2, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	if err := unlock2(); err != nil {
		t.Fatal(err)
	}
}
