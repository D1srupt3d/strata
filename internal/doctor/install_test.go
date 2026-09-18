package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallReportsVersionAndChannel(t *testing.T) {
	e := newEnv(t)
	f := wantSev(t, Run(e.in), "install", "strata", OK)
	if !strings.Contains(f.Detail, "2026.9.9") || !strings.Contains(f.Detail, "source") {
		t.Errorf("detail = %q, want version and channel", f.Detail)
	}
}

func TestThisBinaryOnPathIsOK(t *testing.T) {
	wantSev(t, Run(newEnv(t).in), "install", "PATH", OK)
}

// A stale copy earlier on PATH means 'strata' in a shell isn't this build.
func TestOtherStrataFirstOnPathWarns(t *testing.T) {
	e := newEnv(t)
	other := filepath.Join(t.TempDir(), "strata")
	mustWrite(t, other, "")
	e.in.LookPath = fakePath(map[string]string{"strata": other, "git": "/usr/bin/git"})
	if f := wantSev(t, Run(e.in), "install", "PATH", Warn); !strings.Contains(f.Detail, other) {
		t.Errorf("detail = %q, want it to name %s", f.Detail, other)
	}
}

func TestNoStrataOnPathWarns(t *testing.T) {
	e := newEnv(t)
	e.in.LookPath = fakePath(map[string]string{"git": "/usr/bin/git"})
	wantSev(t, Run(e.in), "install", "PATH", Warn)
}

// A PATH entry is often a symlink to the real binary; that's the same strata.
func TestSymlinkToThisBinaryOnPathIsOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs extra privileges on Windows")
	}
	e := newEnv(t)
	link := filepath.Join(t.TempDir(), "strata")
	if err := os.Symlink(e.in.Bin, link); err != nil {
		t.Fatal(err)
	}
	e.in.LookPath = fakePath(map[string]string{"strata": link, "git": "/usr/bin/git"})
	wantSev(t, Run(e.in), "install", "PATH", OK)
}

func TestNoGitOnPathWarns(t *testing.T) {
	e := newEnv(t)
	e.in.LookPath = fakePath(map[string]string{"strata": e.in.Bin})
	wantSev(t, Run(e.in), "install", "git", Warn)
}

// A setup that uses every feature correctly reports no problems and skips
// nothing: the checks must not cry wolf.
func TestCleanSetupHasNoProblems(t *testing.T) {
	e := newEnv(t, "work")
	e.repoFile(t, "base/.zshrc", "x\n")
	e.repoFile(t, "work/.gitconfig", "email = {{email}}\n")
	e.repoFile(t, "dots.toml", `substitute = [".gitconfig"]
ignore = ["**/*.bak"]
[vars]
email = "me@example.com"
[hooks]
".zshrc" = "echo ok"
[permissions]
".gitconfig" = "600"
`)
	e.homeFile(t, ".zshrc", "x\n")
	e.stateFile(t, `{"version":1,"files":{".zshrc":"abc"},"pending_hooks":[".zshrc"]}`)
	got := Run(e.in)
	if p := problems(got); len(p) > 0 {
		t.Errorf("clean setup reported problems:\n%s", dump(p))
	}
	for _, f := range got {
		if f.Sev == Skip {
			t.Errorf("clean setup skipped a check:\n%s", dump(got))
			break
		}
	}
}
