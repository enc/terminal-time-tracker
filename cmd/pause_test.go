package cmd

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestPauseAtAcceptsRelativeExpressions(t *testing.T) {
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	oldIDGen := IDGen
	defer func() { IDGen = oldIDGen }()

	oldWriter := Writer
	defer func() { Writer = oldWriter }()

	oldPauseAt := pauseAt
	defer func() { pauseAt = oldPauseAt }()
	oldPauseIgnore := pauseIgnoreBudget
	defer func() { pauseIgnoreBudget = oldPauseIgnore }()

	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Reset()
	viper.Set("timezone", "UTC")

	start := Event{
		ID:       "start-1",
		Type:     "start",
		TS:       anchor.Add(-30 * time.Minute),
		Customer: "Acme",
		Project:  "Site",
		Activity: "dev",
		Billable: boolPtr(false),
	}
	writeJournalEvents(t, anchor, []Event{start})

	IDGen = func() string { return "pause-1" }
	fw := &simpleFakeEventWriter{}
	Writer = fw

	pauseAt = "now-5m"
	pauseIgnoreBudget = true

	pauseCmd.Run(&cobra.Command{}, []string{})

	if len(fw.events) == 0 {
		t.Fatalf("no events written")
	}
	ev := fw.events[len(fw.events)-1]
	wantTS := anchor.Add(-5 * time.Minute)
	if !ev.TS.Equal(wantTS) {
		t.Fatalf("timestamp mismatch: got %v want %v", ev.TS, wantTS)
	}
	if ev.Type != "pause" {
		t.Fatalf("expected pause event type, got %q", ev.Type)
	}
	if ev.ID != "pause-1" {
		t.Fatalf("unexpected event ID: got %s want %s", ev.ID, "pause-1")
	}
	if ev.Ref != "start-1" {
		t.Fatalf("unexpected pause ref: got %s want %s", ev.Ref, "start-1")
	}
	if ev.Customer != "Acme" || ev.Project != "Site" || ev.Activity != "dev" {
		t.Fatalf("pause event did not carry entry metadata: %+v", ev)
	}
}
