package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the bubbletea model. All data is computed once at launch (in
// Snapshot); Update only moves selection/tab/overlay state — the TUI is
// strictly read-only.
type Model struct {
	snap    *Snapshot
	tab     int // 0 layers, 1 files, 2 vars & rules
	sel     int // files-tab selection; persists across tab switches
	open    bool
	diff    bool // full-diff view inside the drilldown
	diffOff int
	w, h    int
}

func New(s *Snapshot) Model {
	return Model{snap: s, w: 100, h: 34}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.diff {
				m.diff, m.diffOff = false, 0
			} else {
				m.open = false
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
			case m.tab == 1 && !m.open && m.sel < len(m.snap.Rows)-1:
				m.sel++
			}
		case "enter":
			if m.tab == 1 && len(m.snap.Rows) > 0 {
				m.open = true
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

// Run launches the TUI in the alternate screen.
func Run(s *Snapshot) error {
	p := tea.NewProgram(New(s), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
