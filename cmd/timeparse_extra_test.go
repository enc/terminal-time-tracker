package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestParseFlexibleRange_ExtraCases adds coverage for the remaining smarter time input forms
// such as now-anchored ranges and plain duration tokens.
func TestParseFlexibleRange_ExtraCases(t *testing.T) {
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	tests := []struct {
		name     string
		tokens   []string
		wantSt   time.Time
		wantEn   time.Time
		wantCons int
		wantErr  bool
	}{
		{
			name:     "now-30m single dashed token",
			tokens:   []string{"now-30m"},
			wantSt:   anchor.Add(-30 * time.Minute),
			wantEn:   anchor,
			wantCons: 1,
		},
		{
			name:     "2h-now (duration-left anchored to now)",
			tokens:   []string{"2h-now"},
			wantSt:   anchor.Add(-2 * time.Hour),
			wantEn:   anchor,
			wantCons: 1,
		},
		{
			name:     "start + plain minutes duration (13:00 45)",
			tokens:   []string{"13:00", "45"},
			wantSt:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 13, 0, 0, 0, time.UTC),
			wantEn:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 13, 45, 0, 0, time.UTC),
			wantCons: 2,
		},
		{
			name:   "plain duration token as start-only (+30m) -> interpreted as start relative to Now",
			tokens: []string{"+30m"},
			// Pattern returns start = Now + 30m (parseFlexibleRange treats single duration as start-only fallback)
			wantSt:   anchor.Add(30 * time.Minute),
			wantEn:   time.Time{},
			wantCons: 1,
		},
		{
			name:     "now single token",
			tokens:   []string{"now"},
			wantSt:   anchor,
			wantEn:   time.Time{},
			wantCons: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, en, cons, err := ParseFlexibleRange(tc.tokens, Now())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if cons != tc.wantCons {
				t.Fatalf("consumed tokens mismatch: got %d want %d", cons, tc.wantCons)
			}
			if !st.Equal(tc.wantSt) {
				t.Fatalf("start mismatch: got %v want %v", st, tc.wantSt)
			}
			if tc.wantEn.IsZero() {
				if !en.IsZero() {
					t.Fatalf("expected end to be zero; got %v", en)
				}
			} else {
				if !en.Equal(tc.wantEn) {
					t.Fatalf("end mismatch: got %v want %v", en, tc.wantEn)
				}
			}
		})
	}
}

// TestAddCommand_Integration exercises the `tt add` command end-to-end with the
// flexible parsing. Uses a fake writer to capture events rather than writing to disk.
func TestAddCommand_Integration(t *testing.T) {
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	// deterministic id generator
	oldID := IDGen
	IDGen = func() string { return "evt-add-test-1" }
	defer func() { IDGen = oldID }()

	// replace Writer with capture writer
	cw := &fakeWriter{}
	oldWriter := Writer
	Writer = cw
	defer func() { Writer = oldWriter }()

	tests := []struct {
		name     string
		args     []string
		wantSt   time.Time
		wantEn   time.Time
		wantCust string
		wantProj string
	}{
		{
			name:     "time-only dash range 9-12 with customer/project",
			args:     []string{"9-12", "Acme", "Portal"},
			wantSt:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 12, 0, 0, 0, time.UTC),
			wantCust: "Acme",
			wantProj: "Portal",
		},
		{
			name:     "time + duration (13:00 +45m) with customer/project",
			args:     []string{"13:00", "+45m", "ACME", "Mobilize"},
			wantSt:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 13, 0, 0, 0, time.UTC),
			wantEn:   time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 13, 45, 0, 0, time.UTC),
			wantCust: "ACME",
			wantProj: "Mobilize",
		},
		{
			name:     "now-anchored range now-30m",
			args:     []string{"now-30m", "ClientX", "ProjY"},
			wantSt:   anchor.Add(-30 * time.Minute),
			wantEn:   anchor,
			wantCust: "ClientX",
			wantProj: "ProjY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// reset capture writer events
			cw.events = nil

			// run add command
			// Use an empty *cobra.Command for signature - addCmd doesn't inspect it.
			addCmd.Run(&cobra.Command{}, tc.args)

			// Expect one add event to have been written
			if len(cw.events) == 0 {
				t.Fatalf("no events written by add command")
			}
			// find the add event (there may be other events in writer in other tests)
			var ev Event
			found := false
			for i := range cw.events {
				if cw.events[i].Type == "add" {
					ev = cw.events[i]
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("no add event written; events=%+v", cw.events)
			}

			// Ref should be "<startRFC3339>..<endRFC3339>"
			parts := strings.Split(ev.Ref, "..")
			if len(parts) != 2 {
				t.Fatalf("unexpected Ref format: %q", ev.Ref)
			}
			gotSt, err := time.Parse(time.RFC3339, parts[0])
			if err != nil {
				t.Fatalf("parse start from ref failed: %v", err)
			}
			gotEn, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				t.Fatalf("parse end from ref failed: %v", err)
			}

			if !gotSt.Equal(tc.wantSt) {
				t.Fatalf("start mismatch: got %v want %v", gotSt, tc.wantSt)
			}
			if !gotEn.Equal(tc.wantEn) {
				t.Fatalf("end mismatch: got %v want %v", gotEn, tc.wantEn)
			}
			if ev.Customer != tc.wantCust {
				t.Fatalf("customer mismatch: got %q want %q", ev.Customer, tc.wantCust)
			}
			if ev.Project != tc.wantProj {
				t.Fatalf("project mismatch: got %q want %q", ev.Project, tc.wantProj)
			}
		})
	}
}
