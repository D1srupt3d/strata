package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"strata/internal/config"
	"strata/internal/engine"
	"strata/internal/state"
)

// fixture mirrors the design comp's mock repo shape with real files.
func fixture(t *testing.T) (config.RepoConfig, config.MachineConfig, string) {
	t.Helper()
	root := t.TempDir()
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(rel, content string) {
		t.Helper()
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
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

// The TUI shows what apply would do, so it refuses the same bad section.
func TestBuildRefusesTypoLayerVarsSection(t *testing.T) {
	rc, mc, home := fixture(t)
	rc.LayerVars = map[string]map[string]string{"wrok": {"email": "x"}}
	if _, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "mbp-work"); err == nil {
		t.Fatal("Build accepted [layer_vars.wrok]")
	}
}

// A value from a layer's section shows that section as its source and the
// [vars] default it replaced, and the label reads in full: the FROM column
// used to be a fixed 14 characters and cut longer labels off.
func TestVarsTabShowsLayerVarsSource(t *testing.T) {
	rc, mc, home := fixture(t)
	rc.Vars["palette"] = "everforest"
	rc.LayerVars = map[string]map[string]string{"work": {"palette": "dracula"}}
	s, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "mbp-work")
	if err != nil {
		t.Fatal(err)
	}
	var palette VarRow
	for _, v := range s.Vars {
		if v.Name == "palette" {
			palette = v
		}
	}
	if palette.Value != "dracula" || palette.From != "dots.toml [layer_vars.work]" ||
		!palette.Overridden || palette.Default != "everforest" {
		t.Errorf("palette var: %+v", palette)
	}

	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m, _ = m.Update(key("3")) // Vars tab
	if v := m.View().Content; !strings.Contains(v, "dots.toml [layer_vars.work]") {
		t.Errorf("vars tab cuts the source label short:\n%s", v)
	}
}

func key(k string) tea.KeyPressMsg {
	switch k {
	case "up", "down", "left", "right", "enter", "esc", "backspace":
		codes := map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
			"right": tea.KeyRight, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "backspace": tea.KeyBackspace}
		return tea.KeyPressMsg{Code: codes[k]}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func TestModelNavigation(t *testing.T) {
	s := build(t)
	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})

	if !strings.Contains(m.View().Content, "strata") {
		t.Fatal("header missing")
	}

	m, _ = m.Update(key("2")) // Files tab
	v := m.View().Content
	if !strings.Contains(v, ".gitconfig") || !strings.Contains(v, "WINS HERE") {
		t.Fatalf("files tab missing content:\n%s", v)
	}

	m, _ = m.Update(key("down"))
	m, _ = m.Update(key("enter"))
	v = m.View().Content
	if !strings.Contains(v, "SOURCE → DESTINATION") || !strings.Contains(v, "RESOLVES ON") {
		t.Fatalf("drilldown missing:\n%s", v)
	}

	m, _ = m.Update(key("esc"))
	m, _ = m.Update(key("3"))
	v = m.View().Content
	if !strings.Contains(v, "VALUE HERE") || !strings.Contains(v, "you@work.example") {
		t.Fatalf("vars tab missing:\n%s", v)
	}

	m, _ = m.Update(key("right")) // wraps to Layers
	v = m.View().Content
	if !strings.Contains(v, "↷ work") {
		t.Fatalf("layers tab should show override marker:\n%s", v)
	}
}

// The drilldown diff uses the same headers as 'strata diff': without the
// status, a drifted file's diff looked just like a pending update.
func TestDiffLinesNameTheNewerSide(t *testing.T) {
	r := Row{Rel: ".zshrc", Item: engine.Item{Rel: ".zshrc", Status: engine.Drifted,
		Current: []byte("mine\n"), Desired: []byte("repo\n")}}
	lines := diffLines(r)
	if len(lines) < 2 || lines[0] != "--- home/.zshrc  (drifted: $HOME has the newer edit)" || lines[1] != "+++ repo/.zshrc" {
		t.Fatalf("headers: %q", lines)
	}
}

// press feeds keys to the model in order; "/ssh" is typed as "/", "s", "s", "h".
func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func rels(m tea.Model) []string {
	var out []string
	for _, r := range m.(Model).visible() {
		out = append(out, r.Rel)
	}
	return out
}

// filterFixture is the usual fixture with .zshrc already in $HOME, so one
// row is clean and the attention filter has something to hide.
func filterFixture(t *testing.T) tea.Model {
	t.Helper()
	rc, mc, home := fixture(t)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export EDITOR=nvim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "mbp-work")
	if err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	return press(m, "2")
}

func TestAttentionFilterHidesCleanAndUnresolved(t *testing.T) {
	m := filterFixture(t)
	all := rels(m)
	if !slices.Contains(all, ".zshrc") || !slices.Contains(all, ".wslconfig") {
		t.Fatalf("unfiltered rows should include clean .zshrc and windows-only .wslconfig: %v", all)
	}
	m = press(m, "a")
	got := rels(m)
	if slices.Contains(got, ".zshrc") || slices.Contains(got, ".wslconfig") {
		t.Errorf("attention filter kept a clean or unresolved row: %v", got)
	}
	if !slices.Contains(got, ".ssh/config") {
		t.Errorf("attention filter dropped .ssh/config (create): %v", got)
	}
	if v := m.View().Content; !strings.Contains(v, "needs attention") || !strings.Contains(v, fmt.Sprintf("%d of %d files", len(got), len(all))) {
		t.Errorf("footer should name the filter and count:\n%s", v)
	}
	if got := rels(press(m, "a")); len(got) != len(all) {
		t.Errorf("second a should turn the filter off: %v", got)
	}
}

// While typing, every key is text: q must not quit, d must not open a diff.
func TestSearchTypesKeysAndOpensTheRightRow(t *testing.T) {
	m := filterFixture(t)
	m = press(m, "/", "s", "s", "h")
	if got := rels(m); !slices.Equal(got, []string{".ssh/config"}) {
		t.Fatalf("/ssh = %v, want [.ssh/config]", got)
	}
	m2, cmd := m.Update(key("q"))
	if cmd != nil {
		t.Fatal("q while typing returned a command (quit)")
	}
	if q := m2.(Model).query; q != "sshq" {
		t.Fatalf("query = %q, want sshq", q)
	}
	m = press(m, "enter", "enter") // stop typing, then open the drilldown
	if !m.(Model).open || m.(Model).currentRow().Rel != ".ssh/config" {
		t.Fatalf("drilldown opened %q, want .ssh/config", m.(Model).currentRow().Rel)
	}
}

func TestSearchIgnoresCaseAndBackspaceWidens(t *testing.T) {
	m := press(filterFixture(t), "/", "Z", "S", "H", "x")
	if got := rels(m); len(got) != 0 {
		t.Fatalf("/ZSHx should match nothing: %v", got)
	}
	if v := m.View().Content; !strings.Contains(v, "no files match") {
		t.Errorf("empty result should say so:\n%s", v)
	}
	m = press(m, "enter", "enter") // enter on an empty list must not open (or panic)
	if m.(Model).open {
		t.Fatal("drilldown opened on an empty list")
	}
	m = press(m, "/", "backspace")
	if got := rels(m); !slices.Equal(got, []string{".zshrc"}) {
		t.Fatalf("/ZSH = %v, want [.zshrc]", got)
	}
}

func TestEscClearsFiltersAndEmptyMessage(t *testing.T) {
	m := press(filterFixture(t), "a", "/", "z", "s", "h", "enter")
	if got := rels(m); len(got) != 0 {
		t.Fatalf("clean .zshrc under attention filter = %v, want none", got)
	}
	if v := m.View().Content; !strings.Contains(v, `nothing matching "zsh" needs attention`) {
		t.Errorf("empty list should name both filters:\n%s", v)
	}
	m = press(m, "esc")
	if mm := m.(Model); mm.query != "" || mm.attention {
		t.Fatalf("esc left filters on: query=%q attention=%v", mm.query, mm.attention)
	}
}

// The selection points into the filtered list: narrowing it must not leave
// sel past the end.
func TestFilterClampsSelection(t *testing.T) {
	m := filterFixture(t)
	for range len(rels(m)) {
		m = press(m, "down")
	}
	m = press(m, "/", "s", "s", "h", "enter", "enter")
	if r := m.(Model).currentRow(); r.Rel != ".ssh/config" {
		t.Fatalf("opened %q after narrowing, want .ssh/config", r.Rel)
	}
}

// `a` must list what `strata status` lists: a clean file whose hook failed,
// and a file that left every layer. Neither showed in the TUI before.
func TestAttentionMatchesStatusForHooksAndRemoved(t *testing.T) {
	rc, mc, home := fixture(t)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export EDITOR=nvim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc.Hooks[".zshrc"] = "source ~/.zshrc"
	st := state.State{
		Files:        map[string]string{".old": "deadbeefdeadbeef"},
		PendingHooks: []string{".zshrc", ".gone"}, // .gone's hook left dots.toml: status ignores it
	}
	s, err := Build(rc, mc, home, st, "darwin", "", "mbp-work")
	if err != nil {
		t.Fatal(err)
	}
	if r := rowByRel(t, s, ".zshrc"); !r.HookPending || r.Status != engine.Clean {
		t.Errorf(".zshrc: HookPending=%v Status=%v, want pending hook on a clean file", r.HookPending, r.Status)
	}
	if r := rowByRel(t, s, ".old"); !r.Resolved || r.Status != engine.Removed {
		t.Errorf(".old: Resolved=%v Status=%v, want a removed row", r.Resolved, r.Status)
	}
	for _, r := range s.Rows {
		if r.Rel == ".gone" {
			t.Error(".gone: a pending hook no longer in dots.toml must not get a row")
		}
	}

	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = press(m, "2", "a")
	got := rels(m)
	if !slices.Contains(got, ".zshrc") || !slices.Contains(got, ".old") {
		t.Errorf("attention filter = %v, want .zshrc (hook) and .old (removed)", got)
	}
	v := ansiRe.ReplaceAllString(m.View().Content, "")
	if !regexp.MustCompile(`\.zshrc .*⚙ hook`).MatchString(v) {
		t.Errorf(".zshrc row should show ⚙ hook:\n%s", v)
	}
	if !regexp.MustCompile(`\.old .*✕ removed`).MatchString(v) {
		t.Errorf(".old row should show removed:\n%s", v)
	}

	m = press(m, "/", "z", "s", "h", "enter", "enter")
	if v := m.View().Content; !strings.Contains(v, "pending: apply retries it") {
		t.Errorf("drilldown should say the hook is pending:\n%s", v)
	}
}

// pressR presses r and feeds the reload's result back, as bubbletea would.
func pressR(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	m, cmd := m.Update(key("r"))
	if cmd == nil {
		t.Fatal("r returned no reload command")
	}
	m, _ = m.Update(cmd())
	return m
}

func TestReloadKeepsSelectionAndFilters(t *testing.T) {
	rc, mc, home := fixture(t)
	s1, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "h")
	if err != nil {
		t.Fatal(err)
	}
	// After the reload a new file sorts first, shifting every row down one.
	if err := os.WriteFile(filepath.Join(mc.Repo, "base", ".aaa"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s2, err := Build(rc, mc, home, state.State{Files: map[string]string{}}, "darwin", "", "h")
	if err != nil {
		t.Fatal(err)
	}
	m0 := New(s1)
	m0.reload = func() (*Snapshot, error) { return s2, nil }
	var m tea.Model = m0
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = press(m, "2", "a")
	for m.(Model).currentRow().Rel != ".ssh/config" {
		m = press(m, "down")
	}

	m = pressR(t, m)
	mm := m.(Model)
	if mm.snap != s2 {
		t.Fatal("reload did not swap in the new snapshot")
	}
	if !mm.attention {
		t.Error("reload turned the attention filter off")
	}
	if r := mm.currentRow(); r.Rel != ".ssh/config" {
		t.Errorf("selection after reload = %q, want .ssh/config (found by path, not row number)", r.Rel)
	}
	if !slices.Contains(rels(m), ".aaa") {
		t.Errorf("new file missing after reload: %v", rels(m))
	}
	if v := m.View().Content; !strings.Contains(v, "reloaded") {
		t.Errorf("view should confirm the reload:\n%s", v)
	}
	if v := press(m, "down").View().Content; strings.Contains(v, "reloaded") {
		t.Error("reload note should clear on the next key")
	}
}

// A dots.toml mid-edit must not kill the TUI: keep the old data, say why.
func TestReloadFailureKeepsOldData(t *testing.T) {
	s := build(t)
	m0 := New(s)
	m0.reload = func() (*Snapshot, error) { return nil, errors.New("dots.toml: boom") }
	var m tea.Model = m0
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = pressR(t, m)
	if m.(Model).snap != s {
		t.Fatal("failed reload replaced the snapshot")
	}
	if v := m.View().Content; !strings.Contains(v, "reload failed: dots.toml: boom") {
		t.Errorf("view should show the reload error:\n%s", v)
	}
}

func TestRWhileTypingIsText(t *testing.T) {
	m0 := New(build(t))
	m0.reload = func() (*Snapshot, error) { t.Fatal("r while typing reloaded"); return nil, nil }
	m, cmd := tea.Model(m0).Update(key("2"))
	m = press(m, "/")
	m, cmd = m.Update(key("r"))
	if cmd != nil || m.(Model).query != "r" {
		t.Fatalf("query = %q, cmd = %v; want r typed, no reload", m.(Model).query, cmd)
	}
}
