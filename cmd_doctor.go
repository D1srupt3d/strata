package main

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"strata/internal/doctor"
	"strata/internal/layers"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check this machine's setup and list every problem, each with a fix",
		Long: `Checks this machine's strata setup and lists every problem at once, each
with a fix. Other commands stop at the first broken thing; doctor keeps
going, so one run shows everything.

It checks:
  config     machine.toml, the repo folder, dots.toml, each role layer
  dots.toml  entries naming no file in any layer (substitute, hooks,
             permissions), bad ignore or permission patterns, permission
             rules that disagree, undefined {{vars}}
  state      state.json, entries outside $HOME or gone from it, pending
             hooks whose hook was removed
  install    version, whether 'strata' on your PATH is this binary, git

A check that depends on something broken is shown as skip, naming what it
needed. Doctor only reads — it never changes a file. It checks the setup,
not individual files: for drifted or conflicting files, use 'strata status'.

Exit status: 1 when anything is an error; warnings alone exit 0.`,
		Example: `  strata doctor
  strata doctor && strata apply   # apply only when the setup is sound`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolvePaths()
			if err != nil {
				return err
			}
			findings := doctor.Run(doctor.Inputs{
				Home:        p.Home,
				MachinePath: p.Machine,
				StatePath:   p.State,
				Bin:         p.Bin,
				GOOS:        runtime.GOOS,
				OSRelease:   layers.ReadOSRelease(),
				Version:     version,
				Channel:     channel,
				LookPath:    exec.LookPath,
			})
			if printFindings(cmd.OutOrStdout(), findings) > 0 {
				return exitCode(1)
			}
			return nil
		},
	}
}

// printFindings writes the report grouped by check group, one line per
// finding with its fix beneath, then a count. It returns the error count.
func printFindings(w io.Writer, findings []doctor.Finding) (errs int) {
	warns, group := 0, ""
	for _, f := range findings {
		if f.Group != group {
			if group != "" {
				fmt.Fprintln(w)
			}
			group = f.Group
			fmt.Fprintln(w, group)
		}
		line := f.Subject
		if f.Detail != "" {
			line += ": " + f.Detail
		}
		fmt.Fprintf(w, "  %-5s %s\n", f.Sev, line)
		if f.Fix != "" {
			fmt.Fprintf(w, "        fix: %s\n", f.Fix)
		}
		switch f.Sev {
		case doctor.Error:
			errs++
		case doctor.Warn:
			warns++
		}
	}
	fmt.Fprintln(w)
	if errs+warns == 0 {
		fmt.Fprintln(w, "no problems found")
	} else {
		fmt.Fprintf(w, "%s, %s\n", count(errs, "error"), count(warns, "warning"))
	}
	return errs
}

// count formats "1 error" / "2 errors".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
