package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Alias represents a reusable preset for starting/switching entries.
type Alias struct {
	Customer string   `mapstructure:"customer"`
	Project  string   `mapstructure:"project"`
	Activity string   `mapstructure:"activity"`
	Billable *bool    `mapstructure:"billable"`
	Tags     []string `mapstructure:"tags"`
	Note     string   `mapstructure:"note"`
}

var (
	startAlias  string
	switchAlias string
)

// aliasCmd is the top-level alias management command.
var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage aliases (presets) for quick start/switch",
}

// aliasListCmd lists available aliases.
var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List defined aliases",
	Run: func(cmd *cobra.Command, args []string) {
		aliases := loadAliases()
		if len(aliases) == 0 {
			fmt.Println("no aliases defined")
			return
		}
		// sorted output
		keys := make([]string, 0, len(aliases))
		for k := range aliases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			a := aliases[k]
			bill := "auto"
			if a.Billable != nil {
				bill = strconv.FormatBool(*a.Billable)
			}
			fmt.Printf("%s: %s/%s [%s] billable=%s tags=%v note=%q\n", k, a.Customer, a.Project, a.Activity, bill, a.Tags, a.Note)
		}
	},
}

// aliasShowCmd prints a single alias
var aliasShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show alias details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if a, ok := getAlias(name); ok {
			bill := "auto"
			if a.Billable != nil {
				bill = strconv.FormatBool(*a.Billable)
			}
			fmt.Printf("%s:\n  customer: %s\n  project:  %s\n  activity: %s\n  billable: %s\n  tags:     %v\n  note:     %q\n", name, a.Customer, a.Project, a.Activity, bill, a.Tags, a.Note)
			return
		}
		fmt.Printf("alias %q not found\n", name)
	},
}

// aliasSetCmd creates or updates an alias
var (
	setCustomer string
	setProject  string
	setActivity string
	setBillable bool
	setTags     []string
	setNote     string
)

var aliasSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Create or update an alias",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		// We want to be able to distinguish whether billable flag was provided.
		var billPtr *bool
		if cmd.Flags().Changed("billable") {
			b := setBillable
			billPtr = &b
		}
		a := Alias{
			Customer: setCustomer,
			Project:  setProject,
			Activity: setActivity,
			Billable: billPtr,
			Tags:     setTags,
			Note:     setNote,
		}
		if err := setAlias(name, a); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("alias %q set\n", name)
	},
}

var aliasRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove an alias",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := deleteAlias(name); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("alias %q removed\n", name)
	},
}

func init() {
	// wire alias command into root
	rootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasListCmd, aliasSetCmd, aliasRmCmd, aliasShowCmd)

	// flags for alias set
	aliasSetCmd.Flags().StringVar(&setCustomer, "customer", "", "customer")
	aliasSetCmd.Flags().StringVar(&setProject, "project", "", "project")
	aliasSetCmd.Flags().StringVarP(&setActivity, "activity", "a", "", "activity")
	aliasSetCmd.Flags().BoolVar(&setBillable, "billable", true, "billable (set explicitly)")
	aliasSetCmd.Flags().StringSliceVarP(&setTags, "tag", "t", []string{}, "tags")
	aliasSetCmd.Flags().StringVarP(&setNote, "note", "n", "", "note")

	// Add alias flag to start and switch commands and set up pre-run handlers.
	startCmd.Flags().StringVar(&startAlias, "alias", "", "use named alias to prefill fields (activity, billable, tags, note)")
	switchCmd.Flags().StringVar(&switchAlias, "alias", "", "use named alias to prefill fields (activity, billable, tags, note)")

	// PreRunE handlers apply alias values to flags so start/switch Run functions pick them up.
	// We only override flag-backed values (activity, billable, tag, note). Customer/project positional
	// args are not injected here to avoid surprising behaviour; users may still pass them on the command line.
	startCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if startAlias == "" {
			return nil
		}
		a, ok := getAlias(startAlias)
		if !ok {
			return fmt.Errorf("alias %q not found", startAlias)
		}
		applyAliasToFlags(cmd, a)
		return nil
	}
	switchCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if switchAlias == "" {
			return nil
		}
		a, ok := getAlias(switchAlias)
		if !ok {
			return fmt.Errorf("alias %q not found", switchAlias)
		}
		applyAliasToFlags(cmd, a)
		return nil
	}
}

// Helper: apply alias fields to flags on a command (activity, billable, tag, note)
func applyAliasToFlags(cmd *cobra.Command, a Alias) {
	// activity
	if a.Activity != "" {
		_ = cmd.Flags().Set("activity", a.Activity)
	}
	// billable
	if a.Billable != nil {
		_ = cmd.Flags().Set("billable", strconv.FormatBool(*a.Billable))
	}
	// tags - join with comma (cobra string-slice parsing will split)
	if len(a.Tags) > 0 {
		_ = cmd.Flags().Set("tag", strings.Join(a.Tags, ","))
	}
	// note
	if a.Note != "" {
		_ = cmd.Flags().Set("note", a.Note)
	}
	// NOTE: customer/project are positional args; we do not inject them here to avoid unexpected overrides.
}

// --------- persistence helpers backed by viper config under "aliases" key ----------

// in-memory cache to ensure immediate visibility of aliases after setAlias/deleteAlias.
// Tests and runtime callers can rely on loadAliases() reflecting recent changes without
// requiring a disk read round-trip.
var aliasesCache map[string]Alias

func configFilePath() string {
	if cf := viper.ConfigFileUsed(); cf != "" {
		return cf
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tt")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "config.yaml")
}

// loadAliases returns aliases from the in-memory cache if present, otherwise it reads
// from viper (in-memory config) and falls back to reading the conventional on-disk
// config file ($HOME/.tt/config.yaml) if needed. The result is cached in aliasesCache.
func loadAliases() map[string]Alias {
	// Return a copy of cache if populated to avoid accidental mutation by callers.
	if aliasesCache != nil {
		copy := map[string]Alias{}
		for k, v := range aliasesCache {
			copy[k] = v
		}
		return copy
	}

	// Prefer the in-memory viper aliases if present
	raw := viper.GetStringMap("aliases")
	if len(raw) == 0 {
		// fallback: attempt to read on-disk config
		cf := configFilePath()
		if _, err := os.Stat(cf); err == nil {
			viper.SetConfigFile(cf)
			viper.SetConfigType("yaml")
			_ = viper.ReadInConfig()
			raw = viper.GetStringMap("aliases")
		}
	}

	out := map[string]Alias{}
	for k, v := range raw {
		// v is likely map[string]interface{}
		if mm, ok := v.(map[string]interface{}); ok {
			a := Alias{}
			if cs, ok := mm["customer"].(string); ok {
				a.Customer = cs
			}
			if ps, ok := mm["project"].(string); ok {
				a.Project = ps
			}
			if act, ok := mm["activity"].(string); ok {
				a.Activity = act
			}
			if b, ok := mm["billable"]; ok {
				// could be bool or string
				switch tb := b.(type) {
				case bool:
					a.Billable = &tb
				case string:
					if parsed, err := strconv.ParseBool(tb); err == nil {
						a.Billable = &parsed
					}
				}
			}
			if tg, ok := mm["tags"].([]interface{}); ok {
				for _, iv := range tg {
					if s, ok := iv.(string); ok {
						a.Tags = append(a.Tags, s)
					}
				}
			}
			if n, ok := mm["note"].(string); ok {
				a.Note = n
			}
			out[k] = a
		}
	}

	// populate cache
	aliasesCache = map[string]Alias{}
	for k, v := range out {
		aliasesCache[k] = v
	}

	// return a copy to the caller
	copyOut := map[string]Alias{}
	for k, v := range aliasesCache {
		copyOut[k] = v
	}
	return copyOut
}

func getAlias(name string) (Alias, bool) {
	aliases := loadAliases()
	a, ok := aliases[name]
	return a, ok
}

func setAlias(name string, a Alias) error {
	// Ensure we have current aliases in cache
	aliases := loadAliases()
	aliases[name] = a
	// update cache with the new map
	aliasesCache = map[string]Alias{}
	for k, v := range aliases {
		aliasesCache[k] = v
	}

	// convert back to map[string]any and set into viper
	out := map[string]map[string]any{}
	for k, v := range aliases {
		m := map[string]any{}
		if v.Customer != "" {
			m["customer"] = v.Customer
		}
		if v.Project != "" {
			m["project"] = v.Project
		}
		if v.Activity != "" {
			m["activity"] = v.Activity
		}
		if v.Billable != nil {
			m["billable"] = *v.Billable
		}
		if len(v.Tags) > 0 {
			m["tags"] = v.Tags
		}
		if v.Note != "" {
			m["note"] = v.Note
		}
		out[k] = m
	}
	viper.Set("aliases", out)
	return saveViperConfig()
}

func deleteAlias(name string) error {
	// Ensure cache is populated
	aliases := loadAliases()
	if _, ok := aliases[name]; !ok {
		return fmt.Errorf("alias %q not found", name)
	}
	delete(aliases, name)
	// update cache
	aliasesCache = map[string]Alias{}
	for k, v := range aliases {
		aliasesCache[k] = v
	}

	// rebuild map for viper persistence
	out := map[string]map[string]any{}
	for k, v := range aliases {
		m := map[string]any{}
		if v.Customer != "" {
			m["customer"] = v.Customer
		}
		if v.Project != "" {
			m["project"] = v.Project
		}
		if v.Activity != "" {
			m["activity"] = v.Activity
		}
		if v.Billable != nil {
			m["billable"] = *v.Billable
		}
		if len(v.Tags) > 0 {
			m["tags"] = v.Tags
		}
		if v.Note != "" {
			m["note"] = v.Note
		}
		out[k] = m
	}
	viper.Set("aliases", out)
	return saveViperConfig()
}

// saveViperConfig tries to write the config back to the configured file, falling back to $HOME/.tt/config.yaml.
func saveViperConfig() error {
	// Try WriteConfig first (will fail if no config file yet)
	if err := viper.WriteConfig(); err == nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".tt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, "config.yaml")
	return viper.WriteConfigAs(p)
}
