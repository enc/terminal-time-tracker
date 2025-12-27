package cmd

import (
	"fmt"
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

	entries, err := loadEntries(from, to)
	if err != nil {
		return nil, nil, err
	}

	var active *Entry
	var lastClosed *Entry
	for i := range entries {
		e := entries[i]
		if e.End == nil {
			if active == nil || e.Start.After(active.Start) {
				cpy := e
				active = &cpy
			}
			continue
		}
		if lastClosed == nil || e.End.After(*lastClosed.End) {
			cpy := e
			lastClosed = &cpy
		}
	}

	return active, lastClosed, nil
}
