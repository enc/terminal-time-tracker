package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	switchActivity string
	switchBillable bool
	switchTags     []string
	switchNote     string
	switchAt       string
)

var switchCmd = &cobra.Command{
	Use:   "switch [customer] [project]",
	Short: "Stop current and immediately start a new entry",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// Determine timestamp: either provided via --at or Now provider (injected for tests).
		// Accept flexible/relative expressions (e.g. "now-30m", "+15m", "14:30") by trying the
		// flexible parser first (same parsing used by `add`/ParseFlexibleRange). If that fails,
		// fall back to the legacy absolute parser.
		ts := Now()
		if switchAt != "" {
			if st, _, cons, err := ParseFlexibleRange([]string{switchAt}, Now()); err == nil && cons > 0 && !st.IsZero() {
				ts = st
			} else {
				ts = mustParseTimeLocal(switchAt)
			}
		}

		// stop - ensure we handle any error (use ts so stop/start are aligned)
		stopEv := NewStopEvent(IDGen(), ts)
		if err := Writer.WriteEvent(stopEv); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write stop event: %w", err))
		}

		// start
		customer, project := "", ""
		if len(args) > 0 {
			customer = args[0]
		}
		if len(args) > 1 {
			project = args[1]
		}
		id := IDGen()
		billable := boolPtr(switchBillable)
		ev := NewStartEvent(id, customer, project, switchActivity, billable, switchNote, switchTags, ts)
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("Switched to: %s %s [%s] billable=%v\n", customer, project, switchActivity, fmtBillable(billable))
	},
}

func init() {
	switchCmd.Flags().StringVarP(&switchActivity, "activity", "a", "", "activity for new entry")
	switchCmd.Flags().BoolVarP(&switchBillable, "billable", "b", true, "mark as billable (default true)")
	switchCmd.Flags().StringSliceVarP(&switchTags, "tag", "t", []string{}, "add tag(s)")
	switchCmd.Flags().StringVarP(&switchNote, "note", "n", "", "note for new entry")
	switchCmd.Flags().StringVar(&switchAt, "at", "", "custom switch time (accepts same formats as 'add', including relative expressions like 'now-30m' or '+15m')")
}
