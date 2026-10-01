package main

import (
	"strings"
	"testing"
)

func snapshotRels(t *testing.T) []string {
	t.Helper()
	s, err := loadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, r := range s.Rows {
		rels = append(rels, r.Rel)
	}
	return rels
}

// Reload (r) calls loadSnapshot again, so each call must read the disk
// afresh: a file added after launch shows up, and a broken dots.toml is
// an error the TUI can show instead of a crash.
func TestLoadSnapshotRereadsDisk(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "z\n")
	if got := snapshotRels(t); strings.Join(got, ",") != ".zshrc" {
		t.Fatalf("first load = %v, want [.zshrc]", got)
	}
	writeFile(t, s.repo("base/.vimrc"), "v\n")
	if got := snapshotRels(t); strings.Join(got, ",") != ".vimrc,.zshrc" {
		t.Fatalf("second load = %v, want the new .vimrc too", got)
	}
	writeFile(t, s.repo("dots.toml"), "[hook]\n") // typo for [hooks]
	if _, err := loadSnapshot(); err == nil {
		t.Fatal("broken dots.toml: want an error")
	}
}
