package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rwWeekFlag       string
	rwFromFlag       string
	rwToFlag         string
	rwFormatFlag     string
	rwRoundFlag      int
	rwCustomerFilter string
	rwTagFilters     []string
	rwIncludeOpen    bool
	rwNotesWrap      int
	rwLocale         string
	rwExportTempo    string
	rwTempoRounded   bool
)

// Types used across functions (moved to package-level to avoid visibility issues)
type outNoteGroup struct {
	Customer    string   `json:"customer"`
	Project     string   `json:"project,omitempty"`
	Seconds     int64    `json:"seconds"`
	SecRounded  int64    `json:"secondsRounded"`
	Notes       []string `json:"notes"`
	NotesMerged string   `json:"notesMerged"`
}

type outDay struct {
	Date              string         `json:"date"`
	Weekday           string         `json:"weekday"`
	Groups            []outNoteGroup `json:"groups"`
	DaySeconds        int64          `json:"daySeconds"`
	DaySecondsRounded int64          `json:"daySecondsRounded"`
	Flags             []string       `json:"flags"`
}

var reportWeekCmd = &cobra.Command{
	Use:   "week",
	Short: "Report this ISO week (Mon–Sun) grouped by day and customer/project",
	Run: func(cmd *cobra.Command, args []string) {
		// Determine timezone for grouping (target is Europe/Berlin by default in spec)
		tzName := viper.GetString("timezone")
		if tzName == "" {
			tzName = "Europe/Berlin"
		}
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			fmt.Printf("Warning: failed to load timezone %q, using Local\n", tzName)
			loc = time.Local
		}

		// Resolve range: --from & --to override --week. If none given, current ISO week.
		var from, to time.Time
		if rwFromFlag != "" && rwToFlag != "" {
			from = mustParseTimeLocal(rwFromFlag).In(loc)
			to = mustParseTimeLocal(rwToFlag).In(loc)
		} else {
			// parse week or default to current ISO week
			year, week := time.Now().In(loc).ISOWeek()
			if rwWeekFlag != "" {
				if y, w, perr := parseISOWeek(rwWeekFlag); perr == nil {
					year, week = y, w
				} else {
					cobra.CheckErr(fmt.Errorf("invalid --week: %v", perr))
				}
			}
			start, end := isoWeekRange(year, week, loc)
			from = start
			to = end
		}

		// Normalize from/to to date boundaries in target loc
		from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
		to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, loc)

		// Load entries that intersect the window. The existing loadEntries expects times in local zone;
		// ensure we pass times in the same location as viper timezone to get proper files.
		entries, err := loadEntries(from, to)
		if err != nil {
			fmt.Printf("Warning: failed to load some entries: %v\n", err)
		}

		// Apply basic filters: customer (case-insensitive exact) and tags (AND)
		filtered := make([]Entry, 0, len(entries))
		for _, e := range entries {
			if rwCustomerFilter != "" {
				if !strings.EqualFold(strings.TrimSpace(e.Customer), strings.TrimSpace(rwCustomerFilter)) {
					continue
				}
			}
			if len(rwTagFilters) > 0 {
				etags := make([]string, 0, len(e.Tags))
				for _, t := range e.Tags {
					etags = append(etags, strings.ToLower(strings.TrimSpace(t)))
				}
				ok := true
				for _, rt := range rwTagFilters {
					if !containsString(etags, strings.ToLower(strings.TrimSpace(rt))) {
						ok = false
						break
					}
				}
				if !ok {
					continue
				}
			}
			filtered = append(filtered, e)
		}

		// If no entries, show simple message
		if len(filtered) == 0 {
			fmt.Println("No entries in range.")
			return
		}

		// Prepare split-per-day segments in target timezone.
		type seg struct {
			Day      string // YYYY-MM-DD in loc
			Start    time.Time
			End      time.Time
			Seconds  int64
			EntryID  string
			Customer string
			Project  string
			Notes    []string
			Tags     []string
		}

		var segments []seg
		var badEntries []string // zero/negative durations or missing customer
		for _, e := range filtered {
			// Determine end time: if nil and include-open, set to now UTC
			var end time.Time
			if e.End == nil {
				if rwIncludeOpen {
					end = time.Now().UTC()
				} else {
					// skip running entries unless include-open
					badEntries = append(badEntries, fmt.Sprintf("%s (running)", e.ID))
					continue
				}
			} else {
				end = *e.End
			}
			start := e.Start

			if !end.After(start) {
				badEntries = append(badEntries, fmt.Sprintf("%s (zero/negative)", e.ID))
				continue
			}

			// Convert to local tz for splitting/grouping
			startLoc := start.In(loc)
			endLoc := end.In(loc)

			// iterate day boundaries from startLoc to endLoc
			curStart := startLoc
			for curStart.Before(endLoc) {
				// compute midnight of next day
				y, m, d := curStart.Date()
				nextMidnight := time.Date(y, m, d, 0, 0, 0, 0, loc).Add(24 * time.Hour)
				segEnd := endLoc
				if nextMidnight.Before(endLoc) {
					segEnd = nextMidnight
				}
				seconds := int64(segEnd.Sub(curStart).Seconds())
				dayKey := curStart.Format("2006-01-02")
				cust := e.Customer
				if strings.TrimSpace(cust) == "" {
					cust = "(unknown)"
				}
				segments = append(segments, seg{
					Day:      dayKey,
					Start:    curStart,
					End:      segEnd,
					Seconds:  seconds,
					EntryID:  e.ID,
					Customer: cust,
					Project:  e.Project,
					Notes:    e.Notes,
					Tags:     e.Tags,
				})
				curStart = segEnd
			}
		}

		// Compute quantum early for per-entry rounding (rwRoundFlag is divisions-per-hour).
		if rwRoundFlag <= 0 {
			// default changed to 4 -> 15 minute quantum (per new requirement)
			rwRoundFlag = 4
		}
		quantumMinLocal := 60 / rwRoundFlag
		if quantumMinLocal <= 0 {
			quantumMinLocal = 15
		}
		quantumSecLocal := int64(quantumMinLocal * 60)

		// Distribute per-entry rounding: round up each entry's total seconds to the quantum,
		// then allocate the rounded total across its segments proportionally (floor allocations,
		// remainder goes to the last segment). This preserves per-entry round-up semantics while
		// ensuring per-day and per-group sums match per-entry rounded totals.
		// Build map entryID -> slice of indices into segments.
		entryIndices := map[string][]int{}
		for i := range segments {
			entryIndices[segments[i].EntryID] = append(entryIndices[segments[i].EntryID], i)
		}
		for _, idxs := range entryIndices {
			// compute total raw seconds for this entry
			var total int64 = 0
			for _, idx := range idxs {
				total += segments[idx].Seconds
			}
			if total <= 0 {
				continue
			}
			rounded := roundUpSecondsToQuantum(total, quantumSecLocal)
			if rounded == total {
				continue
			}
			// allocate rounded seconds across segments
			var sumAllocated int64 = 0
			for j, idx := range idxs {
				if j == len(idxs)-1 {
					// last segment gets the remainder to ensure totals match
					segments[idx].Seconds = rounded - sumAllocated
				} else {
					alloc := (segments[idx].Seconds * rounded) / total
					segments[idx].Seconds = alloc
					sumAllocated += alloc
				}
			}
		}

		// Detect overlaps per day
		overlapsByDay := map[string]map[string]struct{}{} // day -> set of entryIDs involved
		overlapRanges := []string{}
		// group segments per day for overlap detection
		segByDay := map[string][]seg{}
		for _, s := range segments {
			segByDay[s.Day] = append(segByDay[s.Day], s)
		}
		for day, segs := range segByDay {
			sort.Slice(segs, func(i, j int) bool { return segs[i].Start.Before(segs[j].Start) })
			for i := 1; i < len(segs); i++ {
				prev := segs[i-1]
				cur := segs[i]
				if cur.Start.Before(prev.End) {
					if _, ok := overlapsByDay[day]; !ok {
						overlapsByDay[day] = map[string]struct{}{}
					}
					overlapsByDay[day][prev.EntryID] = struct{}{}
					overlapsByDay[day][cur.EntryID] = struct{}{}
					overlapRanges = append(overlapRanges, fmt.Sprintf("%s entry ids %s, %s %s–%s",
						day, prev.EntryID, cur.EntryID, prev.Start.Format("15:04"), cur.End.Format("15:04")))
				}
			}
		}

		// Aggregate per (day, customer, project)
		type groupKey struct {
			Day      string
			Customer string
			Project  string
		}
		type groupVal struct {
			Seconds int64
			Notes   []string
		}
		groups := map[groupKey]*groupVal{}
		dayTotals := map[string]int64{}
		weekTotal := int64(0)

		for _, s := range segments {
			k := groupKey{Day: s.Day, Customer: s.Customer, Project: s.Project}
			if _, ok := groups[k]; !ok {
				groups[k] = &groupVal{Seconds: 0, Notes: []string{}}
			}
			groups[k].Seconds += s.Seconds
			// append notes preserving chronological order
			for _, n := range s.Notes {
				normalized := normalizeNote(n)
				if normalized != "" {
					groups[k].Notes = append(groups[k].Notes, normalized)
				}
			}
			dayTotals[s.Day] += s.Seconds
			weekTotal += s.Seconds
		}

		// Display rounding: flag rwRoundFlag is divisions-per-hour (e.g., 6 -> 10 min quantum)
		if rwRoundFlag <= 0 {
			rwRoundFlag = 6 // default per spec (10-minute granularity)
		}
		quantumMin := 60 / rwRoundFlag
		if quantumMin <= 0 {
			quantumMin = 10
		}
		quantumSec := int64(quantumMin * 60)

		outDays := []outDay{}

		// Prepare ordered list of days from 'from' to 'to'
		days := []time.Time{}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			days = append(days, d)
		}

		// For each day build output groups
		for _, d := range days {
			dayKey := d.In(loc).Format("2006-01-02")
			weekdayLabel := weekdayLabelFor(d.In(loc).Weekday(), rwLocale)
			og := outDay{Date: dayKey, Weekday: weekdayLabel, Groups: []outNoteGroup{}}
			// collect groups for this day and sort by customer/project
			keys := []groupKey{}
			for k := range groups {
				if k.Day == dayKey {
					keys = append(keys, k)
				}
			}
			sort.Slice(keys, func(i, j int) bool {
				if keys[i].Customer != keys[j].Customer {
					return keys[i].Customer < keys[j].Customer
				}
				return keys[i].Project < keys[j].Project
			})
			daySec := int64(0)
			daySecRounded := int64(0)
			dayFlags := []string{}
			for _, k := range keys {
				v := groups[k]
				// dedupe and normalize notes
				notesDedup := dedupeStrings(v.Notes)
				merged := mergeNotesForDisplay(notesDedup, rwNotesWrap)
				roundedSec := roundSecondsToQuantum(v.Seconds, quantumSec)
				og.Groups = append(og.Groups, outNoteGroup{
					Customer:    k.Customer,
					Project:     k.Project,
					Seconds:     v.Seconds,
					SecRounded:  roundedSec,
					Notes:       notesDedup,
					NotesMerged: merged,
				})
				daySec += v.Seconds
				daySecRounded += roundedSec
			}
			// flags: overlap?
			if od, ok := overlapsByDay[dayKey]; ok && len(od) > 0 {
				dayFlags = append(dayFlags, "overlap")
			}
			if daySec == 0 && len(og.Groups) == 0 {
				// skip empty days unless format json requires them; we'll include empty days with ok flag
				og.Flags = []string{"ok"}
			} else {
				og.Flags = dayFlags
			}
			og.DaySeconds = daySec
			og.DaySecondsRounded = daySecRounded
			outDays = append(outDays, og)
		}

		// Render based on format
		switch rwFormatFlag {
		case "json":
			out := map[string]interface{}{
				"week":               fmtWeekLabel(from, to),
				"range":              map[string]string{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")},
				"timezone":           tzName,
				"days":               outDays,
				"weekSeconds":        weekTotal,
				"weekSecondsRounded": roundSecondsToQuantum(weekTotal, quantumSec),
				"issues": map[string]interface{}{
					"overlaps":   overlapRanges,
					"badEntries": badEntries,
				},
			}
			j, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(j))
		case "markdown":
			printMarkdownReport(from, to, tzName, outDays, weekTotal, quantumSec, overlapRanges, badEntries)
		default:
			printTableReport(from, to, tzName, outDays, weekTotal, quantumSec, overlapRanges, badEntries)
		}

		// Tempo export if requested
		if rwExportTempo != "" {
			err := writeTempoExport(rwExportTempo, outDays, weekTotal, quantumSec, rwTempoRounded)
			if err != nil {
				fmt.Printf("Failed to write tempo export: %v\n", err)
			} else {
				fmt.Printf("Tempo export written to %s\n", rwExportTempo)
			}
		}
	},
}

func init() {
	// Attach as subcommand to existing reportCmd
	reportCmd.AddCommand(reportWeekCmd)

	reportWeekCmd.Flags().StringVar(&rwWeekFlag, "week", "", "ISO week, e.g. 2025-W41 (default = current ISO week)")
	reportWeekCmd.Flags().StringVar(&rwFromFlag, "from", "", "Start date YYYY-MM-DD (overrides --week if both --from and --to set)")
	reportWeekCmd.Flags().StringVar(&rwToFlag, "to", "", "End date YYYY-MM-DD (overrides --week if both --from and --to set)")
	reportWeekCmd.Flags().StringVar(&rwFormatFlag, "format", "table", "Output format: table|json|markdown")
	reportWeekCmd.Flags().IntVar(&rwRoundFlag, "round", 4, "Rounding divisions-per-hour (e.g., 4 -> 15-minute quantum). Default 4")
	reportWeekCmd.Flags().StringVar(&rwCustomerFilter, "customer", "", "Filter by exact customer (case-insensitive)")
	reportWeekCmd.Flags().StringArrayVar(&rwTagFilters, "tag", []string{}, "Filter by tag (repeatable; AND logic)")
	reportWeekCmd.Flags().BoolVar(&rwIncludeOpen, "include-open", false, "Include entries without end time (treat end = now)")
	reportWeekCmd.Flags().IntVar(&rwNotesWrap, "notes-wrap", 80, "Wrap merged notes to N columns (0 = no wrap)")
	reportWeekCmd.Flags().StringVar(&rwLocale, "locale", "de", "Locale for weekday labels: de|en")
	reportWeekCmd.Flags().StringVar(&rwExportTempo, "export-tempo", "", "Write Tempo JSON export to path")
	reportWeekCmd.Flags().BoolVar(&rwTempoRounded, "tempo-rounded", false, "When exporting to Tempo use rounded seconds instead of raw")
}

// ---------- Helper functions ----------

func parseISOWeek(s string) (int, int, error) {
	// Accept patterns: "2025-W41" or "2025W41"
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("empty")
	}
	var yearPart, weekPart string
	if strings.Contains(s, "-W") {
		parts := strings.SplitN(s, "-W", 2)
		yearPart = parts[0]
		weekPart = parts[1]
	} else if strings.Contains(s, "W") {
		parts := strings.SplitN(s, "W", 2)
		yearPart = parts[0]
		weekPart = parts[1]
	} else {
		return 0, 0, fmt.Errorf("expected format YYYY-Www")
	}
	y, err := strconv.Atoi(yearPart)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid year")
	}
	w, err := strconv.Atoi(weekPart)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid week")
	}
	if w < 1 || w > 53 {
		return 0, 0, fmt.Errorf("week out of range")
	}
	return y, w, nil
}

// isoWeekRange returns Monday 00:00..Sunday 23:59:59 in loc
func isoWeekRange(year, week int, loc *time.Location) (time.Time, time.Time) {
	// Find the Monday of the requested ISO week.
	// Algorithm: start from Jan 4 of the year (always in week 1), find week1 Monday, then add (week-1)*7 days.
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, loc)
	// Determine Monday of week 1
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mondayWeek1 := jan4.AddDate(0, 0, -(wd - 1))
	// Monday of desired week:
	targetMonday := mondayWeek1.AddDate(0, 0, (week-1)*7)
	// ensure time component zeroed
	start := time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 6)
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, loc)
	return start, end
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func normalizeNote(s string) string {
	// Trim, collapse whitespace and newlines into single spaces
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// collapse internal whitespace
	f := strings.Fields(s)
	return strings.Join(f, " ")
}

func dedupeStrings(arr []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range arr {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func mergeNotesForDisplay(notes []string, wrapCols int) string {
	if len(notes) == 0 {
		return ""
	}
	joined := strings.Join(notes, " • ")
	if wrapCols <= 0 {
		return joined
	}
	// simple wrap: insert newline when line exceeds wrapCols at space
	var b strings.Builder
	lineLen := 0
	words := strings.Fields(joined)
	for i, w := range words {
		if i > 0 {
			// space before word
			lineLen++
			if lineLen > wrapCols {
				b.WriteString("\n")
				lineLen = 0
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(w)
		lineLen += len(w)
	}
	return b.String()
}

func roundSecondsToQuantum(sec int64, quantumSec int64) int64 {
	if quantumSec <= 0 {
		return sec
	}
	if sec <= 0 {
		return 0
	}
	rem := sec % quantumSec
	if rem*2 >= quantumSec {
		return ((sec / quantumSec) + 1) * quantumSec
	}
	return (sec / quantumSec) * quantumSec
}

// roundUpSecondsToQuantum always rounds up to the next quantum (unless already aligned).
// This is used for per-entry rounding semantics (each entry rounded up to N-minute intervals).
func roundUpSecondsToQuantum(sec int64, quantumSec int64) int64 {
	if quantumSec <= 0 {
		return sec
	}
	if sec <= 0 {
		return 0
	}
	if sec%quantumSec == 0 {
		return sec
	}
	return ((sec / quantumSec) + 1) * quantumSec
}

func fmtWeekLabel(from, to time.Time) string {
	// Try to get ISO week label like "2025-W41"
	y, w := from.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

func weekdayLabelFor(wd time.Weekday, locale string) string {
	en := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	de := []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	if strings.ToLower(locale) == "en" {
		return en[int(wd)]
	}
	return de[int(wd)]
}

func printTableReport(from, to time.Time, tz string, days []outDay, weekTotal int64, quantumSec int64, overlaps []string, badEntries []string) {
	// Header
	fmt.Printf("Woche %s (%s–%s) · %s\n\n", fmtWeekLabel(from, to), from.Format("02.01.2006"), to.Format("02.01.2006"), tz)
	totalHours := float64(weekTotal) / 3600.0
	for _, d := range days {
		// Weekday short and date
		fmt.Printf("%s %s\n", d.Weekday, d.Date)
		for _, g := range d.Groups {
			hours := float64(g.Seconds) / 3600.0
			// simple formatted line: "  Customer / Project   3.5 h   - note1; note2"
			proj := g.Project
			if proj != "" {
				fmt.Printf("  %s / %s\t%.2fh\t- %s\n", g.Customer, proj, hours, g.NotesMerged)
			} else {
				fmt.Printf("  %s\t%.2fh\t- %s\n", g.Customer, hours, g.NotesMerged)
			}
		}
		dayHours := float64(d.DaySeconds) / 3600.0
		flagMarker := ""
		if len(d.Flags) > 0 {
			flagMarker = "  ! overlap"
		}
		fmt.Printf("\n  Tagessumme\t%.2fh%s\n\n", dayHours, flagMarker)
	}
	fmt.Printf("Wochensumme:\t%.2fh\n", totalHours)
	if len(overlaps) > 0 || len(badEntries) > 0 {
		fmt.Println("Hinweise:")
		for _, o := range overlaps {
			fmt.Printf("  ! overlap: %s\n", o)
		}
		if len(badEntries) > 0 {
			fmt.Printf("  Data issues: %d entries\n", len(badEntries))
			for _, be := range badEntries {
				fmt.Printf("    - %s\n", be)
			}
		}
	}
}

func printMarkdownReport(from, to time.Time, tz string, days []outDay, weekTotal int64, quantumSec int64, overlaps []string, badEntries []string) {
	fmt.Printf("# Woche %s (%s–%s) · %s\n\n", fmtWeekLabel(from, to), from.Format("2006-01-02"), to.Format("2006-01-02"), tz)
	for _, d := range days {
		fmt.Printf("## %s %s\n\n", d.Weekday, d.Date)
		for _, g := range d.Groups {
			h := float64(g.Seconds) / 3600.0
			if g.Project != "" {
				fmt.Printf("- **%s / %s** — %.2fh\n\n  %s\n", g.Customer, g.Project, h, g.NotesMerged)
			} else {
				fmt.Printf("- **%s** — %.2fh\n\n  %s\n", g.Customer, h, g.NotesMerged)
			}
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\n**Wochensumme:** %.2fh\n\n", float64(weekTotal)/3600.0)
	if len(overlaps) > 0 || len(badEntries) > 0 {
		fmt.Println("Hinweise:")
		for _, o := range overlaps {
			fmt.Printf("- ! overlap: %s\n", o)
		}
		if len(badEntries) > 0 {
			fmt.Printf("- Data issues (%d):\n", len(badEntries))
			for _, be := range badEntries {
				fmt.Printf("  - %s\n", be)
			}
		}
	}
}

func writeTempoExport(path string, days []outDay, weekTotal int64, quantumSec int64, tempoRounded bool) error {
	type tempoWL struct {
		Date             string                 `json:"date"`
		StartTime        string                 `json:"startTime"`
		TimeSpentSeconds int64                  `json:"timeSpentSeconds"`
		Description      string                 `json:"description"`
		Attributes       map[string]interface{} `json:"attributes"`
	}
	var out []tempoWL
	for _, d := range days {
		for _, g := range d.Groups {
			seconds := g.Seconds
			if tempoRounded {
				seconds = g.SecRounded
			}
			if seconds <= 0 {
				continue
			}
			desc := g.NotesMerged
			if len(desc) > 250 {
				desc = desc[:250]
			}
			attr := map[string]interface{}{}
			attr["customer"] = g.Customer
			if g.Project != "" {
				attr["project"] = g.Project
			}
			attr["tags"] = []string{} // tag information not preserved at this aggregated level in current implementation
			out = append(out, tempoWL{
				Date:             d.Date,
				StartTime:        "09:00",
				TimeSpentSeconds: seconds,
				Description:      fmt.Sprintf("%s (customer: %s, project: %s)", desc, g.Customer, g.Project),
				Attributes:       attr,
			})
		}
	}
	// ensure directory exists
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	return w.Flush()
}
