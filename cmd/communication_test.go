package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/spf13/viper"
)

// Improved, stricter tests for communication helpers and formatters.
// - Table-driven exact-output checks where deterministic values can be used.
// - Fuzzy/quick-check style tests to validate properties across random inputs.
// - Regex-based assertions for format patterns.

// Helper to build exact expected Start output when colors are disabled.
func expectedStartLine(ev Event) string {
	start := ev.TS.Format("2006-01-02 " + time.Kitchen)
	return fmt.Sprintf("Started: %s / %s [%s] at %s billable=%v", ev.Customer, ev.Project, ev.Activity, start, fmtBillable(ev.Billable))
}

// Helper to build exact expected Stop output for an Entry when colors are disabled.
func expectedStopBlock(ent *Entry, stop time.Time) string {
	startStr := ent.Start.Format("2006-01-02 " + time.Kitchen)
	stopStr := stop.Format("2006-01-02 " + time.Kitchen)
	mins := 0
	if ent.End != nil {
		mins = int(ent.End.Sub(ent.Start).Minutes())
	} else {
		mins = int(stop.Sub(ent.Start).Minutes())
	}
	dur := fmtHHMM(mins)
	bill := "false"
	if ent.Billable {
		bill = "true"
	}
	return fmt.Sprintf("Stopped: %s / %s [%s]\n  started=%s stopped=%s duration=%s billable=%s",
		ent.Customer, ent.Project, ent.Activity, startStr, stopStr, dur, bill)
}

func TestFormatTS_SameAndOtherDay(t *testing.T) {
	now := nowLocal()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"same-day", now, now.Format(time.Kitchen)},
		{"other-day", now.AddDate(0, 0, -1), now.AddDate(0, 0, -1).Format("2006-01-02 " + time.Kitchen)},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := formatTS(tc.t)
			if got != tc.want {
				t.Fatalf("formatTS(%v) = %q; want %q", tc.t, got, tc.want)
			}
		})
	}
}

func TestFmtDurationAndFmtHHMM_Table(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"45m", 45 * time.Minute, "45m"},
		{"90m", 90 * time.Minute, "1h30m"},
		{"2h5m", 125 * time.Minute, "2h05m"},
		{"0m", 0, "0m"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := fmtDuration(tc.in); got != tc.want {
				t.Fatalf("fmtDuration(%v) = %q; want %q", tc.in, got, tc.want)
			}
			mins := int(tc.in.Minutes())
			if got := fmtHHMM(mins); got != tc.want {
				t.Fatalf("fmtHHMM(%d) = %q; want %q", mins, got, tc.want)
			}
		})
	}
}

// Quick/fuzzy checks for duration formatting patterns.
func TestFmtDuration_Quick(t *testing.T) {
	re := regexp.MustCompile(`^(?:\d+h\d{2}m|\d+m)$`)
	f := func(mins int) bool {
		// keep test domain reasonable
		if mins < 0 || mins > 60*24*30 {
			return true
		}
		out := fmtHHMM(mins)
		return re.MatchString(out)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("quick check failed: %v", err)
	}
}

func TestFormatStartStopSwitch_ExactAndTable(t *testing.T) {
	// Use deterministic timezone and disable colors so outputs are exact.
	viper.Set("timezone", "UTC")
	DisableColors()
	defer EnableColors()

	// Table-driven exact checks for start/stop/switch where we can deterministically
	// compute expected strings (use dates not equal to today so formatTS includes date).
	baseDay := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		startEvent Event
		stopEntry  *Entry // optional
		stopTime   time.Time
		expStart   string
		expStop    string // optional
	}{
		{
			name: "start-only",
			startEvent: Event{
				ID:       "s1",
				Type:     "start",
				TS:       baseDay.Add(9 * time.Hour),
				Customer: "ACME",
				Project:  "Website",
				Activity: "design",
				Billable: boolPtr(true),
			},
			expStart: expectedStartLine(Event{Customer: "ACME", Project: "Website", Activity: "design", TS: baseDay.Add(9 * time.Hour), Billable: boolPtr(true)}),
		},
		{
			name: "stop-block",
			startEvent: Event{
				ID:       "s2",
				Type:     "start",
				TS:       baseDay.Add(14 * time.Hour),
				Customer: "Beta",
				Project:  "Proj",
				Activity: "meeting",
				Billable: boolPtr(false),
			},
			stopEntry: &Entry{
				ID:       "e1",
				Start:    baseDay.Add(14 * time.Hour),
				End:      ptrTime(baseDay.Add(15*time.Hour + 15*time.Minute)),
				Customer: "Beta",
				Project:  "Proj",
				Activity: "meeting",
				Billable: false,
			},
			stopTime: baseDay.Add(15*time.Hour + 15*time.Minute),
			expStop:  expectedStopBlock(&Entry{Customer: "Beta", Project: "Proj", Activity: "meeting", Start: baseDay.Add(14 * time.Hour), End: ptrTime(baseDay.Add(15*time.Hour + 15*time.Minute)), Billable: false}, baseDay.Add(15*time.Hour+15*time.Minute)),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Start expected exact
			gotStart := FormatStartResult(tc.startEvent)
			// Only assert exact start output when an expected value was provided.
			// Some table entries are focused on stop-block validation and may leave
			// expStart empty on purpose; avoid failing those cases.
			if tc.expStart != "" {
				if gotStart != tc.expStart {
					t.Fatalf("FormatStartResult = %q; want %q", gotStart, tc.expStart)
				}
			}

			// If stop block present, verify exact multi-line block for stop formatting.
			if tc.stopEntry != nil {
				gotStop := FormatStopResultFromEntry(tc.stopEntry, tc.stopTime)
				if gotStop != tc.expStop {
					t.Fatalf("FormatStopResultFromEntry = %q; want %q", gotStop, tc.expStop)
				}
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// Fuzzy test for FormatStartResult: given randomized components ensure
// the output includes the provided customer/project/activity and billable flag.
func TestFormatStartResult_Quick(t *testing.T) {
	viper.Set("timezone", "UTC")
	DisableColors()
	defer EnableColors()

	f := func(cust, proj, act string, bill bool, hour uint8, min uint8) bool {
		// constrain hour/min
		if hour > 23 || min > 59 {
			return true
		}
		ev := Event{
			ID:       "q",
			Type:     "start",
			TS:       time.Date(2024, 12, 31, int(hour), int(min), 0, 0, time.UTC),
			Customer: cust,
			Project:  proj,
			Activity: act,
			Billable: boolPtr(bill),
		}
		out := FormatStartResult(ev)
		// must contain key parts
		if !strings.Contains(out, "Started:") {
			return false
		}
		if cust != "" && !strings.Contains(out, cust) {
			return false
		}
		if proj != "" && !strings.Contains(out, proj) {
			return false
		}
		if act != "" && !strings.Contains(out, "["+act+"]") {
			return false
		}
		if !strings.Contains(out, "billable=") {
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("quick-check FormatStartResult failed: %v", err)
	}
}

// Ensure LastOpenEntryAt reconstructs an open entry from the journal file (deterministic).
func TestLastOpenEntryAt_ReconstructedFromJournal(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv HOME failed: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	viper.Set("timezone", "UTC")

	origNow := Now
	origID := IDGen
	defer func() {
		Now = origNow
		IDGen = origID
	}()

	fixedNow := time.Date(2025, 10, 20, 12, 0, 0, 0, time.UTC)
	Now = func() time.Time { return fixedNow }
	IDGen = func() string { return "id-fixed-1" }

	start := fixedNow.Add(-30 * time.Minute)
	ev := Event{
		ID:       "ev-open-1",
		Type:     "start",
		TS:       start,
		Customer: "JournalCo",
		Project:  "JProj",
		Activity: "coding",
		Billable: boolPtr(true),
	}
	if err := writeEvent(ev); err != nil {
		t.Fatalf("writeEvent failed: %v", err)
	}

	ent, err := LastOpenEntryAt(fixedNow)
	if err != nil {
		t.Fatalf("LastOpenEntryAt returned error: %v", err)
	}
	if ent == nil {
		t.Fatalf("LastOpenEntryAt did not find the open entry")
	}
	if ent.Customer != ev.Customer || ent.Project != ev.Project || ent.Activity != ev.Activity {
		t.Fatalf("reconstructed entry mismatch: got %+v want subset %+v", ent, ev)
	}
}
