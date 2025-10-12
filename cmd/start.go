package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	startActivity string
	startBillable bool
	startTags     []string
	startNote     string
	startAt       string
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
		// Determine timestamp: either provided via --at or Now provider (injected for tests)
		ts := Now()
		if startAt != "" {
			ts = mustParseTimeLocal(startAt)
		}
		id := IDGen()
		billable := boolPtr(startBillable)
		ev := NewStartEvent(id, customer, project, startActivity, billable, startNote, startTags, ts)
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("Started: %s %s [%s] billable=%v\n", customer, project, startActivity, fmtBillable(billable))
	},
}

func init() {
	startCmd.Flags().StringVarP(&startActivity, "activity", "a", "", "activity (design, workshop, docs, travel, etc.)")
	startCmd.Flags().BoolVarP(&startBillable, "billable", "b", true, "mark as billable (default true)")
	startCmd.Flags().StringSliceVarP(&startTags, "tag", "t", []string{}, "add tag(s)")
	startCmd.Flags().StringVarP(&startNote, "note", "n", "", "note for this entry")
	startCmd.Flags().StringVar(&startAt, "at", "", "custom start time (accepts same formats as 'add')")
}
