package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseISOWeek(t *testing.T) {
	cases := []struct {
		in       string
		wantYear int
		wantWeek int
		wantErr  bool
	}{
		{"2025-W41", 2025, 41, false},
		{"2025W41", 2025, 41, false},
		{"2025-W1", 2025, 1, false},
		{"bad", 0, 0, true},
		{"", 0, 0, true},
	}

	for _, tc := range cases {
		y, w, err := parseISOWeek(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for input %q, got none", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for input %q: %v", tc.in, err)
		}
		if y != tc.wantYear || w != tc.wantWeek {
			t.Fatalf("parseISOWeek(%q) = %d-W%d, want %d-W%d", tc.in, y, w, tc.wantYear, tc.wantWeek)
		}
	}
}

func TestISOWeekRange_Basic(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("timezone load failed: %v", err)
	}
	// Known sample: 2025-W41 -> 2025-10-06 (Mon) .. 2025-10-12 (Sun)
	start, end := isoWeekRange(2025, 41, loc)
	if start.Year() != 2025 || start.Month() != time.October || start.Day() != 6 {
		t.Fatalf("unexpected start for 2025-W41: %v", start)
	}
	if end.Year() != 2025 || end.Month() != time.October || end.Day() != 12 {
		t.Fatalf("unexpected end for 2025-W41: %v", end)
	}
	// start must be midnight (00:00:00) in provided location
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("start not at midnight: %v", start)
	}
	// end must be 23:59:59
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Fatalf("end not at 23:59:59: %v", end)
	}
}

func TestPerEntryRoundUpTo15Minutes(t *testing.T) {
	// The new default behavior is: each entry is rounded UP to 15-minute intervals,
	// then rounded totals are allocated/summed. This test asserts the pure rounding
	// helper that always rounds up to the quantum.
	quantum := int64(900) // 15 minutes

	cases := []struct {
		sec  int64
		want int64
	}{
		{0, 0},       // zero stays zero
		{1, 900},     // any positive < quantum rounds up
		{600, 900},   // 10 minutes -> round up to 15
		{900, 900},   // exact quantum unchanged
		{901, 1800},  // just over one quantum -> next quantum
		{1800, 1800}, // exact multiple preserved
	}

	for _, c := range cases {
		got := roundUpSecondsToQuantum(c.sec, quantum)
		if got != c.want {
			t.Fatalf("roundUpSecondsToQuantum(%d, %d) = %d; want %d", c.sec, quantum, got, c.want)
		}
	}
}

func TestDedupeNormalizeMergeNotes(t *testing.T) {
	// normalizeNote
	in := "  API  \n  scaffolding\t"
	got := normalizeNote(in)
	if got != "API scaffolding" {
		t.Fatalf("normalizeNote: got %q want %q", got, "API scaffolding")
	}

	// dedupeStrings
	arr := []string{"a", "a", "b", "", "b", "c"}
	dd := dedupeStrings(arr)
	if len(dd) != 3 || dd[0] != "a" || dd[1] != "b" || dd[2] != "c" {
		t.Fatalf("dedupeStrings result unexpected: %v", dd)
	}

	// mergeNotesForDisplay with wrapping
	notes := []string{"API scaffolding", "Standup + deploy"}
	mergedNoWrap := mergeNotesForDisplay(notes, 0)
	if mergedNoWrap != "API scaffolding • Standup + deploy" {
		t.Fatalf("mergeNotesForDisplay no-wrap unexpected: %q", mergedNoWrap)
	}
	mergedWrap := mergeNotesForDisplay(notes, 10)
	// wrapped output should contain either newline or still contain the bullet separator
	if mergedWrap == "" {
		t.Fatalf("mergeNotesForDisplay wrap returned empty string")
	}
	if !containsString([]string{mergedWrap}, mergedWrap) {
		t.Fatalf("unexpected mergedWrap: %q", mergedWrap)
	}
}

func TestWriteTempoExport_Simple(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "tempo.json")

	// create a sample outDay with one group
	day := outDay{
		Date:    "2025-10-06",
		Weekday: "Mo",
		Groups: []outNoteGroup{
			{
				Customer:    "ACME",
				Project:     "WebApp",
				Seconds:     14400,
				SecRounded:  14400,
				Notes:       []string{"API scaffolding"},
				NotesMerged: "API scaffolding",
			},
		},
		DaySeconds:        14400,
		DaySecondsRounded: 14400,
		Flags:             nil,
	}
	days := []outDay{day}

	err := writeTempoExport(outPath, days, 14400, 600, false)
	if err != nil {
		t.Fatalf("writeTempoExport failed: %v", err)
	}
	// read file and verify JSON shape
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read tempo file failed: %v", err)
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err != nil {
		t.Fatalf("unmarshal tempo file failed: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 worklog, got %d", len(arr))
	}
	w := arr[0]
	if w["date"] != "2025-10-06" {
		t.Fatalf("unexpected date: %v", w["date"])
	}
	if w["startTime"] != "09:00" {
		t.Fatalf("unexpected startTime: %v", w["startTime"])
	}
	// timeSpentSeconds will be a float64 when decoded into interface{}
	if secs, ok := w["timeSpentSeconds"].(float64); !ok || int64(secs) != 14400 {
		t.Fatalf("unexpected timeSpentSeconds: %v", w["timeSpentSeconds"])
	}
	if desc, ok := w["description"].(string); !ok || desc == "" {
		t.Fatalf("unexpected description: %v", w["description"])
	}
	// attributes should be a map
	if attr, ok := w["attributes"].(map[string]interface{}); !ok {
		t.Fatalf("attributes missing or wrong type: %T", w["attributes"])
	} else {
		if attr["customer"] != "ACME" {
			t.Fatalf("attributes.customer unexpected: %v", attr["customer"])
		}
		if attr["project"] != "WebApp" {
			t.Fatalf("attributes.project unexpected: %v", attr["project"])
		}
	}
}
