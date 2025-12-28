package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var continueAt string
var continueIgnoreBudget bool

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Continue the most recently paused session",
	Run: func(cmd *cobra.Command, args []string) {
		ts := Now()
		if continueAt != "" {
			if st, _, cons, err := ParseFlexibleRange([]string{continueAt}, Now()); err == nil && cons > 0 && !st.IsZero() {
				ts = st
			} else {
				ts = mustParseTimeLocal(continueAt)
			}
		}

		running, _ := LastOpenEntryAt(ts)
		if running != nil {
			cobra.CheckErr(fmt.Errorf("cannot continue: a session is already running (started %s)", formatTS(running.Start)))
		}

		from := ts.AddDate(0, 0, -7)
		to := ts
		events, err := loadEvents(from, to)
		if err != nil {
			cobra.CheckErr(err)
		}

		last := lastStateEventAtOrBefore(events, ts)
		if last == nil || last.Type != "pause" {
			cobra.CheckErr(fmt.Errorf("no paused session to continue before %s", formatTS(ts)))
		}

		entries, err := loadEntries(from, to)
		if err != nil {
			cobra.CheckErr(err)
		}

		paused := findEntryByID(entries, last.Ref)
		if paused == nil {
			paused = entryFromEvent(last)
		}
		if paused == nil {
			cobra.CheckErr(fmt.Errorf("paused session could not be reconstructed"))
		}

		if !continueIgnoreBudget {
			budgets := toRuntimeBudgets(loadBudgets())
			cand := budgetCandidate{Customer: paused.Customer, Project: paused.Project, Activity: paused.Activity, Start: ts}
			warnings, berr := runBudgetChecks(budgets, cand, getRounding(), Now())
			if berr != nil {
				cobra.CheckErr(berr)
			}
			printBudgetWarnings(warnings)
		}

		resume := NewResumeEvent(IDGen(), last.Ref, ts, paused)
		if err := Writer.WriteEvent(resume); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write resume event: %w", err))
		}

		fmt.Println(FormatContinueResult(resume))
	},
}

func init() {
	continueCmd.Flags().StringVar(&continueAt, "at", "", "custom continue time (accepts same formats as 'add' and 'start', including relative expressions like 'now-30m' or '+15m')")
	continueCmd.Flags().BoolVar(&continueIgnoreBudget, "ignore-budget", false, "skip budget checks for this continue")
}

func lastStateEventAtOrBefore(events []Event, ts time.Time) *Event {
	var last *Event
	for i := range events {
		if events[i].TS.After(ts) {
			continue
		}
		switch events[i].Type {
		case "start", "stop", "pause", "resume":
		default:
			continue
		}
		if last == nil || events[i].TS.After(last.TS) {
			last = &events[i]
		}
	}
	return last
}

func findEntryByID(entries []Entry, id string) *Entry {
	if id == "" {
		return nil
	}
	for i := range entries {
		if entries[i].ID == id {
			e := entries[i]
			return &e
		}
	}
	return nil
}

func entryFromEvent(ev *Event) *Entry {
	if ev == nil {
		return nil
	}
	billable := true
	if ev.Billable != nil {
		billable = *ev.Billable
	}
	return &Entry{
		Customer: ev.Customer,
		Project:  ev.Project,
		Activity: ev.Activity,
		Billable: billable,
		Tags:     ev.Tags,
	}
}
