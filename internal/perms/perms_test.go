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
	}{
		{".ssh/config", 0o644, 0o600},
		{".ssh/keys/id_ed25519", 0o644, 0o600}, // ** crosses dirs
		{".ssh/allowed_signers", 0o644, 0o644}, // longer pattern wins
		{".zshrc", 0o644, 0o644},               // default
		{"bin/tool.sh", 0o755, 0o755},          // exec bit preserved
	}
	for _, tt := range tests {
		got, err := ModeFor(tt.rel, tt.sourceMode, rules)
		if err != nil || got != tt.want {
			t.Errorf("ModeFor(%q) = %v, %v; want %v", tt.rel, got, err, tt.want)
		}
	}
}

func TestRuleFor(t *testing.T) {
	rules := map[string]string{".ssh/**": "600"}
	if mode, ok := RuleFor(".ssh/config", rules); !ok || mode != "600" {
		t.Errorf("RuleFor(.ssh/config) = %q, %v", mode, ok)
	}
	if _, ok := RuleFor(".zshrc", rules); ok {
		t.Error("RuleFor(.zshrc) should not match")
	}
}

func TestBadModeString(t *testing.T) {
	if _, err := ModeFor("x", 0o644, map[string]string{"x": "banana"}); err == nil {
		t.Fatal("expected error for unparseable mode")
	}
}
