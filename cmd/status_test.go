package cmd

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestFindActiveAndLast_UsesCorrections(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("timezone", "UTC")

	day := time.Date(2025, 4, 12, 0, 0, 0, 0, time.UTC)
	start := day.Add(9 * time.Hour)
	stop := day.Add(10 * time.Hour)

	startID := "s1"
	if err := writeEvent(Event{
		ID:       startID,
		Type:     "start",
		TS:       start,
		Customer: "ACME",
		Project:  "Portal",
		Activity: "dev",
		Billable: boolPtr(true),
	}); err != nil {
		t.Fatalf("writeEvent start failed: %v", err)
	}
	if err := writeEvent(Event{
		ID:   "st1",
		Type: "stop",
		TS:   stop,
	}); err != nil {
		t.Fatalf("writeEvent stop failed: %v", err)
	}
	if err := writeEvent(Event{
		ID:       "a1",
		Type:     "amend",
		TS:       day.Add(11 * time.Hour),
		Ref:      startID,
		Activity: "review",
	}); err != nil {
		t.Fatalf("writeEvent amend failed: %v", err)
	}

	if err := writeEvent(Event{
		ID:       "s2",
		Type:     "start",
		TS:       day.Add(12 * time.Hour),
		Customer: "ACME",
		Project:  "Portal",
		Activity: "support",
		Billable: boolPtr(true),
	}); err != nil {
		t.Fatalf("writeEvent active start failed: %v", err)
	}

	active, last, err := findActiveAndLast(day, day)
	if err != nil {
		t.Fatalf("findActiveAndLast returned error: %v", err)
	}
	if active == nil || active.ID != "s2" {
		t.Fatalf("expected active entry s2, got %#v", active)
	}
	if last == nil {
		t.Fatalf("expected last closed entry, got nil")
	}
	if last.Activity != "review" {
		t.Fatalf("expected amended activity 'review', got %q", last.Activity)
	}
	if last.End == nil || !last.End.Equal(stop) {
		t.Fatalf("expected last end %v, got %v", stop, last.End)
	}
}
