package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	repToday bool
	repWeek  bool
	repRange string
	repBy    string
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
		entries, _ := loadEntries(from, to)
		if len(entries) == 0 {
			fmt.Println("No entries.")
			return
		}
		r := getRounding()

		by := strings.Split(repBy, ",")
		useBy := map[string]bool{}
		for _, f := range by {
			if f != "" {
				useBy[strings.TrimSpace(f)] = true
			}
		}

		agg := map[aggKey]*aggVal{}
		totalRaw, totalRounded := 0, 0
		for _, e := range entries {
			min := durationMinutes(e)
			if min <= 0 {
				continue
			}
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
		}

		fmt.Printf("Range: %s..%s   TZ: %s\n\n", from.Format("2006-01-02"), to.Format("2006-01-02"), time.Now().Location())
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
		for _, k := range keys {
			v := agg[k]
			fmt.Printf("Group:")
			if useBy["customer"] {
				fmt.Printf(" customer=%s", k.Customer)
			}
			if useBy["project"] {
				fmt.Printf(", project=%s", k.Project)
			}
			if useBy["activity"] {
				fmt.Printf(", activity=%s", k.Activity)
			}
			if useBy["billable"] {
				fmt.Printf(", billable=%v", k.Billable)
			}
			fmt.Printf("\n  Raw: %s   Rounded: %s (+%dm)\n\n", fmtHHMM(v.RawMin), fmtHHMM(v.RoundedMin), v.RoundedMin-v.RawMin)
		}
		fmt.Printf("TOTAL: %s raw → %s rounded (+%dm)\n", fmtHHMM(totalRaw), fmtHHMM(totalRounded), totalRounded-totalRaw)
	},
}

func init() {
	reportCmd.Flags().BoolVar(&repToday, "today", false, "today only")
	reportCmd.Flags().BoolVar(&repWeek, "week", false, "this week (Mon..Sun)")
	reportCmd.Flags().StringVar(&repRange, "range", "", "custom range A..B (ISO or YYYY-MM-DDTHH:MM)")
	reportCmd.Flags().StringVar(&repBy, "by", "customer,project,activity", "group by fields (comma-separated)")
}
