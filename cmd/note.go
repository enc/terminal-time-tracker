package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note <text>",
	Short: "Attach a note to the current running entry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := fmt.Sprintf("tt_%d", time.Now().UnixNano())
		ev := Event{ID: id, Type: "note", TS: nowLocal(), Note: args[0]}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Println("Added note.")
	},
}
