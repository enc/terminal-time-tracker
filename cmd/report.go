package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	repToday    bool
	repWeek     bool
	repRange    string
	repBy       string
	repDetailed bool
)

type aggKey struct {
	Customer, Project, Activity string
	Billable                    bool
}

type aggVal struct{ RawMin, RoundedMin int }

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Summarize entries (billable-ready)",
	Run: func(cmd *cobra.Command, args []string) {
		from, to := parseRangeFlags(repToday, repWeek, repRange)
		entries, err := loadEntries(from, to)
		if err != nil {
			// preserve previous behaviour of continuing on parse errors, but surface a message
			fmt.Printf("Warning: failed to load some entries: %v\n", err)
		}
		if len(entries) == 0 {
			fmt.Println("No entries.")
			return
		}

		// Rounding config
		r := getRounding()

		// Determine grouping fields
		by := strings.Split(repBy, ",")
		useBy := map[string]bool{}
		for _, f := range by {
			if f != "" {
				useBy[strings.TrimSpace(f)] = true
			}
		}

		// Aggregation structures
		agg := map[aggKey]*aggVal{}
		groupEntries := map[aggKey][]Entry{}
		totalRaw, totalRounded := 0, 0
		considered := 0

		for _, e := range entries {
			min := durationMinutes(e)
			// skip running or zero-length entries for reporting
			if min <= 0 {
				continue
			}
			considered++
			rmin := roundMinutes(min, r)
			k := aggKey{}
			if useBy["customer"] {
				k.Customer = e.Customer
			}
			if useBy["project"] {
				k.Project = e.Project
			}
			if useBy["activity"] {
				k.Activity = e.Activity
			}
			if useBy["billable"] {
				k.Billable = e.Billable
			}
			if _, ok := agg[k]; !ok {
				agg[k] = &aggVal{}
			}
			agg[k].RawMin += min
			agg[k].RoundedMin += rmin
			totalRaw += min
			totalRounded += rmin

			// store entry for detailed output
			groupEntries[k] = append(groupEntries[k], e)
		}

		// Header / summary
		fmt.Printf("Report Range: %s → %s   TZ: %s\n", from.Format("2006-01-02"), to.Format("2006-01-02"), time.Now().Location())
		fmt.Printf("Loaded entries: %d   Considered (finished): %d   Rounding: strategy=%s quantum=%d minimum=%d\n\n",
			len(entries), considered, r.Strategy, r.QuantumMin, r.MinimumEntry)

		if considered == 0 {
			fmt.Println("No finished entries in the selected range.")
			return
		}

		// Sort keys for deterministic output
		keys := make([]aggKey, 0, len(agg))
		for k := range agg {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			// customer, project, activity lexical
			if keys[i].Customer != keys[j].Customer {
				return keys[i].Customer < keys[j].Customer
			}
			if keys[i].Project != keys[j].Project {
				return keys[i].Project < keys[j].Project
			}
			return keys[i].Activity < keys[j].Activity
		})

		// Print groups using helper for consistent week/day formatting
		fmt.Print(formatGroups(agg, groupEntries, repDetailed))

		// Overall total
		fmt.Printf("TOTAL: %s raw → %s rounded (+%dm)\n", fmtHHMM(totalRaw), fmtHHMM(totalRounded), totalRounded-totalRaw)
	},
}

func init() {
	reportCmd.Flags().BoolVar(&repToday, "today", false, "today only")
	reportCmd.Flags().BoolVar(&repWeek, "week", false, "this week (Mon..Sun)")
	reportCmd.Flags().StringVar(&repRange, "range", "", "custom range A..B (ISO or YYYY-MM-DDTHH:MM)")
	reportCmd.Flags().StringVar(&repBy, "by", "customer,project,activity", "group by fields (comma-separated)")
	reportCmd.Flags().BoolVar(&repDetailed, "detailed", false, "detailed report including per-entry notes and times")
}
