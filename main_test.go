package main

import "testing"

func TestRelFromArg(t *testing.T) {
	home := "/Users/x"
	for arg, want := range map[string]string{
		".zshrc":                         ".zshrc",
		"~/.zshrc":                       ".zshrc",
		"/Users/x/.config/nvim/init.lua": ".config/nvim/init.lua",
	} {
		got, err := relFromArg(arg, home)
		if err != nil || got != want {
			t.Errorf("relFromArg(%q) = %q, %v; want %q", arg, got, err, want)
		}
	}
	if _, err := relFromArg("/etc/hosts", home); err == nil {
		t.Error("outside home must error")
	}
}
