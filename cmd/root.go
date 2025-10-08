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
	// Try RFC3339 first (accepts explicit timezone)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}

	// Load configured timezone (fall back to local if unavailable)
	tz := viper.GetString("timezone")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.Local
	}

	// Accept full date+time without timezone (T separator)
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}

	// Accept space-separated date+time (with and without seconds)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}

	// Accept time-only inputs and assume today's date in configured location
	if t, err := time.ParseInLocation("15:04:05", s, loc); err == nil {
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
	}
	if t, err := time.ParseInLocation("15:04", s, loc); err == nil {
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}

	cobra.CheckErr(fmt.Errorf("cannot parse time: %s", s))
	return time.Now()
}
