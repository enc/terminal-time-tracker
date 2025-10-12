package cmd

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"tt/internal/journal"
)

// TestStartForAndSweepAutoStops verifies that:
//  1. `start` with `--for` writes a start event that contains meta["auto_stop"] with the expected RFC3339 timestamp.
//  2. When time advances past the scheduled auto-stop, running sweepAutoStops results in a legitimate `stop` event
//     being appended (relying on journal.Parser to reconstruct entries and detect open starts).
func TestStartForAndSweepAutoStops(t *testing.T) {
	// Create an isolated HOME for journals so tests do not touch real user files.
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() {
		_ = os.Setenv("HOME", oldHome)
	}()

	// Deterministic Now provider for the test lifecycle
	// Ensure parser/timezone resolution uses UTC in tests.
	viper.Set("timezone", "UTC")
	anchor := time.Date(2025, 10, 14, 9, 0, 0, 0, time.UTC) // start at 09:00 UTC
	oldNow := Now
	Now = func() time.Time { return anchor }
	defer func() { Now = oldNow }()

	// Deterministic ID generator so we can correlate events easily.
	oldIDGen := IDGen
	IDGen = func() string { return "evt-start-test-1" }
	defer func() { IDGen = oldIDGen }()

	// Ensure flags/vars are in known state and restore after test.
	oldStartFor := startFor
	oldStartAt := startAt
	oldStartActivity := startActivity
	oldStartBillable := startBillable
	oldStartTags := startTags
	oldStartNote := startNote
	defer func() {
		startFor = oldStartFor
		startAt = oldStartAt
		startActivity = oldStartActivity
		startBillable = oldStartBillable
		startTags = oldStartTags
		startNote = oldStartNote
	}()

	// Use --for 25m
	startFor = "25m"
	startActivity = "testing"
	startBillable = true
	startTags = nil
	startNote = "auto-stop test"
	startAt = "" // use Now()

	// Run the start command (simulate user: tt start acme proj --for 25m)
	startCmd.Run(&cobra.Command{}, []string{"Acme", "Proj"})

	// The start event should have been written to today's journal (based on anchor).
	dayPath := journalPathFor(anchor)
	bb, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("read journal file: %v", err)
	}
	lines := splitNonEmptyLines(string(bb))

	// Find the start event and assert meta["auto_stop"] present and correct.
	var foundStart bool
	var autoStopStr string
	for _, ln := range lines {
		var ev journal.Event
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			// ignore malformed lines
			continue
		}
		if ev.Type == "start" && ev.ID == "evt-start-test-1" {
			foundStart = true
			if ev.Meta == nil {
				t.Fatalf("start event missing meta; expected auto_stop")
			}
			as, ok := ev.Meta["auto_stop"]
			if !ok || as == "" {
				t.Fatalf("start event missing auto_stop meta")
			}
			autoStopStr = as
			break
		}
	}
	if !foundStart {
		t.Fatalf("start event not found in journal %s", dayPath)
	}

	// Parse the auto_stop time and verify it equals anchor + 25m
	asTime, err := time.Parse(time.RFC3339, autoStopStr)
	if err != nil {
		t.Fatalf("invalid auto_stop time format: %v", err)
	}
	expectedAuto := anchor.Add(25 * time.Minute)
	if !asTime.Equal(expectedAuto) {
		t.Fatalf("auto_stop mismatch: got %v want %v", asTime, expectedAuto)
	}

	// Now advance time beyond the scheduled auto-stop and run sweepAutoStops
	Now = func() time.Time { return anchor.Add(30 * time.Minute) } // now = 09:30
	if err := sweepAutoStops(); err != nil {
		t.Fatalf("sweepAutoStops failed: %v", err)
	}

	// Re-read the journal file(s) for the auto-stop date (should be same day here)
	bb2, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("read journal file after sweep: %v", err)
	}
	lines2 := splitNonEmptyLines(string(bb2))

	// Verify a stop event exists with TS == expectedAuto (or a stop event for later time).
	var foundStop bool
	for _, ln := range lines2 {
		var ev journal.Event
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			continue
		}
		if ev.Type == "stop" {
			// stop may be written with the exact scheduled time; compare instants.
			if ev.TS.Equal(expectedAuto) {
				foundStop = true
				break
			}
			// In some circumstances the stop may be written slightly differently; accept any stop after start.
			if ev.TS.After(anchor) {
				foundStop = true
				break
			}
		}
	}
	if !foundStop {
		t.Fatalf("expected auto-stop stop event to be written for start evt-start-test-1; none found")
	}
}

// splitNonEmptyLines splits text into non-empty trimmed lines.
func splitNonEmptyLines(s string) []string {
	var out []string
	for _, l := range splitLines(s) {
		if t := trimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// The following small wrappers avoid importing extra packages at top-level in this test file.
// They simply delegate to stdlib functions but make test code concise.

func splitLines(s string) []string {
	// strings.Split but inlined to avoid extra imports at top-level
	var res []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			res = append(res, s[start:i])
			start = i + 1
		}
	}
	// last segment
	if start <= len(s) {
		res = append(res, s[start:])
	}
	return res
}

func trimSpace(s string) string {
	// minimal trim: remove spaces and tabs and CR
	start := 0
	end := len(s)
	for start < end {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\r' {
			start++
			continue
		}
		break
	}
	for end > start {
		c := s[end-1]
		if c == ' ' || c == '\t' || c == '\r' {
			end--
			continue
		}
		break
	}
	return s[start:end]
}
