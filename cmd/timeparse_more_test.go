package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// fakeWriter captures events written by commands for assertions in tests.
type fakeWriter struct {
	events []Event
	err    error
}

func (f *fakeWriter) WriteEvent(e Event) error {
	if f.err != nil {
		return f.err
	}
	// shallow copy is fine for tests
	f.events = append(f.events, e)
	return nil
}

// TestParseFlexibleRange_MoreCases covers the various smarter time-input forms described in the UX:
// - relative date/time: "yesterday 09:00 10:30"
// - weekday shorthands: "mon 14:00 15:00"
// - ranges with dash and time-only: "9-12"
// - durations and anchors: "13:00 +45m"
// - now-anchors: "now-30m", "2h-now", "now-30m now" combos
func TestParseFlexibleRange_MoreCases(t *testing.T) {
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 14, 12, 0, 0, 0, time.UTC) // Tuesday
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	cases := []struct {
		name     string
		tokens   []string
		wantSt   time.Time
		wantEn   time.Time
		wantCons int
	}{
		{
			name:     "relative yesterday with times",
			tokens:   []string{"yesterday", "09:00", "10:30"},
			wantSt:   time.Date(2025, 10, 13, 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 13, 10, 30, 0, 0, time.UTC),
			wantCons: 3,
		},
		{
			name:     "weekday shorthand mon with times",
			tokens:   []string{"mon", "14:00", "15:00"},
			wantSt:   time.Date(2025, 10, 13, 14, 0, 0, 0, time.UTC), // Monday before anchor
			wantEn:   time.Date(2025, 10, 13, 15, 0, 0, 0, time.UTC),
			wantCons: 3,
		},
		{
			name:     "time-only dash 9-12",
			tokens:   []string{"9-12"},
			wantSt:   time.Date(2025, 10, 14, 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 14, 12, 0, 0, 0, time.UTC),
			wantCons: 1,
		},
		{
			name:     "time plus duration 13:00 +45m",
			tokens:   []string{"13:00", "+45m"},
			wantSt:   time.Date(2025, 10, 14, 13, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 14, 13, 45, 0, 0, time.UTC),
			wantCons: 2,
		},
		{
			name:     "now-anchored range now-30m",
			tokens:   []string{"now-30m"},
			wantSt:   anchor.Add(-30 * time.Minute),
			wantEn:   anchor,
			wantCons: 1,
		},
		{
			name:     "duration-left anchored to now 2h-now",
			tokens:   []string{"2h-now"},
			wantSt:   anchor.Add(-2 * time.Hour),
			wantEn:   anchor,
			wantCons: 1,
		},
		{
			name:     "single now token",
			tokens:   []string{"now"},
			wantSt:   anchor,
			wantEn:   time.Time{},
			wantCons: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			st, en, cons, err := ParseFlexibleRange(tc.tokens, Now())
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if cons != tc.wantCons {
				t.Fatalf("consumed tokens: got %d want %d", cons, tc.wantCons)
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

// TestAddCommand_YesterdayRange verifies addCmd supports the multi-token range form:
// `tt add yesterday 09:00 10:30 acme portal` and that it yields an `add` event with the correct Ref,
// Customer and Project. Uses a fake Writer to capture the generated event.
func TestAddCommand_YesterdayRange(t *testing.T) {
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 14, 12, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	// deterministic ID for test readability
	oldID := IDGen
	IDGen = func() string { return "evt-add-yesterday-1" }
	defer func() { IDGen = oldID }()

	// Replace global Writer with fake writer and restore afterwards
	fw := &fakeWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// Prepare args matching the requested UX: yesterday 09:00 10:30 acme portal
	args := []string{"yesterday", "09:00", "10:30", "acme", "portal"}

	// Execute the add command (we pass a *cobra.Command because the signature requires it)
	addCmd.Run(&cobra.Command{}, args)

	// Ensure an add event was written
	if len(fw.events) == 0 {
		t.Fatalf("no events written by add command")
	}
	var addEv *Event
	for i := range fw.events {
		if fw.events[i].Type == "add" {
			// take the first add event
			addEv = &fw.events[i]
			break
		}
	}
	if addEv == nil {
		t.Fatalf("no add event found; events=%+v", fw.events)
	}

	// Verify customer/project
	if addEv.Customer != "acme" {
		t.Fatalf("customer mismatch: got %q want %q", addEv.Customer, "acme")
	}
	if addEv.Project != "portal" {
		t.Fatalf("project mismatch: got %q want %q", addEv.Project, "portal")
	}

	// Verify Ref encodes the expected start and end times (RFC3339)
	parts := strings.Split(addEv.Ref, "..")
	if len(parts) != 2 {
		t.Fatalf("unexpected Ref format: %q", addEv.Ref)
	}
	gotSt, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		t.Fatalf("parse start from ref failed: %v", err)
	}
	gotEn, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		t.Fatalf("parse end from ref failed: %v", err)
	}

	wantSt := time.Date(2025, 10, 13, 9, 0, 0, 0, time.UTC)   // yesterday at 09:00
	wantEn := time.Date(2025, 10, 13, 10, 30, 0, 0, time.UTC) // yesterday at 10:30

	if !gotSt.Equal(wantSt) {
		t.Fatalf("start in ref mismatch: got %v want %v", gotSt, wantSt)
	}
	if !gotEn.Equal(wantEn) {
		t.Fatalf("end in ref mismatch: got %v want %v", gotEn, wantEn)
	}
}
