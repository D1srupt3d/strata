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

// Apply runs on every substituted dotfile. For any content it must not
// panic; with no vars it must fail exactly when there are tokens, naming
// each once in order of first use; and with every token defined it must
// succeed. `go test` runs the seeds; explore further with
//
//	go test -fuzz=FuzzApply ./internal/subst
func FuzzApply(f *testing.F) {
	for _, s := range []string{"email = {{email}}", "{{ a }} {{a}} {{b}}", "${SHELL} {{ }}", "{{{{a}}}}", "{{a}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		toks := Tokens([]byte(content))
		_, err := Apply([]byte(content), nil)
		switch {
		case len(toks) == 0 && err != nil:
			t.Fatalf("Apply(%q) with no tokens failed: %v", content, err)
		case len(toks) > 0 && (err == nil || err.Error() != "undefined variables: "+strings.Join(toks, ", ")):
			t.Fatalf("Apply(%q) = %v, want every token (%v) named once", content, err, toks)
		}
		vars := map[string]string{}
		for _, name := range toks {
			vars[name] = "v"
		}
		if _, err := Apply([]byte(content), vars); err != nil {
			t.Fatalf("Apply(%q) with every token defined failed: %v", content, err)
		}
	})
}

// A var used twice was listed twice ("undefined variables: nope, nope").
func TestUndefinedVarIsNamedOnce(t *testing.T) {
	_, err := Apply([]byte("{{nope}} and {{ nope }}"), nil)
	if err == nil || strings.Count(err.Error(), "nope") != 1 {
		t.Errorf("err = %v, want nope named exactly once", err)
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
