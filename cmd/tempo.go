package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// tempoCmd is the parent command for tempo-related subcommands.
// tempo_day.go expects this to exist so it can attach its subcommand.
var tempoCmd = &cobra.Command{
	Use:   "tempo",
	Short: "Tempo/Jira booking helpers",
	Long:  "Commands to help prepare and book tracked time into Jira/Tempo.",
}

func init() {
	// Attach to root so `tt tempo ...` is available.
	// root.go will already add other subcommands; adding tempoCmd here is safe.
	rootCmd.AddCommand(tempoCmd)
}

// JiraCfg is a minimal configuration shape used by tempo_day.
// We only need mappings for default issue selection.
type JiraCfg struct {
	Mappings map[string]string
}

// readJiraCfg loads a minimal Jira configuration from viper.
// It looks for a map under key "jira.mappings" and returns it as strings.
func readJiraCfg() JiraCfg {
	m := map[string]string{}

	// viper supports reading maps of arbitrary values; normalize to strings.
	if raw := viper.GetStringMap("jira.mappings"); raw != nil {
		for k, v := range raw {
			switch val := v.(type) {
			case string:
				m[k] = val
			default:
				// Fallback to fmt.Sprintf for non-string values so tests/configs remain tolerant.
				m[k] = fmt.Sprintf("%v", val)
			}
		}
	}

	// Also accept a flat mapping encoded as "key1:value1,key2:value2" (convenience)
	// under "jira.mapping_str" if present.
	if s := viper.GetString("jira.mapping_str"); s != "" {
		for _, pair := range strings.Split(s, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				if k != "" {
					m[k] = v
				}
			}
		}
	}

	return JiraCfg{Mappings: m}
}

// summarizeEntry returns a short human-friendly one-line summary for an entry.
// Used when showing example entries in the consolidated day view.
func summarizeEntry(e Entry) string {
	// Build components, skipping empty pieces.
	parts := []string{}
	if e.Customer != "" {
		parts = append(parts, e.Customer)
	}
	if e.Project != "" {
		parts = append(parts, e.Project)
	}
	if e.Activity != "" {
		parts = append(parts, e.Activity)
	}
	label := strings.Join(parts, "/")
	if label == "" {
		label = "misc"
	}
	// Append duration as HHMM
	min := durationMinutes(e)
	return fmt.Sprintf("%s (%s)", label, fmtHHMM(min))
}
