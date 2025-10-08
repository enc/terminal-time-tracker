package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestTempoDayAggregation(t *testing.T) {
	// Prepare isolated HOME so journal files go to temp dir
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	// Ensure timezone is fixed for deterministic behavior
	viper.Set("timezone", "UTC")

	// Choose a date and create events for it
	day := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	start1 := day.Add(9 * time.Hour)                // 09:00
	stop1 := day.Add(10*time.Hour + 30*time.Minute) // 10:30 => 90m
	start2 := day.Add(14 * time.Hour)               // 14:00
	stop2 := day.Add(15*time.Hour + 15*time.Minute) // 15:15 => 75m

	// Create a start/stop pair (start + stop)
	ev1 := Event{
		ID:       "e1",
		Type:     "start",
		TS:       start1,
		Customer: "Acme",
		Project:  "Website",
		Activity: "dev",
		Billable: boolPtr(true),
		Note:     "coding",
	}
	if err := writeEvent(ev1); err != nil {
		t.Fatalf("writeEvent start failed: %v", err)
	}
	ev2 := Event{
		ID:   "e2",
		Type: "stop",
		TS:   stop1,
	}
	if err := writeEvent(ev2); err != nil {
		t.Fatalf("writeEvent stop failed: %v", err)
	}

	// Create an "add" entry (explicit start..end in Ref)
	ev3 := Event{
		ID:       "e3",
		Type:     "add",
		TS:       start2,
		Customer: "Acme",
		Project:  "Website",
		Activity: "meeting",
		Billable: boolPtr(true),
		Note:     "sync",
		Ref:      start2.Format(time.RFC3339) + ".." + stop2.Format(time.RFC3339),
	}
	if err := writeEvent(ev3); err != nil {
		t.Fatalf("writeEvent add failed: %v", err)
	}

	// Configure command-level flags for tempoDayCmd invocation
	origDate := dayDate
	origToday := dayToday
	origGroupBy := dayGroupBy
	origRound := dayRound
	origIssue := dayIssue
	defer func() {
		dayDate = origDate
		dayToday = origToday
		dayGroupBy = origGroupBy
		dayRound = origRound
		dayIssue = origIssue
	}()

	// Set dayDate to a parseable timestamp (mustParseTimeLocal accepts "YYYY-MM-DDTHH:MM")
	dayDate = day.Format("2006-01-02T15:04")
	dayToday = false
	dayGroupBy = "activity"
	dayRound = "rounded"
	dayIssue = ""

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w

	// Run the command handler directly
	if tempoDayCmd == nil {
		t.Fatalf("tempoDayCmd is nil")
	}
	if tempoDayCmd.RunE == nil {
		t.Fatalf("tempoDayCmd.RunE is nil")
	}
	if err := tempoDayCmd.RunE(tempoDayCmd, []string{}); err != nil {
		// restore stdout before failing
		_ = w.Close()
		os.Stdout = oldStdout
		t.Fatalf("tempoDayCmd.RunE returned error: %v", err)
	}

	// Close writer and restore stdout, read output
	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout failed: %v", err)
	}
	out := buf.String()

	// Basic sanity checks on output content
	if !strings.Contains(out, "Consolidated view") {
		t.Fatalf("output missing header; got:\n%s", out)
	}
	// Total should be 90 + 75 = 165 minutes => 2h45m
	if !strings.Contains(out, "TOTAL:") {
		t.Fatalf("output missing TOTAL line; got:\n%s", out)
	}
	if !strings.Contains(out, "2h45m") {
		t.Fatalf("expected total 2h45m in output; got:\n%s", out)
	}

	// Expect booking suggestion contains group-by and date
	if !strings.Contains(out, "--group-by activity") {
		t.Fatalf("booking suggestion missing group-by activity; got:\n%s", out)
	}
	if !strings.Contains(out, "--date "+day.Format("2006-01-02")) {
		t.Fatalf("booking suggestion missing date; got:\n%s", out)
	}
}
