package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	addActivity string
	addBillable bool
	addTags     []string
	addNote     string
)

var addCmd = &cobra.Command{
	Use:   "add <start> <end> [customer] [project]",
	Short: "Add a past time entry (retro)",
	Args:  cobra.RangeArgs(2, 4),
	Run: func(cmd *cobra.Command, args []string) {
		st := mustParseTimeLocal(args[0])
		en := mustParseTimeLocal(args[1])

		// Validate range
		if !en.After(st) {
			cobra.CheckErr(fmt.Errorf("end time must be after start time"))
		}

		customer, project := "", ""
		if len(args) > 2 {
			customer = args[2]
		}
		if len(args) > 3 {
			project = args[3]
		}

		id := IDGen()
		ev := NewAddEvent(id, customer, project, addActivity, boolPtr(addBillable), addNote, addTags, st, en)
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("Added %s..%s %s %s [%s]\n", st.Format(time.Kitchen), en.Format(time.Kitchen), customer, project, addActivity)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addActivity, "activity", "a", "", "activity (design, workshop, docs, travel, etc.)")
	addCmd.Flags().BoolVarP(&addBillable, "billable", "b", true, "mark as billable (default true)")
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", []string{}, "tag(s)")
	addCmd.Flags().StringVarP(&addNote, "note", "n", "", "note")
}
