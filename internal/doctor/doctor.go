// Package doctor checks a strata setup and reports every problem at once.
//
// Every other command starts in main.go's loadContext, which stops at the
// first failure: a broken machine.toml hides a missing repo, which hides a
// hook for a file that doesn't exist. Doctor runs each check on its own and
// lists them all, each with a fix. A check whose input failed to load is
// reported as skipped, naming what it needed — never silently dropped.
//
// Checks call the packages apply uses (config, layers, perms, subst, state)
// rather than restating their rules, so doctor reports what apply would hit.
// Doctor only reads: it never saves state or takes the state lock, so it is
// safe to run at any time. Like engine.Plan, the platform is a parameter
// (Inputs.GOOS), so tests can check a Linux machine from a Mac.
//
// The scope is the setup, not individual files: drifted or conflicting
// files are 'strata status'.
package doctor

// Severity is how bad a finding is. Only Error makes doctor exit non-zero.
type Severity int

const (
	OK    Severity = iota
	Warn           // works, but probably not what you meant
	Error          // a command will fail, or do the wrong thing
	Skip           // not checked: something it depends on failed to load
)

// String is a positional array: append new severities at the end.
func (s Severity) String() string {
	return [...]string{"ok", "warn", "error", "skip"}[s]
}

// Finding is one line of the report.
type Finding struct {
	Group   string // config, dots.toml, state, install
	Sev     Severity
	Subject string // what was checked, e.g. `layer "work"`
	Detail  string // what doctor found; may be empty for OK
	Fix     string // what to do about it; empty for OK and Skip
}

// Inputs is everything doctor needs from outside the files it checks.
// cmd_doctor.go fills it from the real machine; tests fake any of it.
type Inputs struct {
	Home, MachinePath, StatePath string
	Bin                          string // this strata binary; "" if unknown
	GOOS, OSRelease              string
	Version, Channel             string
	LookPath                     func(string) (string, error) // exec.LookPath
}

// Run runs every check group in order and returns all findings.
func Run(in Inputs) []Finding {
	r := &report{}
	l := checkConfig(r, in)
	checkDotsToml(r, in, l)
	checkState(r, in, l)
	checkInstall(r, in)
	return r.findings
}

// report collects findings, tagging each with the group being checked.
type report struct {
	group    string
	findings []Finding
}

func (r *report) add(sev Severity, subject, detail, fix string) {
	r.findings = append(r.findings, Finding{Group: r.group, Sev: sev, Subject: subject, Detail: detail, Fix: fix})
}

// okIfClean adds an OK line when nothing was reported since mark, so a
// check that found no problems still shows that it ran.
func (r *report) okIfClean(mark int, subject, detail string) {
	if len(r.findings) == mark {
		r.add(OK, subject, detail, "")
	}
}
