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
	Args:  cobra.RangeArgs(1, 4),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse a flexible range from the leading tokens. ParseFlexibleRange will consume
		// combined tokens like `9-12`, `yesterday 09:00 10:30`, `13:00 +45m`, `now-30m`, etc.
		st, en, consumed, err := ParseFlexibleRange(args, Now())
		if err != nil {
			cobra.CheckErr(err)
		}

		// If the flexible parser returned a start without an end, allow the next positional
		// token to be the explicit end (preserves legacy `add <start> <end>` usage).
		if en.IsZero() {
			if len(args) <= consumed {
				cobra.CheckErr(fmt.Errorf("end time is required"))
			}
			en = mustParseTimeLocal(args[consumed])
			consumed++
		}

		// Validate range
		if !en.After(st) {
			cobra.CheckErr(fmt.Errorf("end time must be after start time"))
		}

		customer, project := "", ""
		if len(args) > consumed {
			customer = args[consumed]
		}
		if len(args) > consumed+1 {
			project = args[consumed+1]
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
