package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"strata/internal/engine"
)

// Model is the bubbletea model. All data is computed once at launch (in
// Snapshot); Update only moves selection/tab/overlay state - the TUI is
// strictly read-only.
type Model struct {
	snap    *Snapshot
	tab     int // 0 layers, 1 files, 2 vars & rules
	sel     int // index into visible(); persists across tab switches
	open    bool
	diff    bool // full-diff view inside the drilldown
	diffOff int
	w, h    int

	// Files-tab filters. While typing, every key edits query instead of
	// acting as a shortcut, so searching for "q" doesn't quit.
	query     string
	typing    bool
	attention bool // hide clean files and files this machine doesn't get

	reload  func() (*Snapshot, error) // `r`: rebuild from disk; nil disables it
	note    string                    // "↻ reloaded" or the reload error, until the next key
	noteErr bool
}

// reloadMsg carries a reload's result back into Update.
type reloadMsg struct {
	snap *Snapshot
	err  error
}

func New(s *Snapshot) Model {
	return Model{snap: s, w: 100, h: 34}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case reloadMsg:
		if msg.err != nil {
			m.note, m.noteErr = "reload failed: "+msg.err.Error(), true
			return m, nil
		}
		rel := ""
		if vis := m.visible(); m.sel < len(vis) {
			rel = vis[m.sel].Rel
		}
		m.snap = msg.snap
		m.note, m.noteErr = "↻ reloaded", false
		// Keep the same file selected: rows shift when files come or go.
		for i, r := range m.visible() {
			if r.Rel == rel {
				m.sel = i
			}
		}
		m.clampSel()
		if len(m.visible()) == 0 {
			m.open, m.diff = false, false
		}
	case tea.KeyPressMsg:
		m.note = ""
		if m.typing {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "enter":
				m.typing = false
			case "esc":
				m.typing, m.query = false, ""
			case "backspace":
				r := []rune(m.query)
				if len(r) > 0 {
					m.query = string(r[:len(r)-1])
				}
			default:
				m.query += msg.Text
			}
			m.clampSel()
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			switch {
			case m.diff:
				m.diff, m.diffOff = false, 0
			case m.open:
				m.open = false
			case m.tab == 1:
				m.query, m.attention = "", false
				m.clampSel()
			}
		case "left":
			m.tab = (m.tab + 2) % 3
			m.open, m.diff = false, false
		case "right":
			m.tab = (m.tab + 1) % 3
			m.open, m.diff = false, false
		case "1", "2", "3":
			m.tab = int(msg.String()[0] - '1')
			m.open, m.diff = false, false
		case "up":
			switch {
			case m.diff:
				if m.diffOff > 0 {
					m.diffOff--
				}
			case m.tab == 1 && !m.open && m.sel > 0:
				m.sel--
			}
		case "down":
			switch {
			case m.diff:
				m.diffOff++ // clamped against content length in View
			case m.tab == 1 && !m.open && m.sel < len(m.visible())-1:
				m.sel++
			}
		case "enter":
			if m.tab == 1 && len(m.visible()) > 0 {
				m.open = true
			}
		case "r":
			if reload := m.reload; reload != nil {
				return m, func() tea.Msg {
					s, err := reload()
					return reloadMsg{s, err}
				}
			}
		case "/":
			if m.tab == 1 && !m.open {
				m.typing = true
			}
		case "a":
			if m.tab == 1 && !m.open {
				m.attention = !m.attention
				m.clampSel()
			}
		case "d":
			if m.open && !m.diff {
				m.diff = true
				m.diffOff = 0
			}
		}
	}
	return m, nil
}

// visible is the Files-tab rows that pass the filters, in snapshot order.
func (m Model) visible() []Row {
	q := strings.ToLower(m.query)
	var out []Row
	for _, r := range m.snap.Rows {
		if m.attention && !r.HookPending && (!r.Resolved || r.Status == engine.Clean) {
			continue
		}
		if !strings.Contains(strings.ToLower(r.Rel), q) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// clampSel keeps the selection inside a list a filter just narrowed.
func (m *Model) clampSel() {
	m.sel = max(min(m.sel, len(m.visible())-1), 0)
}

// Run launches the TUI in the alternate screen (declared in View). reload
// rebuilds the snapshot from disk when the user presses r.
func Run(s *Snapshot, reload func() (*Snapshot, error)) error {
	m := New(s)
	m.reload = reload
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
