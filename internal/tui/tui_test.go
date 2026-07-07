package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"strata/internal/config"
	"strata/internal/state"
)

// fixture mirrors the design comp's mock repo shape with real files.
func fixture(t *testing.T) (config.RepoConfig, config.MachineConfig, string) {
	t.Helper()
	root := t.TempDir()
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	mk := func(rel, content string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	mk("base/.zshrc", "export EDITOR=nvim\n")
	mk("base/.gitconfig", "[user]\n\temail = {{email}}\n")
	mk("base/.config/alacritty/alacritty.toml", "base alacritty\n")
	mk("mac/.Brewfile", "brew \"gh\"\n")
	mk("linux/.config/alacritty/alacritty.toml", "linux alacritty\n")
	mk("windows/.wslconfig", "[wsl2]\n")
	mk("work/.gitconfig", "[user]\n\temail = {{email}}\nwork = true\n")
	mk("work/.ssh/config", "Host *\n")
	rc := config.RepoConfig{
		Substitute:  []string{".gitconfig"},
		Vars:        map[string]string{"email": "personal@example.com", "editor": "nvim"},
		Permissions: map[string]string{".ssh/**": "600"},
		Hooks:       map[string]string{".Brewfile": "brew bundle"},
	}
	mc := config.MachineConfig{
		Repo:   repo,
		Layers: []string{"work"},
		Vars:   map[string]string{"email": "you@work.example"},
	}
	return rc, mc, home
}

func build(t *testing.T) *Snapshot {
	t.Helper()
	rc, mc, home := fixture(t)
	s, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "mbp-work")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func rowByRel(t *testing.T, s *Snapshot, rel string) Row {
	t.Helper()
	for _, r := range s.Rows {
		if r.Rel == rel {
			return r
		}
	}
	t.Fatalf("row %s not found in %d rows", rel, len(s.Rows))
	return Row{}
}

func TestSnapshotResolution(t *testing.T) {
	s := build(t)

	git := rowByRel(t, s, ".gitconfig")
	if git.Winner != "work" || git.Mac != "work" || git.Linux != "work" || git.Win != "work" {
		t.Errorf("gitconfig resolution: %+v", git)
	}
	if !strings.Contains(git.Badge, "{{ }}") {
		t.Errorf("gitconfig badge = %q", git.Badge)
	}
	if len(git.SubstVars) != 1 || git.SubstVars[0].Name != "email" ||
		git.SubstVars[0].Value != "you@work.example" || git.SubstVars[0].From != "machine.toml" {
		t.Errorf("gitconfig subst vars = %+v", git.SubstVars)
	}

	brew := rowByRel(t, s, ".Brewfile")
	if brew.Winner != "mac" || brew.Linux != "" || brew.Win != "" || brew.Hook == "" {
		t.Errorf("brewfile: %+v", brew)
	}

	wsl := rowByRel(t, s, ".wslconfig")
	if wsl.Resolved || wsl.Winner != "" || wsl.Win != "windows" {
		t.Errorf("wslconfig should be unresolved on darwin: %+v", wsl)
	}

	ala := rowByRel(t, s, ".config/alacritty/alacritty.toml")
	if ala.Winner != "base" || ala.Linux != "linux" {
		t.Errorf("alacritty: %+v", ala)
	}

	ssh := rowByRel(t, s, ".ssh/config")
	if ssh.Perm != "600 (dots.toml)" {
		t.Errorf("ssh perm = %q", ssh.Perm)
	}
}

func TestSnapshotLayersAndVars(t *testing.T) {
	s := build(t)

	var base, linux Layer
	for _, ly := range s.Layers {
		switch ly.Name {
		case "base":
			base = ly
		case "linux":
			linux = ly
		}
	}
	if !base.Active || linux.Active {
		t.Errorf("active flags wrong: base=%v linux=%v", base.Active, linux.Active)
	}
	foundOverridden := false
	for _, f := range base.Files {
		if f.Short == ".gitconfig" && f.OverriddenBy == "work" {
			foundOverridden = true
		}
	}
	if !foundOverridden {
		t.Errorf("base/.gitconfig should show ↷ work: %+v", base.Files)
	}

	var email VarRow
	for _, v := range s.Vars {
		if v.Name == "email" {
			email = v
		}
	}
	if !email.Overridden || email.From != "machine.toml" || email.Default != "personal@example.com" {
		t.Errorf("email var: %+v", email)
	}
}

func key(k string) tea.KeyMsg {
	switch k {
	case "up", "down", "left", "right", "enter", "esc":
		types := map[string]tea.KeyType{"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
			"right": tea.KeyRight, "enter": tea.KeyEnter, "esc": tea.KeyEscape}
		return tea.KeyMsg{Type: types[k]}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func TestModelNavigation(t *testing.T) {
	s := build(t)
	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})

	if !strings.Contains(m.View(), "strata") {
		t.Fatal("header missing")
	}

	m, _ = m.Update(key("2")) // Files tab
	v := m.View()
	if !strings.Contains(v, ".gitconfig") || !strings.Contains(v, "WINS HERE") {
		t.Fatalf("files tab missing content:\n%s", v)
	}

	m, _ = m.Update(key("down"))
	m, _ = m.Update(key("enter"))
	v = m.View()
	if !strings.Contains(v, "SOURCE → DESTINATION") || !strings.Contains(v, "RESOLVES ON") {
		t.Fatalf("drilldown missing:\n%s", v)
	}

	m, _ = m.Update(key("esc"))
	m, _ = m.Update(key("3"))
	v = m.View()
	if !strings.Contains(v, "VALUE HERE") || !strings.Contains(v, "you@work.example") {
		t.Fatalf("vars tab missing:\n%s", v)
	}

	m, _ = m.Update(key("right")) // wraps to Layers
	v = m.View()
	if !strings.Contains(v, "↷ work") {
		t.Fatalf("layers tab should show override marker:\n%s", v)
	}
}
