package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestBuildICSEvents_SplitsAndRoundsAcrossDays(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("tickets", map[string]any{
		"Acme": map[string]any{"Site": "TICKET-42"},
	})
	ticketsCache = nil

	loc := time.UTC
	rounding := Rounding{Strategy: "up", QuantumMin: 15, MinimumEntry: 0}

	start := time.Date(2025, 5, 1, 23, 50, 0, 0, time.UTC)
	end := start.Add(20 * time.Minute) // spans midnight
	entry := Entry{
		ID:       "e1",
		Start:    start,
		End:      &end,
		Customer: "Acme",
		Project:  "Site",
		Activity: "dev",
		Billable: true,
		Notes:    []string{"late work"},
	}

	events, skipped := buildICSEvents([]Entry{entry}, loc, rounding)
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped entries, got %v", skipped)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 day entries, got %d", len(events))
	}

	first := events["2025-05-01"]
	if len(first) != 1 {
		t.Fatalf("expected first day to have 1 event, got %d", len(first))
	}
	if got := first[0].End.Sub(first[0].Start); got != 15*time.Minute {
		t.Fatalf("first segment duration unexpected: %v", got)
	}
	second := events["2025-05-02"]
	if len(second) != 1 {
		t.Fatalf("expected second day to have 1 event, got %d", len(second))
	}
	if got := second[0].End.Sub(second[0].Start); got != 15*time.Minute {
		t.Fatalf("second segment duration unexpected: %v", got)
	}
	if second[0].Summary != "TICKET-42 | late work" {
		t.Fatalf("summary should include ticket and note, got: %s", second[0].Summary)
	}
	if second[0].Description != "TICKET-42 | late work" {
		t.Fatalf("description should include ticket and note, got: %s", second[0].Description)
	}
}

func TestRunExportICS_WritesPerDayFilesWithNotesAndRounding(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("timezone", "UTC")
	viper.Set("rounding.strategy", "up")
	viper.Set("rounding.quantum_min", 15)
	viper.Set("rounding.minimum_billable_min", 0)
	viper.Set("tickets", map[string]any{
		"Acme": map[string]any{"Site": "ABC-123"},
		"Beta": map[string]any{"App": "XYZ-9"},
	})
	ticketsCache = nil

	// entry 1: 10 minutes -> rounded to 15
	day1 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	start1 := day1.Add(9 * time.Hour)
	end1 := start1.Add(10 * time.Minute)
	if err := writeEvent(NewStartEvent("s1", "Acme", "Site", "dev", boolPtr(true), "morning", nil, start1)); err != nil {
		t.Fatalf("write start1 failed: %v", err)
	}
	if err := writeEvent(NewStopEvent("e1", end1)); err != nil {
		t.Fatalf("write stop1 failed: %v", err)
	}

	// entry 2: 30 minutes on next day, keep as-is
	day2 := day1.AddDate(0, 0, 1)
	start2 := day2.Add(10 * time.Hour)
	end2 := start2.Add(30 * time.Minute)
	ev2Start := Event{ID: "start2", Type: "start", TS: start2, Customer: "Beta", Project: "App", Activity: "call", Billable: boolPtr(false), Note: "sync-up"}
	if err := writeEvent(ev2Start); err != nil {
		t.Fatalf("write start2 failed: %v", err)
	}
	if err := writeEvent(NewStopEvent("stop2", end2)); err != nil {
		t.Fatalf("write stop2 failed: %v", err)
	}

	// configure command flags directly
	icsTodayFlag = false
	icsWeekFlag = false
	icsRangeFlag = "2025-03-01T00:00..2025-03-02T23:59"
	outDir := filepath.Join(tmp, "out")
	icsOutDir = outDir
	t.Cleanup(func() {
		icsTodayFlag = false
		icsWeekFlag = false
		icsRangeFlag = ""
		icsOutDir = "."
	})

	buf := &bytes.Buffer{}
	exportIcsCmd.SetOut(buf)
	exportIcsCmd.SetErr(buf)
	if err := exportIcsCmd.RunE(exportIcsCmd, nil); err != nil {
		t.Fatalf("run export ics failed: %v", err)
	}

	path1 := filepath.Join(outDir, "2025-03-01.ics")
	path2 := filepath.Join(outDir, "2025-03-02.ics")
	for _, p := range []string{path1, path2} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file %s to exist: %v", p, err)
		}
	}

	content1, _ := os.ReadFile(path1)
	text1 := unfoldICS(string(content1))
	if !strings.Contains(text1, "SUMMARY:ABC-123 | morning") {
		t.Fatalf("SUMMARY should include ticket and note in day1: %s", text1)
	}
	if !strings.Contains(text1, "DESCRIPTION:ABC-123 | morning") {
		t.Fatalf("DESCRIPTION should include ticket and note in day1: %s", text1)
	}
	if !strings.Contains(text1, "DTEND;TZID=UTC:20250301T091500") {
		t.Fatalf("expected rounded end time in day1, got: %s", text1)
	}

	content2, _ := os.ReadFile(path2)
	text2 := unfoldICS(string(content2))
	if !strings.Contains(text2, "SUMMARY:XYZ-9 | sync-up") {
		t.Fatalf("SUMMARY should include ticket and note in day2: %s", text2)
	}
	if !strings.Contains(text2, "DTSTART;TZID=UTC:20250302T100000") || !strings.Contains(text2, "DTEND;TZID=UTC:20250302T103000") {
		t.Fatalf("start/end mismatch in day2: %s", text2)
	}
	if !strings.Contains(text2, "DESCRIPTION:XYZ-9 | sync-up") {
		t.Fatalf("DESCRIPTION should include ticket and note in day2: %s", text2)
	}
}

func unfoldICS(s string) string {
	s = strings.ReplaceAll(s, "\r\n ", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return s
}
