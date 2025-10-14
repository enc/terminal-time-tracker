package cmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestSwitchAtAcceptsRelativeExpressions verifies that `switch --at` accepts flexible/relative
// expressions such as "now-5m", "+2m", time-of-day like "09:00", and absolute RFC3339 timestamps.
// The test is table-driven and asserts that a stop event and a start event are written with the
// same timestamp produced from the `--at` value.
func TestSwitchAtAcceptsRelativeExpressions(t *testing.T) {
	// Deterministic timezone and Now provider
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 21, 15, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	// Backup global providers and writer
	oldIDGen := IDGen
	defer func() { IDGen = oldIDGen }()
	oldWriter := Writer
	defer func() { Writer = oldWriter }()

	// Backup and restore switch-related globals/flags
	oldAt := switchAt
	oldActivity := switchActivity
	oldBillable := switchBillable
	oldTags := switchTags
	oldNote := switchNote
	defer func() {
		switchAt = oldAt
		switchActivity = oldActivity
		switchBillable = oldBillable
		switchTags = oldTags
		switchNote = oldNote
	}()

	// Default values to assert are forwarded to start event
	switchActivity = "ops"
	switchBillable = true
	switchTags = nil
	switchNote = "switch at test"

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
			name:   "plus-duration (+2m) -> anchor + dur",
			at:     "+2m",
			wantTS: anchor.Add(2 * time.Minute),
		},
		{
			name:   "time-of-day (09:00) -> same day 09:00 UTC",
			at:     "09:00",
			wantTS: time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 9, 0, 0, 0, time.UTC),
		},
		{
			name:   "absolute RFC3339 timestamp",
			at:     "2025-10-21T07:30:00Z",
			wantTS: time.Date(2025, 10, 21, 7, 30, 0, 0, time.UTC),
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Per-case deterministic ID generator so the written events have predictable IDs.
			IDGen = func() string { return fmt.Sprintf("evt-switch-%d", i) }

			// Use fake writer to capture events
			fw := &simpleFakeEventWriter{}
			Writer = fw

			// Set the --at value
			switchAt = tc.at

			// Run command (no positional args)
			switchCmd.Run(&cobra.Command{}, []string{})

			// Expect at least two events: stop then start
			if len(fw.events) < 2 {
				t.Fatalf("expected at least 2 events (stop,start), got %d for case %q", len(fw.events), tc.name)
			}
			stopEv := fw.events[0]
			startEv := fw.events[1]

			// Check types
			if stopEv.Type != "stop" {
				t.Fatalf("expected first event type=stop, got %q", stopEv.Type)
			}
			if startEv.Type != "start" {
				t.Fatalf("expected second event type=start, got %q", startEv.Type)
			}

			// Timestamp equality: both stop and start should use the parsed ts
			if !stopEv.TS.Equal(tc.wantTS) {
				t.Fatalf("stop timestamp mismatch for %q: got %v want %v", tc.name, stopEv.TS, tc.wantTS)
			}
			if !startEv.TS.Equal(tc.wantTS) {
				t.Fatalf("start timestamp mismatch for %q: got %v want %v", tc.name, startEv.TS, tc.wantTS)
			}

			// Ensure start event preserved flags/overrides
			if startEv.Activity != switchActivity {
				t.Fatalf("start event activity mismatch: got %q want %q", startEv.Activity, switchActivity)
			}
			if startEv.Billable == nil || *startEv.Billable != switchBillable {
				t.Fatalf("start event billable mismatch: got %+v want %v", startEv.Billable, switchBillable)
			}
			// Note and tags are forwarded/copied by NewStartEvent usage in the command
			if startEv.Note != switchNote {
				t.Fatalf("start event note mismatch: got %q want %q", startEv.Note, switchNote)
			}
		})
	}
}
