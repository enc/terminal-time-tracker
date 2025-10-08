package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current running entry",
	Run: func(cmd *cobra.Command, args []string) {
		id := fmt.Sprintf("tt_%d", time.Now().UnixNano())
		ev := Event{ID: id, Type: "stop", TS: nowLocal()}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Println("Stopped current entry.")
	},
}
