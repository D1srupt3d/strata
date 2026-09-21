package main

import (
	"bytes"
	"strings"
	"testing"

	"strata/internal/doctor"
)

func TestPrintFindingsGroupsAndCounts(t *testing.T) {
	var out bytes.Buffer
	errs := printFindings(&out, []doctor.Finding{
		{Group: "config", Sev: doctor.OK, Subject: "machine.toml", Detail: "/m.toml"},
		{Group: "config", Sev: doctor.Error, Subject: `layer "wrok"`, Detail: "no folder", Fix: "fix the name"},
		{Group: "install", Sev: doctor.Warn, Subject: "git", Detail: "not found", Fix: "install git"},
		{Group: "install", Sev: doctor.OK, Subject: "strata"},
	})
	want := "config\n" +
		"  ok    machine.toml: /m.toml\n" +
		"  error layer \"wrok\": no folder\n" +
		"        fix: fix the name\n" +
		"\n" +
		"install\n" +
		"  warn  git: not found\n" +
		"        fix: install git\n" +
		"  ok    strata\n" +
		"\n" +
		"1 error, 1 warning\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	if errs != 1 {
		t.Errorf("errs = %d, want 1", errs)
	}
}

func TestPrintFindingsNoProblems(t *testing.T) {
	var out bytes.Buffer
	printFindings(&out, []doctor.Finding{{Group: "install", Sev: doctor.OK, Subject: "git"}})
	if !strings.HasSuffix(out.String(), "\nno problems found\n") {
		t.Errorf("output doesn't end with 'no problems found':\n%s", out.String())
	}
}

// An error is an answer, like status: exit 1, no "error:" line. Doctor must
// also keep going past it and report every other group.
func TestDoctorExitsOneOnErrorAndKeepsGoing(t *testing.T) {
	sandbox(t, "wrok") // role layer with no folder
	out, err := run(t, "doctor")
	if code, msg := exitStatus(err); code != 1 || msg != "" {
		t.Fatalf("exit %d, message %q; want exit 1 and no message\n%s", code, msg, out)
	}
	for _, want := range []string{`error layer "wrok"`, "fix:", "\nstate\n", "\ninstall\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// The sandbox repo has no .git, which is a warning - and warnings alone
// exit 0. (On a dev machine PATH may add a warning of its own; still exit 0.)
func TestDoctorWarningsAloneExitZero(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	writeFile(t, s.repo("dots.toml"), "[hooks]\n\".nope\" = \"echo hi\"\n")
	out, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("err = %v, want exit 0\n%s", err, out)
	}
	if !strings.Contains(out, `warn  hook ".nope"`) {
		t.Errorf("output missing the no-op hook warning:\n%s", out)
	}
}

// Doctor only reads: it must not create state.json or take the state lock.
func TestDoctorNeverWritesState(t *testing.T) {
	s := sandbox(t)
	writeFile(t, s.repo("base/.zshrc"), "x\n")
	if _, err := run(t, "doctor"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{s.State, s.State + ".lock"} {
		if exists(p) {
			t.Errorf("doctor created %s; it must only read", p)
		}
	}
}
