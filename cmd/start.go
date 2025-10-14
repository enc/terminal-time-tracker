package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	startActivity string
	startBillable bool
	startTags     []string
	startNote     string
	startAt       string
	startFor      string
)

var startCmd = &cobra.Command{
	Use:   "start [customer] [project]",
	Short: "Start tracking time (creates a running entry)",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		customer, project := "", ""
		if len(args) > 0 {
			customer = args[0]
		}
		if len(args) > 1 {
			project = args[1]
		}
		// Determine timestamp: either provided via --at or Now provider (injected for tests).
		// Accept flexible/relative expressions (e.g. "now-30m", "+15m", "14:30") by trying the
		// flexible parser first (same parsing used by `add`/ParseFlexibleRange). If that fails,
		// fall back to the legacy absolute parser.
		ts := Now()
		if startAt != "" {
			// Try flexible parsing which understands durations and now-anchored forms.
			if st, _, cons, err := ParseFlexibleRange([]string{startAt}, Now()); err == nil && cons > 0 && !st.IsZero() {
				ts = st
			} else {
				// Maintain backward compatibility with existing absolute formats.
				ts = mustParseTimeLocal(startAt)
			}
		}
		id := IDGen()
		billable := boolPtr(startBillable)
		ev := NewStartEvent(id, customer, project, startActivity, billable, startNote, startTags, ts)

		// If user provided --for, schedule an auto-stop by adding meta["auto_stop"] with RFC3339 time.
		if startFor != "" {
			d, err := parseDuration(startFor)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid --for value: %v", err))
			}
			end := ts.Add(d)
			if ev.Meta == nil {
				ev.Meta = map[string]string{}
			}
			ev.Meta["auto_stop"] = end.Format(time.RFC3339)
		}

		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(err)
		}

		if startFor != "" {
			fmt.Printf("Started: %s %s [%s] billable=%v (auto-stop at %s)\n", customer, project, startActivity, fmtBillable(billable), ev.Meta["auto_stop"])
		} else {
			fmt.Printf("Started: %s %s [%s] billable=%v\n", customer, project, startActivity, fmtBillable(billable))
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(&startActivity, "activity", "a", "", "activity (design, workshop, docs, travel, etc.)")
	startCmd.Flags().BoolVarP(&startBillable, "billable", "b", true, "mark as billable (default true)")
	startCmd.Flags().StringSliceVarP(&startTags, "tag", "t", []string{}, "add tag(s)")
	startCmd.Flags().StringVarP(&startNote, "note", "n", "", "note for this entry")
	startCmd.Flags().StringVar(&startAt, "at", "", "custom start time (accepts same formats as 'add', including relative expressions like 'now-30m' or '+15m')")
	startCmd.Flags().StringVar(&startFor, "for", "", "auto-stop after duration (e.g. 25m)")
}
