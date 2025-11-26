package cmd

import "github.com/spf13/cobra"

// exportCmd is a parent for export-oriented subcommands (e.g., ICS calendar files).
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export journal data to external formats",
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
