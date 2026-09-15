package main

import (
	"errors"
	"strings"
	"testing"
)

// status exits 1 when anything needs attention, so scripts and prompts can
// test it (`strata status || strata diff`). It's a status, not an error:
// main must not print an "error:" line for it.
func TestStatusExitsNonZeroOnlyWhenAttentionNeeded(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "status"); err != nil {
		t.Fatalf("clean status: err = %v, want exit 0", err)
	}

	writeFile(t, s.home(".zshrc"), "my edit\n")
	out, err := run(t, "status")
	if code, msg := exitStatus(err); code != 1 || msg != "" {
		t.Fatalf("dirty status: exit %d, message %q; want exit 1 and no message", code, msg)
	}
	if !strings.Contains(out, "drifted") {
		t.Errorf("status output doesn't list the drifted file:\n%s", out)
	}
}

func TestExitStatusPrintsRealErrors(t *testing.T) {
	if code, msg := exitStatus(nil); code != 0 || msg != "" {
		t.Errorf("exitStatus(nil) = %d, %q; want 0, \"\"", code, msg)
	}
	if code, msg := exitStatus(errors.New("boom")); code != 1 || msg != "error: boom" {
		t.Errorf("exitStatus(boom) = %d, %q; want 1, \"error: boom\"", code, msg)
	}
}
