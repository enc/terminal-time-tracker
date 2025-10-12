package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note <text>",
	Short: "Attach a note to the current running entry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := IDGen()
		ev := Event{ID: id, Type: "note", TS: Now(), Note: args[0]}
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write note event: %w", err))
		}
		fmt.Println("Added note.")
	},
}
