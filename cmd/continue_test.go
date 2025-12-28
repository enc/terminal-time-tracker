package cmd

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestContinueUsesLastPauseEvent(t *testing.T) {
	anchor := time.Date(2025, 11, 2, 15, 0, 0, 0, time.UTC)
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	oldIDGen := IDGen
	defer func() { IDGen = oldIDGen }()

	oldWriter := Writer
	defer func() { Writer = oldWriter }()

	oldContinueAt := continueAt
	defer func() { continueAt = oldContinueAt }()
	oldContinueIgnore := continueIgnoreBudget
	defer func() { continueIgnoreBudget = oldContinueIgnore }()

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
		TS:       anchor.Add(-40 * time.Minute),
		Customer: "Acme",
		Project:  "Site",
		Activity: "dev",
		Billable: boolPtr(false),
		Tags:     []string{"cli"},
	}
	pause := Event{
		ID:       "pause-1",
		Type:     "pause",
		TS:       anchor.Add(-10 * time.Minute),
		Customer: "Acme",
		Project:  "Site",
		Activity: "dev",
		Billable: boolPtr(false),
		Tags:     []string{"cli"},
		Ref:      "start-1",
	}
	writeJournalEvents(t, anchor, []Event{start, pause})

	IDGen = func() string { return "resume-1" }
	fw := &simpleFakeEventWriter{}
	Writer = fw

	continueAt = ""
	continueIgnoreBudget = true

	continueCmd.Run(&cobra.Command{}, []string{})

	if len(fw.events) == 0 {
		t.Fatalf("no events written")
	}
	ev := fw.events[len(fw.events)-1]
	if ev.Type != "resume" {
		t.Fatalf("expected resume event type, got %q", ev.Type)
	}
	if ev.ID != "resume-1" {
		t.Fatalf("unexpected event ID: got %s want %s", ev.ID, "resume-1")
	}
	if ev.Ref != "start-1" {
		t.Fatalf("unexpected resume ref: got %s want %s", ev.Ref, "start-1")
	}
	if ev.Customer != "Acme" || ev.Project != "Site" || ev.Activity != "dev" {
		t.Fatalf("resume event did not carry entry metadata: %+v", ev)
	}
	if ev.Billable == nil || *ev.Billable != false {
		t.Fatalf("resume event billable mismatch: %+v", ev.Billable)
	}
	if !ev.TS.Equal(anchor) {
		t.Fatalf("resume timestamp mismatch: got %v want %v", ev.TS, anchor)
	}
}
