package cmd

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestParseFlexibleRange_TableDriven(t *testing.T) {
	// Deterministic timezone and Now provider
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 14, 12, 0, 0, 0, time.UTC) // Tuesday 2025-10-14
	oldNow := Now
	defer func() { Now = oldNow }()
	Now = func() time.Time { return anchor }

	tests := []struct {
		name     string
		tokens   []string
		wantSt   time.Time
		wantEn   time.Time
		wantCons int
		wantErr  bool
	}{
		{
			name:     "yesterday with two times",
			tokens:   []string{"yesterday", "09:00", "10:30"},
			wantSt:   time.Date(2025, 10, 13, 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 13, 10, 30, 0, 0, time.UTC),
			wantCons: 3,
		},
		{
			name:     "weekday shorthand (mon) with times",
			tokens:   []string{"mon", "14:00", "15:00"},
			wantSt:   time.Date(2025, 10, 13, 14, 0, 0, 0, time.UTC), // most recent Monday <= anchor
			wantEn:   time.Date(2025, 10, 13, 15, 0, 0, 0, time.UTC),
			wantCons: 3,
		},
		{
			name:     "time-only dash range 9-12",
			tokens:   []string{"9-12"},
			wantSt:   time.Date(2025, 10, 14, 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 14, 12, 0, 0, 0, time.UTC),
			wantCons: 1,
		},
		{
			name:     "time + duration",
			tokens:   []string{"13:00", "+45m"},
			wantSt:   time.Date(2025, 10, 14, 13, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 14, 13, 45, 0, 0, time.UTC),
			wantCons: 2,
		},
		{
			name:     "duration - now (2h-now) -> start = now-2h, end = now",
			tokens:   []string{"2h-now"},
			wantSt:   anchor.Add(-2 * time.Hour),
			wantEn:   anchor,
			wantCons: 1,
		},
		{
			name:     "single time-only token returns start-only",
			tokens:   []string{"14:00"},
			wantSt:   time.Date(2025, 10, 14, 14, 0, 0, 0, time.UTC),
			wantEn:   time.Time{},
			wantCons: 1,
		},
		{
			name:     "full RFC3339 dash range",
			tokens:   []string{"2025-10-10T09:00:00Z-2025-10-10T11:00:00Z"},
			wantSt:   time.Date(2025, 10, 10, 9, 0, 0, 0, time.UTC),
			wantEn:   time.Date(2025, 10, 10, 11, 0, 0, 0, time.UTC),
			wantCons: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, en, consumed, err := ParseFlexibleRange(tc.tokens, Now())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if consumed != tc.wantCons {
				t.Fatalf("consumed: got %d want %d", consumed, tc.wantCons)
			}
			// Compare times by Unix and location/UTC equivalence
			if !st.Equal(tc.wantSt) {
				t.Fatalf("start: got %v (%s) want %v (%s)", st, st.Location(), tc.wantSt, tc.wantSt.Location())
			}
			// If expected end is zero, ensure en.IsZero()
			if tc.wantEn.IsZero() {
				if !en.IsZero() {
					t.Fatalf("end: got %v want <zero>", en)
				}
			} else {
				if en.IsZero() {
					t.Fatalf("end: got <zero> want %v", tc.wantEn)
				}
				if !en.Equal(tc.wantEn) {
					t.Fatalf("end: got %v (%s) want %v (%s)", en, en.Location(), tc.wantEn, tc.wantEn.Location())
				}
			}
		})
	}
}
