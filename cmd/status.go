package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current active session and last entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Search a sensible window (last 7 days) for events to determine state.
		now := nowLocal()
		from := now.AddDate(0, 0, -7)
		to := now

		active, last, err := findActiveAndLast(from, to)
		if err != nil {
			return err
		}

		fmt.Println()
		// Active session
		if active != nil {
			mins := int(now.Sub(active.Start).Minutes())
			fmt.Println("Active session:")
			fmt.Printf("  Started: %s  (%s elapsed)\n", active.Start.Format("2006-01-02 15:04:05"), fmtHHMM(mins))
			if active.Customer != "" || active.Project != "" || active.Activity != "" {
				fmt.Printf("  %s / %s  [%s]  billable=%v\n", active.Customer, active.Project, active.Activity, active.Billable)
			}
			if len(active.Tags) > 0 {
				fmt.Printf("  tags: %v\n", active.Tags)
			}
			if len(active.Notes) > 0 {
				fmt.Printf("  notes: %v\n", active.Notes)
			}
		} else {
			fmt.Println("No active session.")
		}

		fmt.Println()

		// Last closed entry
		if last != nil {
			fmt.Println("Last entry:")
			endStr := ""
			if last.End != nil {
				endStr = last.End.Format("2006-01-02 15:04:05")
			} else {
				endStr = "(running)"
			}
			fmt.Printf("  %s → %s  (%s)\n", last.Start.Format("2006-01-02 15:04:05"), endStr, fmtHHMM(durationMinutes(*last)))
			if last.Customer != "" || last.Project != "" || last.Activity != "" {
				fmt.Printf("  %s / %s  [%s]  billable=%v\n", last.Customer, last.Project, last.Activity, last.Billable)
			}
			if len(last.Tags) > 0 {
				fmt.Printf("  tags: %v\n", last.Tags)
			}
			if len(last.Notes) > 0 {
				fmt.Printf("  notes: %v\n", last.Notes)
			}
		} else {
			fmt.Println("No previous closed entry found in the last 7 days.")
		}
		fmt.Println()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

// findActiveAndLast reconstructs entries from journal events between from..to (inclusive).
// It returns:
// - active: a pointer to the current running Entry if present (End == nil)
// - last: the most recently closed Entry (End != nil) seen in the window
func findActiveAndLast(from, to time.Time) (*Entry, *Entry, error) {
	// Normalize to local day boundaries
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	var events []Event
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		p := journalPathFor(d)
		b, err := os.ReadFile(p)
		if err != nil {
			// ignore missing files
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				events = append(events, ev)
			}
		}
	}
	// sort events by timestamp
	sort.Slice(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	var current *Entry
	var lastClosed *Entry

	for _, ev := range events {
		switch ev.Type {
		case "start":
			// if there is already a running entry, auto-stop it at this start time
			if current != nil {
				// close previous
				end := ev.TS
				current.End = &end
				// copy to lastClosed
				cpy := *current
				lastClosed = &cpy
			}
			billable := true
			if ev.Billable != nil {
				billable = *ev.Billable
			}
			current = &Entry{
				ID:       ev.ID,
				Start:    ev.TS,
				End:      nil,
				Customer: ev.Customer,
				Project:  ev.Project,
				Activity: ev.Activity,
				Billable: billable,
				Notes:    []string{},
				Tags:     ev.Tags,
			}
			if ev.Note != "" {
				current.Notes = append(current.Notes, ev.Note)
			}
		case "note":
			if current != nil {
				current.Notes = append(current.Notes, ev.Note)
			}
		case "stop":
			if current != nil {
				end := ev.TS
				current.End = &end
				cpy := *current
				lastClosed = &cpy
				current = nil
			}
		case "add":
			// add is a closed entry; ev.Ref is "startISO..endISO"
			parts := strings.Split(ev.Ref, "..")
			if len(parts) == 2 {
				st, err1 := time.Parse(time.RFC3339, parts[0])
				en, err2 := time.Parse(time.RFC3339, parts[1])
				if err1 == nil && err2 == nil {
					billable := true
					if ev.Billable != nil {
						billable = *ev.Billable
					}
					e := Entry{
						ID:       ev.ID,
						Start:    st,
						End:      &en,
						Customer: ev.Customer,
						Project:  ev.Project,
						Activity: ev.Activity,
						Billable: billable,
						Notes:    []string{ev.Note},
						Tags:     ev.Tags,
					}
					// this is a closed entry, so it's the most recent closed so far
					cpy := e
					lastClosed = &cpy
				}
			}
		}
	}

	// current may be non-nil here if a running session exists
	return current, lastClosed, nil
}
