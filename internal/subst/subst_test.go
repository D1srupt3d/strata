package subst

import (
	"strings"
	"testing"
)

func TestApply(t *testing.T) {
	vars := map[string]string{"email": "a@b.c", "name": "Luke"}
	tests := []struct {
		in, want string
	}{
		{"email = {{email}}", "email = a@b.c"},
		{"{{ email }} and {{name}}", "a@b.c and Luke"},
		{"no tokens ${SHELL_VAR} {{ }}", "no tokens ${SHELL_VAR} {{ }}"}, // shell syntax untouched
	}
	for _, tt := range tests {
		got, err := Apply([]byte(tt.in), vars)
		if err != nil || string(got) != tt.want {
			t.Errorf("Apply(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestTokens(t *testing.T) {
	got := Tokens([]byte("a {{email}} b {{ name }} c {{email}} ${SHELL}"))
	if len(got) != 2 || got[0] != "email" || got[1] != "name" {
		t.Errorf("Tokens = %v", got)
	}
}

func TestUndefinedVarFails(t *testing.T) {
	_, err := Apply([]byte("hi {{missing}} {{alsomissing}}"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "alsomissing") {
		t.Errorf("error should name all undefined vars: %v", err)
	}
}
