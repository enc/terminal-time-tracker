package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pauseAt string
var pauseIgnoreBudget bool

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause the current running entry",
	Run: func(cmd *cobra.Command, args []string) {
		ts := Now()
		if pauseAt != "" {
			if st, _, cons, err := ParseFlexibleRange([]string{pauseAt}, Now()); err == nil && cons > 0 && !st.IsZero() {
				ts = st
			} else {
				ts = mustParseTimeLocal(pauseAt)
			}
		}

		running, _ := LastOpenEntryAt(ts)
		if running == nil {
			cobra.CheckErr(fmt.Errorf("no running entry to pause at %s", formatTS(ts)))
		}

		if !pauseIgnoreBudget {
			budgets := toRuntimeBudgets(loadBudgets())
			cand := budgetCandidate{Customer: running.Customer, Project: running.Project, Activity: running.Activity, Start: running.Start, End: &ts}
			relevant := filterBudgetsForCandidate(budgets, cand)
			if len(relevant) > 0 {
				from, to := windowForBudgets(relevant, cand)
				entries, _ := loadEntries(from, to)
				var filtered []Entry
				for _, e := range entries {
					if e.ID == running.ID {
						continue // avoid double-counting the running entry; candidate covers it
					}
					filtered = append(filtered, e)
				}
				warnings, berr := checkBudgets(relevant, filtered, cand, getRounding(), ts)
				if berr != nil {
					fmt.Printf("%sBUDGET:%s %v\n", ansiWarn, ansiReset, berr)
				}
				printBudgetWarnings(warnings)
			}
		}

		ev := NewPauseEvent(IDGen(), running.ID, ts, running)
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write pause event: %w", err))
		}

		fmt.Println(FormatPauseResultFromEntry(running, ts))
	},
}

func init() {
	pauseCmd.Flags().StringVar(&pauseAt, "at", "", "custom pause time (accepts same formats as 'add' and 'start', including relative expressions like 'now-30m' or '+15m')")
	pauseCmd.Flags().BoolVar(&pauseIgnoreBudget, "ignore-budget", false, "skip budget checks for this pause")
}
