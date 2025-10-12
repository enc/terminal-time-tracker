package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current running entry",
	Run: func(cmd *cobra.Command, args []string) {
		ev := NewStopEvent(IDGen(), Now())
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write stop event: %w", err))
		}
		fmt.Println("Stopped current entry.")
	},
}
