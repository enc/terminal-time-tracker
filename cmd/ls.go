package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	lsToday bool
	lsRange string
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List entries for a period (default today)",
	Run: func(cmd *cobra.Command, args []string) {
		from, to := parseRangeFlags(lsToday, false, lsRange)
		entries, _ := loadEntries(from, to)
		if len(entries) == 0 {
			fmt.Println("No entries.")
			return
		}
		fmt.Printf("Range: %s..%s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"))
		for _, e := range entries {
			end := "running"
			if e.End != nil { end = e.End.Format("15:04") }
			fmt.Printf("%s  %s-%s  %-8s  %-20s  %-20s  billable=%v  %s\n",
				e.Start.Format("2006-01-02"), e.Start.Format("15:04"), end,
				e.Activity, e.Customer, e.Project, e.Billable, fmtHHMM(durationMinutes(e)))
			if len(e.Notes) > 0 { fmt.Printf("    notes: %v\n", e.Notes) }
		}
	},
}

func init() {
	lsCmd.Flags().BoolVar(&lsToday, "today", false, "today only")
	lsCmd.Flags().StringVar(&lsRange, "range", "", "custom range A..B (ISO or YYYY-MM-DDTHH:MM)")
}
