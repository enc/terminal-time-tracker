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
)

var switchCmd = &cobra.Command{
	Use:   "switch [customer] [project]",
	Short: "Stop current and immediately start a new entry",
	Args:  cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// stop - ensure we handle any error
		stopEv := NewStopEvent(IDGen(), Now())
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
		ev := NewStartEvent(id, customer, project, switchActivity, billable, switchNote, switchTags, Now())
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
}
