package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"strata/internal/config"
	"strata/internal/layers"
)

// loaded is what checkConfig managed to load. Later groups run the checks
// whose inputs are here, and report Skip — naming the dependency — for the rest.
type loaded struct {
	machine bool          // machine.toml parsed
	repo    bool          // the repo folder exists
	dots    bool          // dots.toml parsed
	roles   bool          // every role layer is a valid folder
	cfg     config.Config // RepoDir/RoleLayers valid if machine; the rest if dots
}

// checkConfig loads machine.toml, the repo and dots.toml the way loadContext
// does, but reports each failure and keeps going where it can.
func checkConfig(r *report, in Inputs) loaded {
	r.group = "config"
	var l loaded

	mc, err := config.LoadMachineConfig(in.MachinePath)
	if err != nil {
		fix := "fix the line or key the error names"
		if errors.Is(err, fs.ErrNotExist) {
			fix = "run 'strata init' to set up this machine"
		}
		r.add(Error, "machine.toml", err.Error(), fix)
		r.add(Skip, "repo, git, dots.toml, layers", "need a readable machine.toml", "")
		return l
	}
	l.machine, l.cfg = true, config.Merge(config.RepoConfig{}, mc)
	r.add(OK, "machine.toml", in.MachinePath, "")

	info, err := os.Stat(mc.Repo)
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("%s is not a folder", mc.Repo)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.add(Error, "repo", err.Error()+" — every command reads a missing repo as an empty one, so every managed file reads as removed and apply would delete them",
				"clone your dotfiles repo there, or set repo in machine.toml to where it is")
		} else {
			r.add(Error, "repo", err.Error()+" — commands that read the repo fail until this is fixed",
				"set repo in machine.toml to your dotfiles folder")
		}
		r.add(Skip, "git, dots.toml, layers", "need the repo folder", "")
		return l
	}
	l.repo = true
	r.add(OK, "repo", mc.Repo, "")

	// .git is a folder in a normal clone and a file in a worktree; either
	// counts, at the repo itself or at any ancestor — 'strata init' and
	// 'strata sync' both work when the dotfiles repo is a subfolder of a
	// git repo. git itself isn't run: doctor only reads.
	found := false
	for dir := mc.Repo; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			found = true
			if dir == mc.Repo {
				r.add(OK, "git", "the repo is a git repository", "")
			} else {
				r.add(OK, "git", "the repo is inside the git repository at "+dir, "")
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if !found {
		r.add(Warn, "git", "the repo is not a git repository", "'strata sync' needs one: run 'git init' in the repo, or clone it again")
	}

	rc, err := config.LoadRepoConfig(mc.Repo)
	if err != nil {
		r.add(Error, "dots.toml", err.Error(), "fix the line or key the error names")
	} else {
		l.dots, l.cfg = true, config.Merge(rc, mc)
		detail := filepath.Join(mc.Repo, "dots.toml")
		if _, err := os.Stat(detail); errors.Is(err, fs.ErrNotExist) {
			detail = "none, using defaults"
		}
		r.add(OK, "dots.toml", detail, "")
	}

	l.roles = true
	for _, role := range mc.Layers {
		subject := fmt.Sprintf("layer %q", role)
		// One role at a time, so every bad layer is listed, not just the first.
		if err := layers.CheckRoles(mc.Repo, []string{role}); err != nil {
			l.roles = false
			r.add(Error, subject, err.Error(), "fix the name in machine.toml, or create the folder in the repo")
			continue
		}
		r.add(OK, subject, "", "")
	}
	return l
}
