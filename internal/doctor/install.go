package doctor

import (
	"fmt"
	"path/filepath"
)

// checkInstall reports which strata this is, and whether typing 'strata'
// and 'git' in a shell finds what strata expects.
func checkInstall(r *report, in Inputs) {
	r.group = "install"
	r.add(OK, "strata", fmt.Sprintf("version %s, %s build", in.Version, in.Channel), "")

	onPath, err := in.LookPath("strata")
	switch {
	case in.Bin == "":
		r.add(Skip, "PATH", "can't tell where this strata binary is", "")
	case err != nil:
		r.add(Warn, "PATH", "no strata on your PATH, so typing 'strata' won't run "+in.Bin,
			"add its folder to PATH in your shell config")
	case !samePath(onPath, in.Bin):
		r.add(Warn, "PATH", fmt.Sprintf("typing 'strata' runs %s, not this binary (%s)", onPath, in.Bin),
			"remove the other copy, or move this one's folder earlier in PATH")
	default:
		r.add(OK, "PATH", onPath, "")
	}

	if gitPath, err := in.LookPath("git"); err != nil {
		r.add(Warn, "git", "not found on PATH — 'strata init <url>' and 'strata sync' need it", "install git")
	} else {
		r.add(OK, "git", gitPath, "")
	}
}

// samePath reports whether a and b are the same file once symlinks are
// followed. If either can't be resolved, it falls back to comparing the
// cleaned paths.
func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}
