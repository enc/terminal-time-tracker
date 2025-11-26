package cmd

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestBudgetUsageAndChecks(t *testing.T) {
	viper.Set("timezone", "UTC")
	round := Rounding{Strategy: "up", QuantumMin: 15}

	b := Budget{ID: "b1", Customer: "ACME", Project: "Web", Hours: 2}
	br := toRuntimeBudgets([]Budget{b})[0]

	start := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(60 * time.Minute)
	entries := []Entry{{ID: "e1", Customer: "ACME", Project: "Web", Activity: "dev", Start: start, End: &end, Billable: true}}

	used := budgetUsage(br, entries, round, end, nil, nil)
	if used != 60 {
		t.Fatalf("expected used=60, got %d", used)
	}

	// Add a 90m candidate; projected 150m exceeds 120m target -> warn
	endCand := start.Add(90 * time.Minute)
	cand := budgetCandidate{Customer: "ACME", Project: "Web", Activity: "dev", Start: start, End: &endCand}
	warns, err := checkBudgets([]budgetRuntime{br}, entries, cand, round, end)
	if err != nil {
		t.Fatalf("unexpected block: %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected warning when projected exceeds target")
	}

	// enforce=block should block when projected exceeds target
	b.Enforce = "block"
	brBlock := toRuntimeBudgets([]Budget{b})[0]
	_, err = checkBudgets([]budgetRuntime{brBlock}, entries, cand, round, end)
	if err == nil {
		t.Fatalf("expected block error when enforce=block and projected exceeds target")
	}
}

func TestBudgetWindowFiltering(t *testing.T) {
	viper.Set("timezone", "UTC")
	round := Rounding{Strategy: "up", QuantumMin: 15}

	start := time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	b := Budget{ID: "b1", Customer: "ACME", Hours: 1, StartsAt: start.Add(-time.Hour).Format(time.RFC3339), EndsAt: start.Add(time.Hour).Format(time.RFC3339)}
	br := toRuntimeBudgets([]Budget{b})[0]

	entries := []Entry{{ID: "e1", Customer: "ACME", Start: start, End: &end}}
	used := budgetUsage(br, entries, round, end, nil, nil)
	if used != 30 {
		t.Fatalf("expected 30m used inside window, got %d", used)
	}

	// candidate outside window should not be considered matching
	cand := budgetCandidate{Customer: "ACME", Start: start.Add(48 * time.Hour)}
	if budgetWindowOverlaps(br, cand.Start, candEndOrStart(cand)) {
		t.Fatalf("expected no overlap for candidate outside budget window")
	}
}
