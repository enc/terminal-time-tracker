package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "tt",
	Short: "tt — a fast, local, billing-friendly time tracker",
	Long:  "Dead-simple CLI time tracker with append-only JSONL journal and audit verification.",
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tt/config.yaml)")

	// Attach subcommands (each subcommand is in its own file)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(switchCmd)
	rootCmd.AddCommand(noteCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(auditCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		dir := filepath.Join(home, ".tt")
		_ = os.MkdirAll(dir, 0o755)
		viper.AddConfigPath(dir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}
	viper.SetDefault("timezone", "Europe/Berlin")
	// Safe read; if missing, proceed with defaults
	_ = viper.ReadInConfig()
}

func mustParseTimeLocal(s string) time.Time {
	// Accept ISO8601 or YYYY-MM-DDTHH:MM (assume local)
	if t, err := time.Parse(time.RFC3339, s); err == nil { return t }
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		tz := viper.GetString("timezone")
		loc, err := time.LoadLocation(tz)
		if err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		}
		return t
	}
	cobra.CheckErr(fmt.Errorf("cannot parse time: %s", s))
	return time.Now()
}
