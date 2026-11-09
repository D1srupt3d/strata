package main

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
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

// statusJSON runs `status --json` and decodes it, failing the test if the
// output isn't exactly one JSON document.
func statusJSON(t *testing.T) (statusReport, error) {
	t.Helper()
	out, err := run(t, "status", "--json")
	var r statusReport
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&r); derr != nil {
		t.Fatalf("status --json is not valid JSON: %v\n%s", derr, out)
	}
	return r, err
}

// --json lists every managed file, clean ones included (a script can filter
// what it doesn't need, but can't recover what was never printed), with the
// layer that provides it. The exit code is the same as the text output's.
func TestStatusJSONListsEveryFile(t *testing.T) {
	s := sandbox(t, "work")
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	writeFile(t, s.repo("work/.gitconfig"), "work\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.home(".zshrc"), "my edit\n")

	r, err := statusJSON(t)
	if code, msg := exitStatus(err); code != 1 || msg != "" {
		t.Fatalf("exit %d, message %q; want exit 1 and no message", code, msg)
	}
	if r.Clean {
		t.Error("clean = true with a drifted file")
	}
	want := []statusFile{
		{Path: ".gitconfig", Status: "clean", Layer: "work"},
		{Path: ".zshrc", Status: "drifted", Layer: "base"},
	}
	if !reflect.DeepEqual(r.Files, want) {
		t.Errorf("files = %+v\nwant %+v", r.Files, want)
	}
	// An empty list, not null, so `jq '.pending_hooks | length'` just works.
	if r.PendingHooks == nil || len(r.PendingHooks) != 0 {
		t.Errorf("pending_hooks = %#v, want []", r.PendingHooks)
	}
}

func TestStatusJSONClean(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	r, err := statusJSON(t)
	if err != nil {
		t.Fatalf("clean status --json: err = %v, want exit 0", err)
	}
	if !r.Clean || len(r.Files) != 1 || r.Files[0].Status != "clean" {
		t.Errorf("report = %+v, want clean with one clean file", r)
	}
}

// A removed file has no layer left, so the layer field is left out.
func TestStatusJSONRemovedFileHasNoLayer(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "repo\n")
	if _, err := run(t, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.repo("base/.zshrc")); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, "status", "--json")
	if strings.Contains(out, `"layer"`) {
		t.Errorf("removed file still has a layer field:\n%s", out)
	}
	r, _ := statusJSON(t)
	if r.Clean || len(r.Files) != 1 || r.Files[0].Status != "removed" {
		t.Errorf("report = %+v, want one removed file", r)
	}
}

// A pending hook makes the report not clean, even when every file is.
func TestStatusJSONPendingHook(t *testing.T) {
	hookSandbox(t)
	if _, err := run(t, "apply"); err == nil {
		t.Fatal("apply: want the hook failure reported")
	}
	r, err := statusJSON(t)
	if code, _ := exitStatus(err); code != 1 {
		t.Errorf("exit %d, want 1 while a hook is pending", code)
	}
	if r.Clean {
		t.Error("clean = true while a hook is pending")
	}
	if !reflect.DeepEqual(r.PendingHooks, []string{".zshrc"}) {
		t.Errorf("pending_hooks = %#v, want [.zshrc]", r.PendingHooks)
	}
}
