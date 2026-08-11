package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"strata/internal/engine"
)

// Charm-classic dark theme: purple/pink accents over a purple-tinted dark
// chrome. Dark terminals only.
var (
	cAccent   = lipgloss.Color("#A78BFA") // brand, keys, selection bar
	cOnAccent = lipgloss.Color("#261C4D") // text on the purple brand chip
	cLavender = lipgloss.Color("#C4B5FD") // layer names, active tab
	cPink     = lipgloss.Color("#F472B6") // {{ }} substitution + ⚙ hook markers

	cBright   = lipgloss.Color("#F5F3FF")
	cBody     = lipgloss.Color("#D4D4D8")
	cSoft     = lipgloss.Color("#A0A0AC")
	cMuted    = lipgloss.Color("#7A748C")
	cFaint    = lipgloss.Color("#55506B")
	cDisabled = lipgloss.Color("#3A3A44")
	cCyan     = lipgloss.Color("#56C8D8")

	cBgChrome     = lipgloss.Color("#171522")
	cBgModal      = lipgloss.Color("#151320")
	cBgModalTitle = lipgloss.Color("#1B1828")
	cBgSel        = lipgloss.Color("#2B2350")
	cBgMuted      = lipgloss.Color("#232030")
	cBorder       = lipgloss.Color("#4C3D7A")

	cGreen, cBgGreen   = lipgloss.Color("#7EE0A0"), lipgloss.Color("#16321F")
	cYellow, cBgYellow = lipgloss.Color("#E8C56A"), lipgloss.Color("#33290F")
	cBlue, cBgBlue     = lipgloss.Color("#7EB8F0"), lipgloss.Color("#14263C")
	cRed, cBgRed       = lipgloss.Color("#E88888"), lipgloss.Color("#3A1518")

	rolePalette = []color.Color{
		lipgloss.Color("#CF8FD6"), lipgloss.Color("#D68FA8"),
		lipgloss.Color("#B08FD6"), lipgloss.Color("#D6A08F"),
	}
)

// statusGlyph maps a file status to its glyph text and badge fg/bg colors.
func statusGlyph(st engine.FileStatus) (string, color.Color, color.Color) {
	switch st {
	case engine.Clean:
		return "● clean", cGreen, cBgGreen
	case engine.Create:
		return "✚ create", cGreen, cBgGreen
	case engine.Update:
		return "↑ update", cBlue, cBgBlue
	case engine.Drifted:
		return "~ drifted", cYellow, cBgYellow
	case engine.Conflict:
		return "✖ conflict", cRed, cBgRed
	case engine.Removed:
		return "✕ removed", cRed, cBgRed
	default:
		return "? unmanaged", cMuted, cBgMuted
	}
}

func (s *Snapshot) layerColor(name string) (color.Color, color.Color) {
	switch s.Kind[name] {
	case "base":
		return cSoft, cDisabled
	case "distro":
		return lipgloss.Color("#7FC76A"), lipgloss.Color("#33502D")
	case "role":
		return rolePalette[s.RoleIndex[name]%len(rolePalette)], lipgloss.Color("#5A3A5E")
	}
	switch name {
	case "mac":
		return cCyan, lipgloss.Color("#2E5A63")
	case "linux":
		return lipgloss.Color("#7FC76A"), lipgloss.Color("#33502D")
	case "windows":
		return cBlue, lipgloss.Color("#2D3F5C")
	}
	return cSoft, cDisabled
}

// panel is the rounded purple border used for the files table and modal.
func panel() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cBorder)
}

// brandChip renders the header's purple app-name chip.
func brandChip() string {
	return lipgloss.NewStyle().Background(cAccent).Foreground(cOnAccent).Bold(true).Render(" ✦ strata ")
}

// sectionLabel renders a tinted section-header chip (vars & rules tab).
func sectionLabel(s string) string {
	return lipgloss.NewStyle().Background(cBgSel).Foreground(cLavender).Render(" " + s + " ")
}

// keyHints renders key/description pairs for chrome lines: keys in accent,
// descriptions faint, on the chrome background.
func keyHints(pairs ...[2]string) string {
	var b strings.Builder
	sep := lipgloss.NewStyle().Foreground(cFaint).Background(cBgChrome).Render(" · ")
	for i, p := range pairs {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(cAccent).Background(cBgChrome).Render(p[0]))
		b.WriteString(lipgloss.NewStyle().Foreground(cFaint).Background(cBgChrome).Render(" " + p[1]))
	}
	return b.String()
}

// badgeLabel renders already-padded plain text with the badge substring in
// pink. base carries the row's fg (and bg when the row is selected).
func badgeLabel(plain, badge string, base lipgloss.Style) string {
	if badge == "" {
		return base.Render(plain)
	}
	i := strings.LastIndex(plain, badge)
	if i < 0 { // badge lost to truncation
		return base.Render(plain)
	}
	return base.Render(plain[:i]) +
		base.Foreground(cPink).Render(badge) +
		base.Render(plain[i+len(badge):])
}
