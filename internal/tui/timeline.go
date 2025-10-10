package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// dayStat is a small shared struct used by the week timeline renderer.
// Promoted to package-level so helper functions and other render paths
// can reference it (previously it lived inside RenderWeekTimeline).
type dayStat struct {
	secs int // total seconds that day
	cnt  int // number of entries touching that day
	ents []Entry
}

// RenderWeekTimeline renders a week view grouped by customer.
//
// - `entries` are the set of journal entries (may span days).
// - `weekStart` is the start of the 7-day window (should be at 00:00 of day in tz).
// - `tz` optional timezone; if nil time.Local is used.
// - `width` total available width for rendering (including customer column).
//
// Output is a multi-line string suitable for inclusion in the dashboard body.
// The view groups rows by customer and draws a small horizontal timeline for
// each day (7 columns). Colors:
//   - running entries (End == nil): Accent
//   - billable: Good
//   - non-billable: Warn
//
// The renderer also shows per-day totals (hours and count) on the right of each
// row and a small legend at the bottom.
func RenderWeekTimeline(entries []Entry, weekStart time.Time, tz *time.Location, width int) string {
	if tz == nil {
		tz = time.Local
	}

	// Normalize weekStart to TZ midnight.
	weekStart = time.Date(weekStart.In(tz).Year(), weekStart.In(tz).Month(), weekStart.In(tz).Day(), 0, 0, 0, 0, tz)

	// Group entries by customer, and bucket per day.
	customers := map[string][7]dayStat{}
	customerSet := map[string]struct{}{}

	// Helper to add an entry into appropriate day buckets (may split across days).
	for _, e := range entries {
		// Convert entry times to timezone for consistent day boundaries.
		start := e.Start.In(tz)
		var end time.Time
		if e.End != nil {
			end = e.End.In(tz)
		} else {
			// running entries use now as end for totals but still marked as running
			end = time.Now().In(tz)
		}

		// Skip entries that end before weekStart or start after weekEnd.
		weekEnd := weekStart.AddDate(0, 0, 7)
		if !start.Before(weekEnd) || !end.After(weekStart) {
			continue
		}

		// Clip to the week window.
		if start.Before(weekStart) {
			start = weekStart
		}
		if end.After(weekEnd) {
			end = weekEnd
		}

		// For each day the entry intersects, add intersection duration.
		for d := 0; d < 7; d++ {
			dayStart := weekStart.AddDate(0, 0, d)
			dayEnd := dayStart.AddDate(0, 0, 1)

			segStart := maxTime(start, dayStart)
			segEnd := minTime(end, dayEnd)
			if !segStart.Before(segEnd) {
				continue
			}
			secs := int(segEnd.Sub(segStart).Seconds())
			cust := e.Customer
			if cust == "" {
				cust = "-"
			}
			customerSet[cust] = struct{}{}
			arr := customers[cust]
			ds := arr[d]
			ds.secs += secs
			ds.cnt++
			ds.ents = append(ds.ents, e)
			arr[d] = ds
			customers[cust] = arr
		}
	}

	// Build sorted list of customers to have stable rendering.
	custList := make([]string, 0, len(customerSet))
	for c := range customerSet {
		custList = append(custList, c)
	}
	sort.Strings(custList)

	// Layout calculation
	minLeft := 12
	maxLeft := 28
	leftW := min(maxLeft, max(minLeft, longestLen(append(custList, "Customer"))+2))
	// Reserve spacing for day columns and per-day summary area.
	remaining := width - leftW - 2
	if remaining < 14 {
		// small terminal: fall back to compact text list
		return renderCompactWeek(customers, custList, weekStart, tz, width)
	}
	dayW := remaining / 7
	if dayW < 6 {
		dayW = 6
	}

	// Header line: customer label + seven day headings
	var b strings.Builder
	// Title row
	titleLeft := EmphStyle.Render("Customer")
	b.WriteString(padRight(titleLeft, leftW))
	for d := 0; d < 7; d++ {
		day := weekStart.AddDate(0, 0, d)
		dayLabel := day.Format("Mon 02")
		// center the label within dayW
		b.WriteString(centerText(dayLabel, dayW))
	}
	b.WriteString("\n")

	// For each customer build a row
	for _, cust := range custList {
		// Customer column
		custName := ListItemStyle.Render(cust)
		b.WriteString(padRight(custName, leftW))

		// For each day render a mini-timeline
		for d := 0; d < 7; d++ {
			ds := customers[cust][d]
			// Build base cell of spaces to color
			cell := make([]rune, dayW)
			for i := range cell {
				cell[i] = ' '
			}

			// For each entry affecting that day, overlay colored segments
			// We render in order so later segments overwrite earlier ones.
			dayStart := weekStart.AddDate(0, 0, d)
			dayEnd := dayStart.AddDate(0, 0, 1)
			// sort entries by start to make rendering predictable
			sort.SliceStable(ds.ents, func(i, j int) bool {
				return ds.ents[i].Start.Before(ds.ents[j].Start)
			})

			for _, e := range ds.ents {
				est := maxTime(e.Start.In(tz), dayStart)
				eet := dayEnd
				if e.End != nil {
					eet = minTime(e.End.In(tz), dayEnd)
				} else {
					// running entry clipped to now or dayEnd
					now := time.Now().In(tz)
					if now.Before(dayEnd) {
						eet = minTime(now, dayEnd)
					}
				}
				// Map to columns
				relStart := est.Sub(dayStart).Seconds()
				relEnd := eet.Sub(dayStart).Seconds()
				startCol := int((relStart / 86400.0) * float64(dayW))
				endCol := int((relEnd / 86400.0) * float64(dayW))
				if startCol < 0 {
					startCol = 0
				}
				if endCol > dayW {
					endCol = dayW
				}
				if endCol <= startCol {
					endCol = min(startCol+1, dayW)
				}
				// Choose color
				var bg lipgloss.Color
				if e.End == nil {
					bg = ColorAccent
				} else if e.Billable {
					bg = ColorGood
				} else {
					bg = ColorWarn
				}
				// Fill
				for i := startCol; i < endCol; i++ {
					cell[i] = '█' // solid block for better visibility
				}
				// Convert runes to string then style segment
				seg := string(cell)
				st := lipgloss.NewStyle().Background(bg).Foreground(ColorInverseFg).Render(seg)
				// Because we rendered into the same cell array, we will render once after coloring decisions.
				// To keep it simple, we will use a single background per cell below (see end of loop).
				_ = st // no-op here; actual rendering uses per-character style per color below
			}

			// Instead of per-entry style complexity, build colored spans by scanning entries again,
			// but with simpler approach: create a slice of bg colors per column, default to SectionBg.
			bgCols := make([]lipgloss.Color, dayW)
			for i := range bgCols {
				bgCols[i] = ColorSectionBg
			}
			for _, e := range ds.ents {
				est := maxTime(e.Start.In(tz), dayStart)
				eet := dayEnd
				if e.End != nil {
					eet = minTime(e.End.In(tz), dayEnd)
				} else {
					now := time.Now().In(tz)
					if now.Before(dayEnd) {
						eet = minTime(now, dayEnd)
					}
				}
				relStart := est.Sub(dayStart).Seconds()
				relEnd := eet.Sub(dayStart).Seconds()
				startCol := int((relStart / 86400.0) * float64(dayW))
				endCol := int((relEnd / 86400.0) * float64(dayW))
				if startCol < 0 {
					startCol = 0
				}
				if endCol > dayW {
					endCol = dayW
				}
				if endCol <= startCol {
					endCol = min(startCol+1, dayW)
				}
				var bg lipgloss.Color
				if e.End == nil {
					bg = ColorAccent
				} else if e.Billable {
					bg = ColorGood
				} else {
					bg = ColorWarn
				}
				for i := startCol; i < endCol; i++ {
					bgCols[i] = bg
				}
			}

			// Now build the cell rendering by grouping consecutive same-bg runs for efficient styling.
			var cellBuilder strings.Builder
			i := 0
			for i < dayW {
				j := i + 1
				for j < dayW && bgCols[j] == bgCols[i] {
					j++
				}
				spanLen := j - i
				span := strings.Repeat(" ", spanLen)
				st := lipgloss.NewStyle().Background(bgCols[i]).Render(span)
				cellBuilder.WriteString(st)
				i = j
			}
			// Append the day cell
			b.WriteString(cellBuilder.String())
		}

		// After 7 day columns, append per-day totals summary (simple aggregation right of row)
		// Format: "  Mon: 4h30m (2) Tue: 2h (1) ..." or condensed totals.
		// We'll show totals for the week aggregated.
		weekSecs := 0
		weekCnt := 0
		for d := 0; d < 7; d++ {
			ds := customers[cust][d]
			weekSecs += ds.secs
			weekCnt += ds.cnt
		}
		summary := "  " + fmtDurationShort(weekSecs)
		if weekCnt > 0 {
			summary += fmt.Sprintf(" (%d)", weekCnt)
		} else {
			summary += " (-)"
		}
		b.WriteString(" " + MutedStyle.Render(summary))
		b.WriteString("\n")
	}

	// Legend
	b.WriteString("\n")
	legend := buildLegend()
	b.WriteString(legend)

	return RenderSection("Week timelines", b.String(), width)
}

// ---------- Helpers ----------

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func longestLen(ss []string) int {
	max := 0
	for _, s := range ss {
		l := lipgloss.Width(s)
		if l > max {
			max = l
		}
	}
	return max
}

func padRight(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return lipgloss.NewStyle().Width(w).Render(s)
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func centerText(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return lipgloss.NewStyle().Width(w).Render(s)
	}
	padding := w - lipgloss.Width(s)
	left := padding / 2
	right := padding - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func fmtDurationShort(sec int) string {
	if sec <= 0 {
		return "0h0m"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func buildLegend() string {
	var b strings.Builder
	legendItems := []struct {
		color lipgloss.Color
		label string
	}{
		{ColorAccent, "running"},
		{ColorGood, "billable"},
		{ColorWarn, "non-billable"},
	}
	for _, it := range legendItems {
		sample := lipgloss.NewStyle().Background(it.color).Render("  ")
		b.WriteString(sample + " " + MutedStyle.Render(it.label) + "  ")
	}
	return b.String()
}

func renderCompactWeek(customers map[string][7]dayStat, custList []string, weekStart time.Time, tz *time.Location, width int) string {
	var b strings.Builder
	for _, cust := range custList {
		// header
		line := EmphStyle.Render(cust)
		b.WriteString(line + "\n")
		// per day short stats
		for d := 0; d < 7; d++ {
			ds := customers[cust][d]
			day := weekStart.AddDate(0, 0, d)
			dayLabel := day.Format("Mon")
			s := fmt.Sprintf("  %s: %s (%d)\n", dayLabel, fmtDurationShort(ds.secs), ds.cnt)
			b.WriteString(MutedStyle.Render(s))
		}
		b.WriteString("\n")
	}
	return b.String()
}
