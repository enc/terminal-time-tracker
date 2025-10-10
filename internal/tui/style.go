package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Core palette. Call SetTheme to override at runtime (e.g., dark/light, custom colors).
var (
	ColorFg        = lipgloss.Color("#E6E6E6")
	ColorBg        = lipgloss.Color("#1F2430")
	ColorSubtle    = lipgloss.Color("#7A8490")
	ColorAccent    = lipgloss.Color("#7FB4FF")
	ColorGood      = lipgloss.Color("#78D38D")
	ColorWarn      = lipgloss.Color("#FFCC66")
	ColorError     = lipgloss.Color("#FF6C6B")
	ColorBorder    = lipgloss.Color("#3A3F4B")
	ColorSectionBg = lipgloss.Color("#252A36")
	ColorInverseFg = lipgloss.Color("#0C0F14")
	ColorInverseBg = lipgloss.Color("#D8DEE9")
)

// Shared styles (customize via SetTheme if you want to change palette globally).
var (
	HeaderBarStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			Background(ColorBg).
			Bold(true).
			Padding(0, 1)

	HeaderTitleStyle = lipgloss.NewStyle().
				Foreground(ColorFg).
				Bold(true)

	HeaderInfoStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	FooterBarStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			Background(ColorBg).
			Padding(0, 1)

	FooterHintKeyStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	FooterHintTextStyle = lipgloss.NewStyle().
				Foreground(ColorSubtle)

	SectionTitleStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	SectionBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorFg).
			Background(ColorSectionBg).
			Padding(0, 1)

	MutedStyle = lipgloss.NewStyle().Foreground(ColorSubtle)

	EmphStyle = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	ListItemStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(2)

	ListSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorInverseBg).
				Background(ColorAccent).
				Bold(true).
				PaddingLeft(2)

	StatusOKStyle   = lipgloss.NewStyle().Foreground(ColorGood).Bold(true)
	StatusWarnStyle = lipgloss.NewStyle().Foreground(ColorWarn).Bold(true)
	StatusErrStyle  = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
)

// SetTheme lets callers swap the palette in one place (e.g., for dark/light modes).
// Pass empty strings to keep current values.
func SetTheme(colors map[string]string) {
	set := func(dst *lipgloss.Color, key string) {
		if hex, ok := colors[key]; ok && strings.TrimSpace(hex) != "" {
			*dst = lipgloss.Color(hex)
		}
	}

	set(&ColorFg, "fg")
	set(&ColorBg, "bg")
	set(&ColorSubtle, "subtle")
	set(&ColorAccent, "accent")
	set(&ColorGood, "good")
	set(&ColorWarn, "warn")
	set(&ColorError, "error")
	set(&ColorBorder, "border")
	set(&ColorSectionBg, "sectionBg")
	set(&ColorInverseFg, "inverseFg")
	set(&ColorInverseBg, "inverseBg")

	// Rebuild styles to pick up new colors.
	HeaderBarStyle = HeaderBarStyle.Foreground(ColorFg).Background(ColorBg)
	HeaderTitleStyle = HeaderTitleStyle.Foreground(ColorFg)
	HeaderInfoStyle = HeaderInfoStyle.Foreground(ColorSubtle)
	FooterBarStyle = FooterBarStyle.Foreground(ColorFg).Background(ColorBg)
	FooterHintKeyStyle = FooterHintKeyStyle.Foreground(ColorAccent)
	FooterHintTextStyle = FooterHintTextStyle.Foreground(ColorSubtle)
	SectionTitleStyle = SectionTitleStyle.Foreground(ColorAccent)
	SectionBoxStyle = SectionBoxStyle.
		BorderForeground(ColorBorder).
		Foreground(ColorFg).
		Background(ColorSectionBg)
	MutedStyle = MutedStyle.Foreground(ColorSubtle)
	EmphStyle = EmphStyle.Foreground(ColorAccent)
	ListItemStyle = ListItemStyle.Foreground(ColorFg)
	ListSelectedStyle = ListSelectedStyle.Foreground(ColorInverseBg).Background(ColorAccent)
	StatusOKStyle = StatusOKStyle.Foreground(ColorGood)
	StatusWarnStyle = StatusWarnStyle.Foreground(ColorWarn)
	StatusErrStyle = StatusErrStyle.Foreground(ColorError)
}

// RenderHeader renders a left title and a right-aligned info segment within width.
// Background spans the full width.
func RenderHeader(left, right string, width int) string {
	leftR := HeaderTitleStyle.Render(left)
	rightR := HeaderInfoStyle.Render(right)
	fill := max(1, width-lipgloss.Width(leftR)-lipgloss.Width(rightR))
	line := leftR + strings.Repeat(" ", fill) + rightR
	return HeaderBarStyle.Width(width).Render(line)
}

// RenderFooter renders a left-aligned hint segment and a right-aligned status/info.
func RenderFooter(hints []Hint, right string, width int) string {
	leftText := JoinHints(hints, "  ")
	rightR := MutedStyle.Render(right)
	fill := max(1, width-lipgloss.Width(leftText)-lipgloss.Width(rightR))
	line := leftText + strings.Repeat(" ", fill) + rightR
	return FooterBarStyle.Width(width).Render(line)
}

// Hint represents a small keybinding hint for the footer: "[k] Do thing".
type Hint struct {
	Key  string
	Text string
}

func (h Hint) String() string {
	return FooterHintKeyStyle.Render(h.Key) + " " + FooterHintTextStyle.Render(h.Text)
}

func JoinHints(h []Hint, sep string) string {
	if len(h) == 0 {
		return ""
	}
	parts := make([]string, len(h))
	for i := range h {
		parts[i] = h[i].String()
	}
	return strings.Join(parts, sep)
}

// RenderSection wraps content in a bordered box with a styled title.
// If width > 0 it clamps both title and box to width; otherwise it uses natural width.
func RenderSection(title string, content string, width int) string {
	titleR := SectionTitleStyle.Render(title)
	box := SectionBoxStyle
	if width > 0 {
		// Account for border; SectionBoxStyle.Padding(0,1) already adds inner padding.
		box = box.Width(width)
	}
	// Title line above the box
	titleLine := titleR
	if width > 0 && lipgloss.Width(titleLine) > width {
		titleLine = lipgloss.NewStyle().Width(width).Render(titleR)
	}
	return titleLine + "\n" + box.Render(content)
}

// RenderList renders a vertical list with an optional selected index.
// Width clamps the item lines when > 0.
func RenderList(items []string, selected int, width int) string {
	var (
		out    []string
		itemSt = ListItemStyle
		selSt  = ListSelectedStyle
	)
	if width > 0 {
		itemSt = itemSt.Width(width)
		selSt = selSt.Width(width)
	}
	for i, it := range items {
		if i == selected {
			out = append(out, selSt.Render(it))
		} else {
			out = append(out, itemSt.Render(it))
		}
	}
	return strings.Join(out, "\n")
}

// RenderKeyValueList renders k: v pairs with aligned colons.
func RenderKeyValueList(kv [][2]string, width int) string {
	maxKey := 0
	for _, p := range kv {
		if w := lipgloss.Width(p[0]); w > maxKey {
			maxKey = w
		}
	}
	lines := make([]string, 0, len(kv))
	for _, p := range kv {
		key := EmphStyle.Render(p[0])
		// Compute padding to align colons.
		pad := maxKey - lipgloss.Width(p[0])
		line := key + strings.Repeat(" ", pad) + ": " + MutedStyle.Render(p[1])
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(joined)
	}
	return joined
}

// RenderStatus composes a colored status label and a message: e.g., "[OK] Saved".
func RenderStatus(kind string, msg string) string {
	var tag string
	switch strings.ToLower(kind) {
	case "ok", "success", "good":
		tag = StatusOKStyle.Render("[OK]")
	case "warn", "warning":
		tag = StatusWarnStyle.Render("[WARN]")
	case "err", "error", "fail":
		tag = StatusErrStyle.Render("[ERR]")
	default:
		tag = MutedStyle.Render("[INFO]")
	}
	return fmt.Sprintf("%s %s", tag, msg)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
