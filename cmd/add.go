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
		cust, proj := "", ""
		if len(args) > 2 { cust = args[2] }
		if len(args) > 3 { proj = args[3] }
		id := fmt.Sprintf("tt_%d", time.Now().UnixNano())
		ev := Event{ ID: id, Type: "add", TS: nowLocal(), Customer: cust, Project: proj,
			Activity: addActivity, Billable: boolPtr(addBillable), Note: addNote, Tags: addTags,
			Ref: st.Format(time.RFC3339)+".."+en.Format(time.RFC3339),
		}
		if err := writeEvent(ev); err != nil { cobra.CheckErr(err) }
		fmt.Printf("Added %s..%s %s %s [%s]\n", st.Format(time.Kitchen), en.Format(time.Kitchen), cust, proj, addActivity)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addActivity, "activity", "a", "", "activity (design, workshop, docs, travel, etc.)")
	addCmd.Flags().BoolVarP(&addBillable, "billable", "b", true, "mark as billable (default true)")
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", []string{}, "tag(s)")
	addCmd.Flags().StringVarP(&addNote, "note", "n", "", "note")
}
