package cmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Tests for `stop --at` accepting flexible/relative expressions.
func TestStopAtAcceptsRelativeExpressions(t *testing.T) {
	// Deterministic timezone and Now provider
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	// Deterministic ID generator so events are predictable per case.
	oldIDGen := IDGen
	defer func() { IDGen = oldIDGen }()

	// Backup and restore global Writer
	oldWriter := Writer
	defer func() { Writer = oldWriter }()

	// Backup and restore stopAt global
	oldStopAt := stopAt
	defer func() { stopAt = oldStopAt }()

	cases := []struct {
		name   string
		at     string
		wantTS time.Time
	}{
		{
			name:   "now-anchored past (now-5m)",
			at:     "now-5m",
			wantTS: anchor.Add(-5 * time.Minute),
		},
		{
			name:   "plus-duration (+5m) -> anchor + dur",
			at:     "+5m",
			wantTS: anchor.Add(5 * time.Minute),
		},
		{
			name:   "plain-duration (5m) -> anchor + dur (legacy behaviour of ParseFlexibleRange)",
			at:     "5m",
			wantTS: anchor.Add(5 * time.Minute),
		},
		{
			name:   "time-of-day (09:15) -> same day 09:15 UTC",
			at:     "09:15",
			wantTS: time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 9, 15, 0, 0, time.UTC),
		},
		{
			name:   "absolute RFC3339 timestamp",
			at:     "2025-10-20T08:00:00Z",
			wantTS: time.Date(2025, 10, 20, 8, 0, 0, 0, time.UTC),
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Per-case deterministic ID
			IDGen = func() string { return fmt.Sprintf("evt-stop-test-%d", i) }

			// Fake writer captures the written event
			fw := &simpleFakeEventWriter{}
			Writer = fw

			// Set the flag value
			stopAt = tc.at

			// Run the command (no positional args)
			stopCmd.Run(&cobra.Command{}, []string{})

			// We expect exactly one event written
			if len(fw.events) == 0 {
				t.Fatalf("no events written for case %q", tc.name)
			}
			ev := fw.events[len(fw.events)-1]

			// Compare instants: because Now is deterministic, the parsed times should match exactly.
			if !ev.TS.Equal(tc.wantTS) {
				t.Fatalf("timestamp mismatch for %q: got %v (loc %v) want %v (loc %v)",
					tc.name, ev.TS, ev.TS.Location(), tc.wantTS, tc.wantTS.Location())
			}

			// Basic sanity: event should be of type stop and have the expected ID
			if ev.Type != "stop" {
				t.Fatalf("expected stop event type, got %q", ev.Type)
			}
			expectedID := fmt.Sprintf("evt-stop-test-%d", i)
			if ev.ID != expectedID {
				t.Fatalf("unexpected event ID: got %s want %s", ev.ID, expectedID)
			}
		})
	}
}
