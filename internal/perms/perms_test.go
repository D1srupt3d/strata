package perms

import (
	"os"
	"testing"
)

func TestModeFor(t *testing.T) {
	rules := map[string]string{".ssh/**": "600", ".ssh/allowed_signers": "644"}
	tests := []struct {
		rel        string
		sourceMode os.FileMode
		want       os.FileMode
		explicit   bool // came from a [permissions] rule, not a default
	}{
		{".ssh/config", 0o644, 0o600, true},
		{".ssh/keys/id_ed25519", 0o644, 0o600, true}, // ** crosses dirs
		{".ssh/allowed_signers", 0o644, 0o644, true}, // longer pattern wins
		{".zshrc", 0o644, 0o644, false},              // default
		{"bin/tool.sh", 0o755, 0o755, false},         // exec bit preserved
	}
	for _, tt := range tests {
		got, explicit, err := ModeFor(tt.rel, tt.sourceMode, rules)
		if err != nil || got != tt.want || explicit != tt.explicit {
			t.Errorf("ModeFor(%q) = %v, explicit=%v, %v; want %v, explicit=%v",
				tt.rel, got, explicit, err, tt.want, tt.explicit)
		}
	}
}

func TestRuleFor(t *testing.T) {
	rules := map[string]string{".ssh/**": "600"}
	if mode, ok, err := RuleFor(".ssh/config", rules); err != nil || !ok || mode != "600" {
		t.Errorf("RuleFor(.ssh/config) = %q, %v, %v", mode, ok, err)
	}
	if _, ok, err := RuleFor(".zshrc", rules); err != nil || ok {
		t.Errorf("RuleFor(.zshrc) should not match (ok=%v, err=%v)", ok, err)
	}
	// RuleFor used to swallow pattern errors (ModeFor reported them); both
	// now share one matcher, so a malformed glob fails the same way in each.
	if _, _, err := RuleFor(".zshrc", map[string]string{"[": "600"}); err == nil {
		t.Error("RuleFor with malformed pattern: want error")
	}
}

// "Longest pattern wins" had no tie-break: two equally long patterns that
// matched with different modes were resolved by Go's random map order, so
// the same config produced 600 on one run and 644 on the next.
func TestEqualLengthRulesThatDisagreeAreAnError(t *testing.T) {
	rules := map[string]string{".ssh/**": "600", "*/con*g": "644"} // both 7 chars
	if mode, _, err := ModeFor(".ssh/config", 0o644, rules); err == nil {
		t.Fatalf("ambiguous rules resolved to %v without error", mode)
	}
}

func TestEqualLengthRulesThatAgreeAreFine(t *testing.T) {
	rules := map[string]string{".ssh/**": "600", "*/con*g": "0600"} // same mode, spelled differently
	if mode, _, err := ModeFor(".ssh/config", 0o644, rules); err != nil || mode != 0o600 {
		t.Fatalf("ModeFor = %v, %v; want 0600, nil", mode, err)
	}
}

func TestBadModeString(t *testing.T) {
	if _, _, err := ModeFor("x", 0o644, map[string]string{"x": "banana"}); err == nil {
		t.Fatal("expected error for unparseable mode")
	}
}
