package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	icsTodayFlag bool
	icsWeekFlag  bool
	icsRangeFlag string
	icsOutDir    string
)

// icsEvent is a minimal representation of an exported calendar item.
type icsEvent struct {
	UID         string
	Summary     string
	Description string
	Start       time.Time
	End         time.Time
}

var exportIcsCmd = &cobra.Command{
	Use:   "ics",
	Short: "Export entries as per-day .ics calendar files",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExportICS(cmd)
	},
}

func init() {
	exportCmd.AddCommand(exportIcsCmd)
	exportIcsCmd.Flags().BoolVar(&icsTodayFlag, "today", false, "export today only")
	exportIcsCmd.Flags().BoolVar(&icsWeekFlag, "week", false, "export this week (Mon..Sun)")
	exportIcsCmd.Flags().StringVar(&icsRangeFlag, "range", "", "custom range A..B (ISO or YYYY-MM-DDTHH:MM)")
	exportIcsCmd.Flags().StringVar(&icsOutDir, "out", ".", "output directory for per-day .ics files")
}

func runExportICS(cmd *cobra.Command) error {
	from, to := parseRangeFlags(icsTodayFlag, icsWeekFlag, icsRangeFlag)
	entries, err := loadEntries(from, to)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to load some entries: %v\n", err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No entries.")
		return nil
	}

	loc := loadConfiguredLocation()
	rounding := getRounding()
	now := Now()

	dayEvents, skipped := buildICSEvents(entries, loc, rounding)
	if len(dayEvents) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No finished entries to export.")
		return nil
	}

	if err := os.MkdirAll(icsOutDir, 0o755); err != nil {
		return err
	}

	// deterministic file order
	var days []string
	for d := range dayEvents {
		days = append(days, d)
	}
	sort.Strings(days)

	for _, day := range days {
		path := filepath.Join(icsOutDir, day+".ics")
		if err := writeICSFile(path, loc, now, dayEvents[day]); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d calendar files to %s\n", len(days), icsOutDir)
	if len(skipped) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Skipped %d entries (running or zero-length): %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	return nil
}

func buildICSEvents(entries []Entry, loc *time.Location, rounding Rounding) (map[string][]icsEvent, []string) {
	res := map[string][]icsEvent{}
	var skipped []string

	for _, e := range entries {
		if e.End == nil {
			skipped = append(skipped, e.ID)
			continue
		}
		totalSec := int64(e.End.Sub(e.Start).Seconds())
		if totalSec <= 0 {
			skipped = append(skipped, e.ID)
			continue
		}

		roundedMin := roundMinutes(int(e.End.Sub(e.Start).Minutes()), rounding)
		if roundedMin <= 0 {
			skipped = append(skipped, e.ID)
			continue
		}
		roundedSec := int64(roundedMin * 60)

		segs := splitEntryByDay(e, loc)
		if len(segs) == 0 {
			skipped = append(skipped, e.ID)
			continue
		}
		var rawTotal int64
		for _, s := range segs {
			rawTotal += s.rawSeconds
		}
		if rawTotal <= 0 {
			skipped = append(skipped, e.ID)
			continue
		}

		var allocated int64
		for i, s := range segs {
			alloc := (s.rawSeconds * roundedSec) / rawTotal
			if i == len(segs)-1 {
				alloc = roundedSec - allocated
			}
			if alloc <= 0 {
				continue
			}
			allocated += alloc

			end := s.start.Add(time.Duration(alloc) * time.Second)
			ev := icsEvent{
				UID:         fmt.Sprintf("%s-%s@tt", e.ID, s.start.UTC().Format("20060102T150405Z")),
				Summary:     buildIcsSummary(e),
				Description: buildIcsDescription(e),
				Start:       s.start,
				End:         end,
			}
			day := s.start.In(loc).Format("2006-01-02")
			res[day] = append(res[day], ev)
		}
	}

	// sort events by start time within each day for deterministic output
	for day := range res {
		sort.Slice(res[day], func(i, j int) bool {
			return res[day][i].Start.Before(res[day][j].Start)
		})
	}

	return res, skipped
}

type entrySegment struct {
	start      time.Time
	rawSeconds int64
}

func splitEntryByDay(e Entry, loc *time.Location) []entrySegment {
	if e.End == nil {
		return nil
	}
	start := e.Start.In(loc)
	end := e.End.In(loc)
	if !end.After(start) {
		return nil
	}

	var segs []entrySegment
	cur := start
	for cur.Before(end) {
		y, m, d := cur.Date()
		nextMidnight := time.Date(y, m, d, 0, 0, 0, 0, loc).Add(24 * time.Hour)
		segEnd := end
		if nextMidnight.Before(end) {
			segEnd = nextMidnight
		}
		raw := segEnd.Sub(cur).Seconds()
		segs = append(segs, entrySegment{
			start:      cur,
			rawSeconds: int64(raw),
		})
		cur = segEnd
	}
	return segs
}

func buildIcsSummary(e Entry) string {
	var parts []string
	if t, ok := getTicket(e.Customer, e.Project); ok && strings.TrimSpace(t) != "" {
		parts = append(parts, strings.TrimSpace(t))
	}
	customer := strings.TrimSpace(e.Customer)
	if customer == "" {
		customer = "(no customer)"
	}
	if p := strings.TrimSpace(e.Project); p != "" {
		parts = append(parts, fmt.Sprintf("%s / %s", customer, p))
	} else {
		parts = append(parts, customer)
	}
	return strings.Join(parts, " ")
}

func buildIcsDescription(e Entry) string {
	if len(e.Notes) > 0 {
		return strings.Join(e.Notes, " | ")
	}
	return ""
}

func writeICSFile(path string, loc *time.Location, now time.Time, events []icsEvent) error {
	var lines []string
	lines = append(lines, "BEGIN:VCALENDAR")
	lines = append(lines, "VERSION:2.0")
	lines = append(lines, "PRODID:-//tt//ics-export//EN")
	dtstamp := now.UTC().Format("20060102T150405Z")
	tzName := loc.String()

	for _, ev := range events {
		lines = append(lines, "BEGIN:VEVENT")
		lines = append(lines, foldICSLines("UID:"+icsEscape(ev.UID))...)
		lines = append(lines, "DTSTAMP:"+dtstamp)
		lines = append(lines, "DTSTART;TZID="+tzName+":"+ev.Start.In(loc).Format("20060102T150405"))
		lines = append(lines, "DTEND;TZID="+tzName+":"+ev.End.In(loc).Format("20060102T150405"))
		lines = append(lines, foldICSLines("SUMMARY:"+icsEscape(ev.Summary))...)
		lines = append(lines, foldICSLines("DESCRIPTION:"+icsEscape(ev.Description))...)
		lines = append(lines, "END:VEVENT")
	}

	lines = append(lines, "END:VCALENDAR")
	content := strings.Join(lines, "\r\n") + "\r\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func foldICSLines(line string) []string {
	const maxLen = 75
	if len(line) <= maxLen {
		return []string{line}
	}
	var res []string
	for len(line) > maxLen {
		res = append(res, line[:maxLen])
		line = " " + line[maxLen:]
	}
	res = append(res, line)
	return res
}

func loadConfiguredLocation() *time.Location {
	loc := time.Local
	if tz := viper.GetString("timezone"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	return loc
}
