package tui

import (
	"regexp"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestDemoRender prints stripped frames for visual inspection (-v).
func TestDemoRender(t *testing.T) {
	s := build(t)
	var m tea.Model = New(s)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	m, _ = m.Update(key("2"))
	t.Log("FILES TAB:\n" + ansiRe.ReplaceAllString(m.View(), ""))

	m, _ = m.Update(key("enter"))
	t.Log("DRILLDOWN:\n" + ansiRe.ReplaceAllString(m.View(), ""))

	m, _ = m.Update(key("esc"))
	m, _ = m.Update(key("1"))
	t.Log("LAYERS TAB:\n" + ansiRe.ReplaceAllString(m.View(), ""))
}
