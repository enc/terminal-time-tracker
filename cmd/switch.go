package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	switchActivity string
	switchBillable bool
	switchTags     []string
	switchNote     string
)

var switchCmd = &cobra.Command{
	Use:   "switch [customer] [project]",
	Short: "Stop current and immediately start a new entry",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// stop
		_ = writeEvent(Event{ID: fmt.Sprintf("tt_%d", time.Now().UnixNano()), Type: "stop", TS: nowLocal()})
		// start
		cust, proj := "", ""
		if len(args) > 0 {
			cust = args[0]
		}
		if len(args) > 1 {
			proj = args[1]
		}
		id := fmt.Sprintf("tt_%d", time.Now().UnixNano())
		billable := boolPtr(switchBillable)
		ev := Event{ID: id, Type: "start", TS: nowLocal(), Customer: cust, Project: proj,
			Activity: switchActivity, Billable: billable, Note: switchNote, Tags: switchTags}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("Switched to: %s %s [%s] billable=%v\n", cust, proj, switchActivity, *billable)
	},
}

func init() {
	switchCmd.Flags().StringVarP(&switchActivity, "activity", "a", "", "activity for new entry")
	switchCmd.Flags().BoolVarP(&switchBillable, "billable", "b", true, "mark as billable (default true)")
	switchCmd.Flags().StringSliceVarP(&switchTags, "tag", "t", []string{}, "add tag(s)")
	switchCmd.Flags().StringVarP(&switchNote, "note", "n", "", "note for new entry")
}
