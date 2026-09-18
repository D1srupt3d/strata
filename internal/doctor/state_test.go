package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoStateFileIsOK(t *testing.T) {
	e := newEnv(t)
	if f := wantSev(t, Run(e.in), "state", "state.json", OK); !strings.Contains(f.Detail, "none yet") {
		t.Errorf("detail = %q, want it to say nothing has been applied yet", f.Detail)
	}
}

func TestStateTracksOneFileSingular(t *testing.T) {
	e := newEnv(t)
	e.homeFile(t, ".zshrc", "x\n")
	e.stateFile(t, `{"version":1,"files":{".zshrc":"abc"}}`)
	f := get(t, Run(e.in), "state", "state.json")
	if f.Detail != "1 file tracked" {
		t.Errorf("detail = %q, want %q", f.Detail, "1 file tracked")
	}
}

func TestUnreadableStateIsError(t *testing.T) {
	for name, content := range map[string]string{
		"corrupt":      "{nope",
		"newer strata": `{"version": 99}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.stateFile(t, content)
			wantSev(t, Run(e.in), "state", "state.json", Error)
		})
	}
}

// engine.Plan refuses to run with such an entry, so every apply fails.
func TestStateEntryOutsideHomeIsError(t *testing.T) {
	e := newEnv(t)
	e.stateFile(t, `{"version":1,"files":{"../escape":"abc"}}`)
	if f := wantSev(t, Run(e.in), "state", `entry "../escape"`, Error); !strings.Contains(f.Fix, e.in.StatePath) {
		t.Errorf("fix = %q, want it to name the state file", f.Fix)
	}
}

func TestStateEntryGoneFromHomeWarns(t *testing.T) {
	e := newEnv(t)
	e.homeFile(t, ".bashrc", "x\n")
	e.stateFile(t, `{"version":1,"files":{".zshrc":"abc",".bashrc":"def"}}`)
	got := Run(e.in)
	wantSev(t, got, "state", `entry ".zshrc"`, Warn)
	absent(t, got, "state", `entry ".bashrc"`)
}

// engine.Plan skips ignored state entries entirely (Ignoring is not
// removing), so apply never removes them from state; doctor must not warn
// about something apply can't fix.
func TestIgnoredStateEntryGoneFromHomeIsNotWarned(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "dots.toml", "ignore = [\".old\"]\n")
	e.stateFile(t, `{"version":1,"files":{".old":"abc"}}`)
	absent(t, Run(e.in), "state", `entry ".old"`)
}

func TestPendingHookWithNoHookLeftWarns(t *testing.T) {
	e := newEnv(t)
	e.repoFile(t, "dots.toml", "[hooks]\n\".vimrc\" = \"echo ok\"\n")
	e.stateFile(t, `{"version":1,"files":{},"pending_hooks":[".vimrc",".zshrc"]}`)
	got := Run(e.in)
	wantSev(t, got, "state", `pending hook ".zshrc"`, Warn)
	absent(t, got, "state", `pending hook ".vimrc"`)
}

// Reading state needs no config, so a broken machine.toml must not hide
// state problems. Only the pending-hook check needs dots.toml.
func TestStateIsCheckedWithoutMachineToml(t *testing.T) {
	e := newEnv(t)
	mustRemove(t, e.in.MachinePath)
	e.stateFile(t, `{"version":1,"files":{"../escape":"abc"},"pending_hooks":[".zshrc"]}`)
	got := Run(e.in)
	wantSev(t, got, "state", `entry "../escape"`, Error)
	wantSev(t, got, "state", "pending hooks", Skip)
}

func TestStateEntryUnreadableInHomeIsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX file modes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	e := newEnv(t)
	e.homeFile(t, ".config/app/x", "content\n")
	e.stateFile(t, `{"version":1,"files":{".config/app/x":"abc"}}`)

	// Make the parent directory unreadable.
	dir := filepath.Join(e.in.Home, ".config", "app")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(dir, 0o755)
	})

	got := Run(e.in)
	wantSev(t, got, "state", `entry ".config/app/x"`, Error)
	absent(t, got, "state", "entries")
}
