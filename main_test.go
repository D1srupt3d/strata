package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestRelFromArg(t *testing.T) {
	// The fake home must be absolute on the OS under test: "/Users/x" is
	// not absolute on Windows (no drive letter), which sends relFromArg
	// down the wrong branch.
	home, outside := "/Users/x", "/etc/hosts"
	if runtime.GOOS == "windows" {
		home, outside = `C:\Users\x`, `C:\Windows\system.ini`
	}
	for arg, want := range map[string]string{
		".zshrc":   ".zshrc",
		"~/.zshrc": ".zshrc",
		filepath.Join(home, ".config", "nvim", "init.lua"): ".config/nvim/init.lua",
	} {
		got, err := relFromArg(arg, home)
		if err != nil || got != want {
			t.Errorf("relFromArg(%q) = %q, %v; want %q", arg, got, err, want)
		}
	}
	if _, err := relFromArg(outside, home); err == nil {
		t.Error("outside home must error")
	}
}
