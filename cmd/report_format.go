package cmd

import (
	"fmt"
	"sort"
	"strings"
)

// formatGroups renders aggregated groups in a compact, week-like table style and returns the string.
// It intentionally uses the subtle ANSI palette defined elsewhere in the package for consistent styling.
func formatGroups(agg map[aggKey]*aggVal, groupEntries map[aggKey][]Entry, detailed bool) string {
	var b strings.Builder

	const labelW = 30
	const hoursW = 7

	// colors are package-level constants (defined in report_week.go). Use them directly.
	reset := ansiReset
	heading := ansiHeading
	labelCol := ansiLabel
	hoursCol := ansiHours
	notesCol := ansiNotes

	// Sort keys for deterministic output
	keys := make([]aggKey, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Customer != keys[j].Customer {
			return keys[i].Customer < keys[j].Customer
		}
		if keys[i].Project != keys[j].Project {
			return keys[i].Project < keys[j].Project
		}
		return keys[i].Activity < keys[j].Activity
	})

	for _, k := range keys {
		v := agg[k]

		// Build display name
		name := k.Customer
		if k.Project != "" {
			name = fmt.Sprintf("%s / %s", k.Customer, k.Project)
		}
		if name == "" {
			name = "(unknown)"
		}
		if len(name) > labelW-2 {
			name = name[:labelW-5] + "..."
		}

		// Hours (raw minutes -> hours)
		hours := float64(v.RawMin) / 60.0

		// Main line: label (colored) + hours (colored)
		// Example: "  ACME / WebApp               3.50h"
		b.WriteString(fmt.Sprintf("  %s%-*s%s %s%*.2fh%s\n", labelCol, labelW, name, reset, hoursCol, hoursW, hours, reset))

		// Notes: either detailed per-entry or merged
		if detailed {
			entries := groupEntries[k]
			sort.Slice(entries, func(i, j int) bool { return entries[i].Start.Before(entries[j].Start) })
			for _, e := range entries {
				if len(e.Notes) == 0 {
					continue
				}
				for _, n := range e.Notes {
					norm := normalizeNote(n)
					if norm == "" {
						continue
					}
					b.WriteString(fmt.Sprintf("    %s- %s%s\n", notesCol, norm, reset))
				}
			}
		} else {
			entries := groupEntries[k]
			notes := []string{}
			for _, e := range entries {
				for _, n := range e.Notes {
					norm := normalizeNote(n)
					if norm != "" {
						notes = append(notes, norm)
					}
				}
			}
			notes = dedupeStrings(notes)
			merged := mergeNotesForDisplay(notes, 80)
			if merged != "" {
				// put merged notes on next indented line
				b.WriteString(fmt.Sprintf("    %s- %s%s\n", notesCol, merged, reset))
			}
		}

		// Totals line for the group — align totals under the hours column.
		// We print the label padded to the same `labelW` then emit the raw/rounded values
		// starting at the same column where hours appear above.
		b.WriteString(fmt.Sprintf("  %s%-*s%s %sRaw=%s Rounded=%s (+%dm)\n\n",
			heading, labelW, "Group total:", reset, hoursCol, fmtHHMM(v.RawMin), fmtHHMM(v.RoundedMin), v.RoundedMin-v.RawMin))
	}

	return b.String()
}
