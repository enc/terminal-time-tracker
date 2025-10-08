package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestJournalPathAndDir(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	// Choose a deterministic date
	d := time.Date(2025, 3, 14, 9, 0, 0, 0, time.UTC)
	dir := journalDirFor(d)
	if !filepath.HasPrefix(dir, tmp) {
		t.Fatalf("journalDirFor returned dir outside HOME: %s", dir)
	}
	// journalPathFor should create the directory and return a path ending with date.jsonl
	p := journalPathFor(d)
	if !filepath.HasPrefix(p, tmp) {
		t.Fatalf("journalPathFor returned path outside HOME: %s", p)
	}
	if filepath.Ext(p) != ".jsonl" {
		t.Fatalf("journalPathFor expected .jsonl extension, got: %s", p)
	}
	// Directory should exist
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("journal dir not created: %v", err)
	}
}

func TestWriteEventAndReadLastHash(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Set("timezone", "UTC")

	day := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	start := day.Add(9 * time.Hour)

	ev := Event{
		ID:       "test1",
		Type:     "start",
		TS:       start,
		Customer: "ACME",
		Project:  "P",
		Activity: "dev",
		Billable: boolPtr(true),
		Note:     "testing",
	}

	if err := writeEvent(ev); err != nil {
		t.Fatalf("writeEvent failed: %v", err)
	}

	// Journal file should exist and contain the event JSON line
	p := journalPathFor(start)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("failed to read journal file: %v", err)
	}
	lines := bytesTrimSplitLines(b)
	if len(lines) == 0 {
		t.Fatalf("journal file empty")
	}
	var got Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}
	if got.Type != ev.Type || got.ID != ev.ID || got.Note != ev.Note {
		t.Fatalf("written event does not match; got=%+v want subset=%+v", got, ev)
	}

	// readLastHash should return the hash that was stored
	h := readLastHash(p)
	if h == "" {
		t.Fatalf("readLastHash returned empty")
	}
	if h != got.Hash {
		t.Fatalf("readLastHash %q does not match event.Hash %q", h, got.Hash)
	}
}

func TestLoadEntries_Reconstruction(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Set("timezone", "UTC")

	day := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	start1 := day.Add(9 * time.Hour)                // 09:00
	stop1 := day.Add(10*time.Hour + 30*time.Minute) // 10:30 => 90m
	start2 := day.Add(14 * time.Hour)               // 14:00
	stop2 := day.Add(15*time.Hour + 15*time.Minute) // 15:15 => 75m

	// start/stop pair
	ev1 := Event{ID: "e1", Type: "start", TS: start1, Customer: "Acme", Project: "Website", Activity: "dev", Billable: boolPtr(true), Note: "coding"}
	if err := writeEvent(ev1); err != nil {
		t.Fatalf("writeEvent ev1 failed: %v", err)
	}
	ev2 := Event{ID: "e2", Type: "stop", TS: stop1}
	if err := writeEvent(ev2); err != nil {
		t.Fatalf("writeEvent ev2 failed: %v", err)
	}

	// add entry
	ev3 := Event{ID: "e3", Type: "add", TS: start2, Customer: "Acme", Project: "Website", Activity: "meeting", Billable: boolPtr(true), Note: "sync", Ref: start2.Format(time.RFC3339) + ".." + stop2.Format(time.RFC3339)}
	if err := writeEvent(ev3); err != nil {
		t.Fatalf("writeEvent ev3 failed: %v", err)
	}

	from, to := day, day
	entries, err := loadEntries(from, to)
	if err != nil {
		t.Fatalf("loadEntries returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	mins := []int{durationMinutes(entries[0]), durationMinutes(entries[1])}
	expected := []int{90, 75}
	if !reflect.DeepEqual(mins, expected) && !reflect.DeepEqual(reverseIntSlice(mins), expected) {
		t.Fatalf("durations mismatch; got %v want %v", mins, expected)
	}
}

func TestDurationAndFmtHHMM(t *testing.T) {
	start := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(2*time.Hour + 45*time.Minute)
	e := Entry{Start: start, End: &end}
	if d := durationMinutes(e); d != 165 {
		t.Fatalf("durationMinutes expected 165 got %d", d)
	}
	if f := fmtHHMM(165); f != "2h45m" {
		t.Fatalf("fmtHHMM expected 2h45m got %s", f)
	}
	// nil end -> 0
	e2 := Entry{Start: start, End: nil}
	if d := durationMinutes(e2); d != 0 {
		t.Fatalf("durationMinutes expected 0 for running got %d", d)
	}
}

func TestRoundMinutesVarieties(t *testing.T) {
	// up strategy
	r := Rounding{Strategy: "up", QuantumMin: 15, MinimumEntry: 0}
	if got := roundMinutes(7, r); got != 15 {
		t.Fatalf("round up expected 15 got %d", got)
	}
	// down strategy
	r.Strategy = "down"
	if got := roundMinutes(29, r); got != 15 {
		t.Fatalf("round down expected 15 got %d", got)
	}
	// nearest strategy
	r.Strategy = "nearest"
	if got := roundMinutes(22, r); got != 30 {
		t.Fatalf("round nearest expected 30 got %d", got)
	}
	// quantum zero fallback to 15
	r.QuantumMin = 0
	r.Strategy = "up"
	if got := roundMinutes(16, r); got != 30 {
		t.Fatalf("quantum fallback expected 30 got %d", got)
	}
	// minimum entry enforces floor
	r = Rounding{Strategy: "up", QuantumMin: 15, MinimumEntry: 60}
	if got := roundMinutes(10, r); got != 60 {
		t.Fatalf("minimum entry expected 60 got %d", got)
	}
}

func TestGetRounding_FromViper(t *testing.T) {
	viper.Set("rounding.quantum_min", 10)
	viper.Set("rounding.minimum_billable_min", 20)
	viper.Set("rounding.strategy", "down")
	r := getRounding()
	if r.QuantumMin != 10 || r.MinimumEntry != 20 || r.Strategy != "down" {
		t.Fatalf("getRounding returned unexpected values: %+v", r)
	}
}

// Helper utilities for tests

func bytesTrimSplitLines(b []byte) []string {
	// Portable split into non-empty trimmed lines
	out := []string{}
	start := 0
	for i, c := range b {
		if c == '\n' {
			line := string(b[start:i])
			start = i + 1
			if s := trimSpaces(line); s != "" {
				out = append(out, s)
			}
		}
	}
	// trailing partial line
	if start < len(b) {
		if s := trimSpaces(string(b[start:])); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func trimSpaces(s string) string {
	return string([]byte(s))
}

func reverseIntSlice(s []int) []int {
	out := make([]int, len(s))
	for i := range s {
		out[len(s)-1-i] = s[i]
	}
	return out
}
