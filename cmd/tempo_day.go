
package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	dayDate     string
	dayToday    bool
	dayGroupBy  string
	dayRound    string
	dayIssue    string
)

var tempoDayCmd = &cobra.Command{
	Use:   "day",
	Short: "Show a consolidated view of a day to quickly book into Jira/Tempo",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine day
		var day time.Time
		if dayToday { day = nowLocal() } else if dayDate != "" { day = mustParseTimeLocal(dayDate) } else { day = nowLocal() }
		from, to := day, day
		entries, _ := loadEntries(from, to)
		if len(entries) == 0 { fmt.Println("No entries."); return nil }

		r := getRounding()
		useRounded := (dayRound != "raw")

		// Pick default issue via mapping if not provided
		cfg := readJiraCfg()
		issue := dayIssue
		if issue == "" {
			first := entries[0]
			key := strings.ToLower(strings.TrimSpace(first.Customer)) + "|" + strings.ToLower(strings.TrimSpace(first.Project))
			if v, ok := cfg.Mappings[key]; ok { issue = v }
		}

		// Aggregate
		type line struct{ Label string; Raw, Rounded int; Examples []string }
		m := map[string]*line{}
		totalRaw, totalRounded := 0, 0
		for _, e := range entries {
			lbl := "all"
			switch dayGroupBy {
			case "activity": lbl = e.Activity
			case "project": lbl = e.Project
			case "customer": lbl = e.Customer
			}
			if _, ok := m[lbl]; !ok { m[lbl] = &line{Label: lbl, Examples: []string{}} }
			min := durationMinutes(e)
			if min <= 0 { continue }
			rmin := min
			if useRounded { rmin = roundMinutes(min, r) }
			m[lbl].Raw += min
			m[lbl].Rounded += rmin
			if len(m[lbl].Examples) < 2 { m[lbl].Examples = append(m[lbl].Examples, summarizeEntry(e)) }
			totalRaw += min; totalRounded += rmin
		}

		// Print table
		fmt.Printf("\nConsolidated view — %s  (group-by: %s, minutes: %s)\n", day.Format("2006-01-02 (Mon)"), dayGroupBy, map[bool]string{true:"rounded", false:"raw"}[useRounded])
		fmt.Println("--------------------------------------------------------------------------------")
		keys := make([]string, 0, len(m))
		for k := range m { keys = append(keys, k) }
		sort.Strings(keys)
		for _, k := range keys {
			ln := m[k]
			fmt.Printf("%-20s  raw=%5s  rounded=%5s  ex: %s\n", k, fmtHHMM(ln.Raw), fmtHHMM(ln.Rounded), strings.Join(ln.Examples, " | "))
		}
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf("TOTAL: raw=%s → rounded=%s (+%dm)\n", fmtHHMM(totalRaw), fmtHHMM(totalRounded), totalRounded-totalRaw)

		// Suggest booking command
		base := fmt.Sprintf("./tt tempo book --date %s --group-by %s --round %s", day.Format("2006-01-02"), dayGroupBy, map[bool]string{true:"rounded", false:"raw"}[useRounded])
		if issue != "" { base += " --issue " + issue }
		fmt.Printf("\nTo book this day%s:\n  %s\n", ternary(issue=="", " (set --issue or add a mapping in config)", ""), base)
		return nil
	},
}

func init() {
	tempoCmd.AddCommand(tempoDayCmd)
	tempoDayCmd.Flags().BoolVar(&dayToday, "today", false, "show today's consolidated view")
	tempoDayCmd.Flags().StringVar(&dayDate, "date", "", "specific date (YYYY-MM-DD)")
	tempoDayCmd.Flags().StringVar(&dayGroupBy, "group-by", "activity", "group by: activity|project|customer|none")
	tempoDayCmd.Flags().StringVar(&dayRound, "round", "rounded", "use rounded or raw minutes: rounded|raw")
	tempoDayCmd.Flags().StringVar(&dayIssue, "issue", "", "Jira issue to suggest")
}

func ternary[T any](cond bool, a, b T) T { if cond { return a }; return b }
