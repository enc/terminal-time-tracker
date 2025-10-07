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
)

var startCmd = &cobra.Command{
	Use:   "start [customer] [project]",
	Short: "Start tracking time (creates a running entry)",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cust, proj := "", ""
		if len(args) > 0 { cust = args[0] }
		if len(args) > 1 { proj = args[1] }
		id := fmt.Sprintf("tt_%d", time.Now().UnixNano())
		billable := boolPtr(startBillable)
		ev := Event{ ID: id, Type: "start", TS: nowLocal(), Customer: cust, Project: proj,
			Activity: startActivity, Billable: billable, Note: startNote, Tags: startTags,
		}
		if err := writeEvent(ev); err != nil { cobra.CheckErr(err) }
		fmt.Printf("Started: %s %s [%s] billable=%v\n", cust, proj, startActivity, *billable)
	},
}

func init() {
	startCmd.Flags().StringVarP(&startActivity, "activity", "a", "", "activity (design, workshop, docs, travel, etc.)")
	startCmd.Flags().BoolVarP(&startBillable, "billable", "b", true, "mark as billable (default true)")
	startCmd.Flags().StringSliceVarP(&startTags, "tag", "t", []string{}, "add tag(s)")
	startCmd.Flags().StringVarP(&startNote, "note", "n", "", "note for this entry")
}
