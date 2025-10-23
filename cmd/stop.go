package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopAt string

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current running entry",
	Run: func(cmd *cobra.Command, args []string) {
		// Use Now() so stop timestamp is consistent across reconstruction and writing.
		ts := Now()

		// Allow overriding via --at (supports same flexible formats as start/add).
		if stopAt != "" {
			// Try flexible parsing which understands durations and now-anchored forms.
			if st, _, cons, err := ParseFlexibleRange([]string{stopAt}, Now()); err == nil && cons > 0 && !st.IsZero() {
				ts = st
			} else {
				// Maintain backward compatibility with existing absolute formats.
				ts = mustParseTimeLocal(stopAt)
			}
		}

		// Attempt to reconstruct the running entry that was active at ts so we can
		// provide a richer, explanatory response to the user. This is best-effort:
		// when reconstruction fails or no running entry is found, FormatStopResultFromEntry
		// will produce an appropriate fallback message.
		running, _ := LastOpenEntryAt(ts)

		ev := NewStopEvent(IDGen(), ts)
		if err := Writer.WriteEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write stop event: %w", err))
		}

		// Print a detailed summary of what was stopped (or note that none was found).
		fmt.Println(FormatStopResultFromEntry(running, ts))
	},
}

func init() {
	stopCmd.Flags().StringVar(&stopAt, "at", "", "custom stop time (accepts same formats as 'add' and 'start', including relative expressions like 'now-30m' or '+15m')")
}
