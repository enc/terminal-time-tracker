package cmd

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestMustParseTimeLocal_Formats(t *testing.T) {
	// Use a deterministic timezone for test expectations
	viper.Set("timezone", "UTC")
	loc := time.UTC

	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			name:  "RFC3339 with offset",
			input: "2025-10-08T14:30:00+02:00",
			// parse expected with standard parser so offset handling is validated
			expected: func() time.Time {
				tt, err := time.Parse(time.RFC3339, "2025-10-08T14:30:00+02:00")
				if err != nil {
					t.Fatalf("failed preparing expected time: %v", err)
				}
				return tt
			}(),
		},
		{
			name:  "Date+time T without seconds (assume timezone)",
			input: "2026-04-05T09:00",
			expected: time.Date(
				2026, time.April, 5, 9, 0, 0, 0, loc,
			),
		},
		{
			name:  "Space-separated date+time with seconds",
			input: "2026-04-05 09:00:30",
			expected: time.Date(
				2026, time.April, 5, 9, 0, 30, 0, loc,
			),
		},
		{
			name:  "Space-separated date+time without seconds",
			input: "2026-04-05 09:00",
			expected: time.Date(
				2026, time.April, 5, 9, 0, 0, 0, loc,
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustParseTimeLocal(tc.input)
			// For RFC3339 comparison we want to assert instants are equal.
			if !got.Equal(tc.expected) {
				t.Fatalf("input %q: got %v (%s), want %v (%s)", tc.input, got, got.Location(), tc.expected, tc.expected.Location())
			}
		})
	}

	// Time-only tests (assume today's date in configured timezone)
	t.Run("Time-only HH:MM", func(t *testing.T) {
		viper.Set("timezone", "UTC")
		loc := time.UTC
		now := time.Now().In(loc)
		input := "14:30"
		got := mustParseTimeLocal(input)
		want := time.Date(now.Year(), now.Month(), now.Day(), 14, 30, 0, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("input %q: got %v, want %v", input, got, want)
		}
	})

	t.Run("Time-only HH:MM:SS", func(t *testing.T) {
		viper.Set("timezone", "UTC")
		loc := time.UTC
		now := time.Now().In(loc)
		input := "14:30:15"
		got := mustParseTimeLocal(input)
		want := time.Date(now.Year(), now.Month(), now.Day(), 14, 30, 15, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("input %q: got %v, want %v", input, got, want)
		}
	})
}
