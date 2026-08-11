package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pmezard/go-difflib/difflib"

	"strata/internal/engine"
)

// Colors and shared styles live in theme.go.

// ── text helpers ─────────────────────────────────────────────────────

func trunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if w <= 1 {
		return "…"
	}
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func padr(s string, w int) string {
	s = trunc(s, w)
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func padl(s string, w int) string {
	s = trunc(s, w)
	return strings.Repeat(" ", w-lipgloss.Width(s)) + s
}

// fillHeight pads or clips a block to exactly h lines.
func fillHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func chromeLine(w int, left, right string) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return lipgloss.NewStyle().Background(cBgChrome).Render(left + strings.Repeat(" ", gap) + right)
}

// ── top-level view ───────────────────────────────────────────────────

func (m Model) View() tea.View {
	bodyH := m.h - 3
	if bodyH < 4 {
		bodyH = 4
	}
	var body string
	switch m.tab {
	case 0:
		body = m.layersView()
	case 1:
		body = m.filesView(bodyH)
	case 2:
		body = m.varsView()
	}
	if m.open {
		body = m.overlayView(bodyH)
	}
	v := tea.NewView(m.headerView() + "\n" + m.tabsView() + "\n" + fillHeight(body, bodyH) + "\n" + m.footerView())
	v.AltScreen = true
	return v
}

func (m Model) headerView() string {
	st := func(c color.Color, b bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Background(cBgChrome).Bold(b)
	}
	left := " " + brandChip() + " " + st(cMuted, false).Render(m.snap.RepoPath)
	right := st(cMuted, false).Render("machine: ") + st(cBright, false).Render(m.snap.MachineName) +
		st(cMuted, false).Render(" · layers: ") + st(cLavender, false).Render(m.snap.ActiveText) + " "
	return chromeLine(m.w, left, right)
}

func (m Model) tabsView() string {
	labels := []string{"Layers", "Files", "Vars & Rules"}
	var parts []string
	for i, l := range labels {
		if i == m.tab {
			parts = append(parts, lipgloss.NewStyle().Background(cBgSel).Foreground(cLavender).Bold(true).Render(" "+l+" "))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(cMuted).Render(" "+l+" "))
		}
	}
	left := " " + strings.Join(parts, " ")
	right := lipgloss.NewStyle().Foreground(cFaint).Render("read-only · later layer wins whole file") + " "
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) footerView() string {
	hints := keyHints(
		[2]string{"←→ 1-3", "tabs"}, [2]string{"↑↓", "move"},
		[2]string{"enter", "detail"}, [2]string{"esc", "close"}, [2]string{"q", "quit"})
	return chromeLine(m.w, " "+hints, "")
}

// ── Layers tab ───────────────────────────────────────────────────────

func (m Model) layersView() string {
	s := m.snap
	n := len(s.Layers)
	if n == 0 {
		return lipgloss.NewStyle().Foreground(cMuted).Render(" no layer folders found in the repo")
	}
	colW := (m.w - 2) / n
	if colW < 14 {
		colW = 14
	}
	var blocks []string
	for _, ly := range s.Layers {
		fg, border := s.layerColor(ly.Name)
		title := ly.Name
		if ly.Active {
			title += " " + lipgloss.NewStyle().Foreground(cAccent).Render("✓")
		}
		head := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(border).
			Foreground(fg).Align(lipgloss.Center).Width(colW - 3).Render(title)
		var lines []string
		for _, f := range ly.Files {
			label := f.Short
			if f.Badge != "" {
				label += " " + f.Badge
			}
			if f.OverriddenBy != "" {
				lines = append(lines,
					lipgloss.NewStyle().Foreground(cFaint).Strikethrough(true).Render(trunc(f.Short, colW-2)),
					lipgloss.NewStyle().Foreground(cFaint).Render("  ↷ "+f.OverriddenBy))
			} else {
				lines = append(lines, badgeLabel(trunc(label, colW-2), f.Badge, lipgloss.NewStyle().Foreground(cBody)))
			}
		}
		block := head + "\n" + strings.Join(lines, "\n")
		style := lipgloss.NewStyle().Width(colW).PaddingLeft(1)
		if !ly.Active {
			style = style.Faint(true)
		}
		blocks = append(blocks, style.Render(block))
	}
	grid := lipgloss.JoinHorizontal(lipgloss.Top, blocks...)

	legend := " " + lipgloss.NewStyle().Foreground(cMuted).Render(
		"↷ overridden by later layer · ▲ overrides earlier layer · dimmed = inactive on this machine")

	var sum []string
	if len(s.Vars) > 0 {
		v := s.Vars[0]
		sum = append(sum, "VARS "+v.Name+" = "+v.Value+" ("+v.From+")")
	}
	if len(s.Hooks) > 0 {
		sum = append(sum, "HOOKS "+s.Hooks[0][0]+" → "+trunc(s.Hooks[0][1], 30))
	}
	if len(s.Perms) > 0 {
		var pp []string
		for _, p := range s.Perms {
			pp = append(pp, p[0]+" = "+p[1])
		}
		sum = append(sum, "PERMS "+strings.Join(pp, " · "))
	}
	summary := chromeLine(m.w, " "+lipgloss.NewStyle().Foreground(cMuted).Background(cBgChrome).Render(trunc(strings.Join(sum, "  ·  "), m.w-2)), "")

	return grid + "\n" + legend + "\n" + summary
}

// ── Files tab ────────────────────────────────────────────────────────

func (m Model) filesView(bodyH int) string {
	s := m.snap
	wide := m.w >= 92
	statusW, winsW, osW := 13, 11, 9
	inner := m.w - 4 // rounded border + one padding column per side
	fileW := inner - 1 - winsW - statusW
	if wide {
		fileW -= 3 * osW
	}
	if fileW < 18 {
		fileW = 18
	}

	head := " " + padr("FILE", fileW) + padr("WINS HERE", winsW)
	if wide {
		head += padr("MAC", osW) + padr("LINUX", osW) + padr("WIN", osW)
	}
	head += padl("STATUS", statusW)
	rows := []string{lipgloss.NewStyle().Foreground(cMuted).Render(head)}

	rowsArea := bodyH - 4 // panel border (2) + column header + legend
	if rowsArea < 1 {
		rowsArea = 1
	}
	off := 0
	if len(s.Rows) > rowsArea && m.sel >= rowsArea {
		off = m.sel - rowsArea + 1
	}
	end := off + rowsArea
	if end > len(s.Rows) {
		end = len(s.Rows)
	}

	for i := off; i < end; i++ {
		r := s.Rows[i]
		selRow := i == m.sel
		rowSt := func(fg color.Color) lipgloss.Style {
			st := lipgloss.NewStyle().Foreground(fg)
			if selRow {
				st = st.Background(cBgSel)
			}
			return st
		}
		cell := func(text string, fg color.Color) string { return rowSt(fg).Render(text) }

		bar := " "
		if selRow {
			bar = rowSt(cAccent).Render("▎")
		}
		fileFg := cBright
		if selRow {
			fileFg = lipgloss.Color("#FFFFFF")
		}
		name := r.Rel
		if r.Badge != "" {
			name += " " + r.Badge
		}
		line := bar + badgeLabel(padr(name, fileW), r.Badge, rowSt(fileFg))

		if r.Winner != "" {
			wc, _ := s.layerColor(r.Winner)
			line += cell(padr(r.Winner, winsW), wc)
		} else {
			line += cell(padr("n/a", winsW), cDisabled)
		}
		if wide {
			for _, osw := range []string{r.Mac, r.Linux, r.Win} {
				if osw == "" {
					line += cell(padr("—", osW), cDisabled)
				} else {
					line += cell(padr(osw, osW), cFaint)
				}
			}
		}
		if r.Resolved {
			g, fg, bg := statusGlyph(r.Status)
			b := lipgloss.NewStyle().Foreground(fg).Background(bg).Render(" " + g + " ")
			pad := statusW - lipgloss.Width(g) - 2
			if pad < 0 {
				pad = 0
			}
			line += cell(strings.Repeat(" ", pad), cBody) + b
		} else {
			line += cell(padl("—", statusW), cDisabled)
		}
		rows = append(rows, line)
	}

	for len(rows) < rowsArea+1 {
		rows = append(rows, "")
	}
	table := panel().Padding(0, 1).Render(strings.Join(rows, "\n"))

	legend := chromeLine(m.w,
		" "+lipgloss.NewStyle().Background(cBgChrome).Foreground(cMuted).Render(
			"● clean  ~ drifted  ↑ update  ✖ conflict  ✚ create  ? unmanaged"),
		lipgloss.NewStyle().Background(cBgChrome).Foreground(cFaint).Render("{{ }} substituted · ⚙ hook · 600 perms")+" ")
	return table + "\n" + legend
}

// ── Vars & Rules tab ─────────────────────────────────────────────────

func (m Model) varsView() string {
	s := m.snap
	nameW, valW, fromW := 16, 28, 14
	head := " " + padr("VAR", nameW) + padr("VALUE HERE", valW) + padr("FROM", fromW) + "DEFAULT (dots.toml)"
	out := []string{lipgloss.NewStyle().Foreground(cMuted).Render(head)}

	for _, v := range s.Vars {
		fromFg := cSoft
		if v.From == "machine.toml" {
			fromFg = rolePalette[0]
		}
		defSt := lipgloss.NewStyle().Foreground(cFaint)
		if v.Overridden {
			defSt = defSt.Strikethrough(true)
		}
		out = append(out, " "+
			lipgloss.NewStyle().Foreground(cBright).Render(padr(v.Name, nameW))+
			lipgloss.NewStyle().Foreground(cBright).Render(padr(v.Value, valW))+
			lipgloss.NewStyle().Foreground(fromFg).Render(padr(v.From, fromW))+
			defSt.Render(v.Default))
	}
	if len(s.Vars) == 0 {
		out = append(out, lipgloss.NewStyle().Foreground(cFaint).Render(" no variables defined"))
	}

	out = append(out, "", " "+sectionLabel("USED BY")+
		lipgloss.NewStyle().Foreground(cFaint).Render(" files listed in dots.toml [substitute]"))
	if len(s.UsedBy) == 0 {
		out = append(out, lipgloss.NewStyle().Foreground(cFaint).Render(" none"))
	} else {
		var parts []string
		for _, u := range s.UsedBy {
			parts = append(parts, u[0]+" "+u[1])
		}
		out = append(out, " "+lipgloss.NewStyle().Foreground(cBody).Render(strings.Join(parts, " · ")))
	}

	out = append(out, "", " "+sectionLabel("HOOKS")+
		lipgloss.NewStyle().Foreground(cFaint).Render(" run after apply, only if that file changed"))
	if len(s.Hooks) == 0 {
		out = append(out, lipgloss.NewStyle().Foreground(cFaint).Render(" none"))
	}
	for _, h := range s.Hooks {
		out = append(out, " "+lipgloss.NewStyle().Foreground(cBody).Render(h[0]+" → "+h[1]))
	}

	out = append(out, "", " "+sectionLabel("PERMS")+
		lipgloss.NewStyle().Foreground(cFaint).Render(" glob → mode, longest match wins"))
	var pp []string
	for _, p := range s.Perms {
		pp = append(pp, p[0]+" = "+p[1])
	}
	pp = append(pp, "default 644")
	out = append(out, " "+lipgloss.NewStyle().Foreground(cBody).Render(strings.Join(pp, " · ")))

	return strings.Join(out, "\n")
}

// ── Drilldown overlay ────────────────────────────────────────────────

func (m Model) currentRow() Row {
	sel := m.sel
	if sel >= len(m.snap.Rows) {
		sel = len(m.snap.Rows) - 1
	}
	return m.snap.Rows[sel]
}

func diffLines(r Row) []string {
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(r.Item.Current)),
		B:        difflib.SplitLines(string(r.Item.Desired)),
		FromFile: "home/" + r.Rel,
		ToFile:   "repo/" + r.Rel,
		Context:  3,
	})
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

func colorDiffLine(l string) string {
	switch {
	case strings.HasPrefix(l, "+"):
		return lipgloss.NewStyle().Foreground(cGreen).Render(l)
	case strings.HasPrefix(l, "-"):
		return lipgloss.NewStyle().Foreground(cRed).Render(l)
	case strings.HasPrefix(l, "@"):
		return lipgloss.NewStyle().Foreground(cMuted).Render(l)
	}
	return lipgloss.NewStyle().Foreground(cBody).Render(l)
}

func driftLabel(st engine.FileStatus) string {
	switch st {
	case engine.Drifted:
		return "LOCAL DRIFT (repo → $HOME)"
	case engine.Update:
		return "PENDING UPDATE (repo → $HOME)"
	case engine.Conflict:
		return "CONFLICT — repo and $HOME both changed"
	case engine.Unmanaged:
		return "UNMANAGED — existing file differs from repo"
	}
	return ""
}

func (m Model) overlayView(bodyH int) string {
	if m.diff {
		return m.diffView(bodyH)
	}
	s := m.snap
	r := m.currentRow()
	mw := 76
	if mw > m.w-6 {
		mw = m.w - 6
	}
	inner := mw - 4

	muted := lipgloss.NewStyle().Foreground(cMuted)
	faint := lipgloss.NewStyle().Foreground(cFaint)
	body := lipgloss.NewStyle().Foreground(cBody)

	var b []string
	srcLayer := r.Winner
	if srcLayer == "" && len(r.Providers) > 0 {
		srcLayer = r.Providers[0]
	}
	b = append(b, muted.Render("SOURCE → DESTINATION"))
	b = append(b, body.Render(trunc(s.RepoPath+"/"+srcLayer+"/"+r.Rel+" → ~/"+r.Rel, inner)))
	b = append(b, faint.Render(trunc(m.noteFor(r), inner)), "")

	dash := func(v string) string {
		if v == "" {
			return "—"
		}
		return v
	}
	b = append(b, muted.Render("RESOLVES ON"))
	b = append(b, body.Render("mac → "+dash(r.Mac)+" · linux → "+dash(r.Linux)+" · windows → "+dash(r.Win)), "")

	if len(r.SubstVars) > 0 {
		b = append(b, muted.Render("SUBSTITUTED VARS"))
		for _, v := range r.SubstVars {
			b = append(b, body.Render("{{"+v.Name+"}} = "+v.Value+" ")+faint.Render("("+v.From+")"))
		}
		b = append(b, "")
	}

	last := "—"
	if r.LastHash != "" {
		last = "hash " + r.LastHash
	}
	hook := r.Hook
	if hook == "" {
		hook = "none"
	}
	b = append(b, muted.Render(padr("PERMS", 18)+padr("HOOK", 28)+"LAST APPLIED"))
	b = append(b, body.Render(padr(r.Perm, 18)+padr(trunc(hook, 26), 28)+last))

	if r.Resolved && r.Status != engine.Clean && r.Status != engine.Create {
		b = append(b, "", faint.Render(driftLabel(r.Status)))
		dl := diffLines(r)
		if len(dl) > 2 {
			dl = dl[2:] // drop ---/+++ headers in the excerpt
		}
		if len(dl) > 6 {
			dl = append(dl[:6], faint.Render("… (d for full diff)"))
		}
		for _, l := range dl {
			b = append(b, colorDiffLine(trunc(l, inner)))
		}
	}

	title := chromeTitle(mw, r)
	content := lipgloss.NewStyle().Padding(0, 2).Width(mw).Render(strings.Join(b, "\n"))
	foot := lipgloss.NewStyle().Padding(0, 2).Render(
		lipgloss.NewStyle().Foreground(cAccent).Render("esc") +
			lipgloss.NewStyle().Foreground(cFaint).Render(" close · ") +
			lipgloss.NewStyle().Foreground(cAccent).Render("d") +
			lipgloss.NewStyle().Foreground(cFaint).Render(" full diff"))
	modal := panel().Background(cBgModal).
		Render(title + "\n" + content + "\n" + foot)
	return lipgloss.Place(m.w, bodyH, lipgloss.Center, lipgloss.Center, modal)
}

func chromeTitle(mw int, r Row) string {
	name := lipgloss.NewStyle().Foreground(cBright).Bold(true).Background(cBgModalTitle).Render(r.Rel)
	var st string
	if r.Resolved {
		g, c, _ := statusGlyph(r.Status)
		st = lipgloss.NewStyle().Foreground(c).Background(cBgModalTitle).Render(g)
	} else {
		st = lipgloss.NewStyle().Foreground(cMuted).Background(cBgModalTitle).Render("not applied on this machine")
	}
	gap := mw - lipgloss.Width(name) - lipgloss.Width(st) - 4
	if gap < 1 {
		gap = 1
	}
	pad := lipgloss.NewStyle().Background(cBgModalTitle).Render(strings.Repeat(" ", gap))
	side := lipgloss.NewStyle().Background(cBgModalTitle).Render("  ")
	return side + name + pad + st + side
}

// noteFor mirrors the design's provenance note under SOURCE → DESTINATION.
func (m Model) noteFor(r Row) string {
	if !r.Resolved {
		return "provided by " + strings.Join(r.Providers, ", ") + " — layer inactive here"
	}
	var earlier, later []string
	seenWinner := false
	for _, p := range r.Providers {
		if p == r.Winner {
			seenWinner = true
			continue
		}
		if seenWinner {
			later = append(later, p+" (inactive here)")
		} else {
			earlier = append(earlier, p+"/"+r.Rel)
		}
	}
	var parts []string
	if len(earlier) > 0 {
		parts = append(parts, "overrides "+strings.Join(earlier, ", "))
	}
	if len(later) > 0 {
		parts = append(parts, "also provided by "+strings.Join(later, ", "))
	}
	if len(parts) == 0 {
		return "only layer providing this file"
	}
	return strings.Join(parts, " · ")
}

func (m Model) diffView(bodyH int) string {
	r := m.currentRow()
	lines := diffLines(r)
	area := bodyH - 2
	if area < 1 {
		area = 1
	}
	maxOff := len(lines) - area
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.diffOff
	if off > maxOff {
		off = maxOff
	}
	end := off + area
	if end > len(lines) {
		end = len(lines)
	}
	out := []string{lipgloss.NewStyle().Foreground(cMuted).Render(
		fmt.Sprintf(" diff — %s · ↑↓ scroll · esc back (%d/%d)", r.Rel, end, len(lines)))}
	for _, l := range lines[off:end] {
		out = append(out, " "+colorDiffLine(trunc(l, m.w-2)))
	}
	return strings.Join(out, "\n")
}
