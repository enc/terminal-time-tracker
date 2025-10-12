package cmd

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// helper to strip ANSI escape sequences for predictable assertions
func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

// helper to check that all substrings exist in s
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestFormatGroups_TableDriven(t *testing.T) {
	// common timestamps
	loc := time.UTC
	start1 := time.Date(2025, 10, 6, 8, 30, 0, 0, loc)
	end1 := time.Date(2025, 10, 6, 12, 0, 0, 0, loc) // 3.5h -> 210min

	start2 := time.Date(2025, 10, 7, 9, 0, 0, 0, loc)
	end2 := time.Date(2025, 10, 7, 10, 0, 0, 0, loc) // 1h -> 60min

	tests := []struct {
		name         string
		agg          map[aggKey]*aggVal
		groupEntries map[aggKey][]Entry
		detailed     bool
		expecteds    []string
		unexpected   []string
	}{
		{
			name: "concise single group shows label hours and merged note and group totals",
			agg: map[aggKey]*aggVal{
				{Customer: "ACME", Project: "WebApp"}: {RawMin: 210, RoundedMin: 210},
			},
			groupEntries: map[aggKey][]Entry{
				{Customer: "ACME", Project: "WebApp"}: {
					{
						ID:       "e1",
						Start:    start1,
						End:      &end1,
						Customer: "ACME",
						Project:  "WebApp",
						Notes:    []string{"API scaffolding"},
					},
				},
			},
			detailed:   false,
			expecteds:  []string{"ACME / WebApp", "3.50h", "Raw=3h30m", "API scaffolding"},
			unexpected: []string{},
		},
		{
			name: "detailed prints per-entry notes and group totals",
			agg: map[aggKey]*aggVal{
				{Customer: "ACME", Project: "WebApp"}: {RawMin: 210, RoundedMin: 210},
			},
			groupEntries: map[aggKey][]Entry{
				{Customer: "ACME", Project: "WebApp"}: {
					{
						ID:       "e1",
						Start:    start1,
						End:      &end1,
						Customer: "ACME",
						Project:  "WebApp",
						Notes:    []string{"API scaffolding", "Standup + deploy"},
					},
				},
			},
			detailed:   true,
			expecteds:  []string{"ACME / WebApp", "- API scaffolding", "- Standup + deploy", "Group total:", "Raw=3h30m"},
			unexpected: []string{},
		},
		{
			name: "unknown customer label for empty customer",
			agg: map[aggKey]*aggVal{
				{Customer: "", Project: ""}: {RawMin: 60, RoundedMin: 60},
			},
			groupEntries: map[aggKey][]Entry{
				{Customer: "", Project: ""}: {
					{
						ID:       "e2",
						Start:    start2,
						End:      &end2,
						Customer: "",
						Project:  "",
						Notes:    []string{"Email backlog"},
					},
				},
			},
			detailed:   false,
			expecteds:  []string{"(unknown)", "1.00h", "Raw=1h00m", "Email backlog"},
			unexpected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := formatGroups(tc.agg, tc.groupEntries, tc.detailed)
			clean := stripANSI(out)

			// Check expected substrings
			for _, ex := range tc.expecteds {
				if !strings.Contains(clean, ex) {
					t.Fatalf("output did not contain expected %q\nOutput:\n%s", ex, clean)
				}
			}
			// Ensure unexpected substrings are not present
			for _, ux := range tc.unexpected {
				if strings.Contains(clean, ux) {
					t.Fatalf("output contained unexpected %q\nOutput:\n%s", ux, clean)
				}
			}
		})
	}
}
