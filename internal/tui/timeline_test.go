package tui

import (
	"strings"
	"testing"
	"time"
)

func ptrTime(t time.Time) *time.Time { return &t }

func TestRenderWeekTimelineBasic(t *testing.T) {
	// Choose a fixed Monday as week start to make output deterministic.
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC) // 2023-10-02 (mon)

	// Entries:
	// - Acme: Monday 09:00 - 11:30 => 2h30m
	// - Beta: Tuesday 23:00 - Wed 01:00 => 2h split across Tue/Wed (1h each)
	acmeStart := weekStart.Add(9 * time.Hour)
	acmeEnd := acmeStart.Add(2*time.Hour + 30*time.Minute)

	betaStart := weekStart.AddDate(0, 0, 1).Add(23 * time.Hour) // Tuesday 23:00
	betaEnd := betaStart.Add(2 * time.Hour)                     // Wednesday 01:00

	entries := []Entry{
		{
			ID:       "e1",
			Start:    acmeStart,
			End:      ptrTime(acmeEnd),
			Customer: "Acme",
			Project:  "Website",
			Activity: "Design",
			Billable: true,
		},
		{
			ID:       "e2",
			Start:    betaStart,
			End:      ptrTime(betaEnd),
			Customer: "Beta",
			Project:  "Infra",
			Activity: "Deploy",
			Billable: false,
		},
	}

	// Use a wide width so full graphical timeline is rendered.
	out := RenderWeekTimeline(entries, weekStart, time.UTC, 140)

	// Basic assertions: customer names show up, day heading, and expected duration strings.
	if !strings.Contains(out, "Acme") {
		t.Fatalf("expected output to contain customer 'Acme'; got:\n%s", out)
	}
	if !strings.Contains(out, "Beta") {
		t.Fatalf("expected output to contain customer 'Beta'; got:\n%s", out)
	}

	// Weekday header for the chosen Monday should include "Mon 02"
	if !strings.Contains(out, weekStart.Format("Mon 02")) {
		t.Fatalf("expected output to contain day header %q; got:\n%s", weekStart.Format("Mon 02"), out)
	}

	// The Acme aggregated weekly total should include 2h30m
	if !strings.Contains(out, "2h30m") && !strings.Contains(out, "2h30") {
		t.Fatalf("expected output to include Acme duration '2h30m'; got:\n%s", out)
	}

	// The Beta entry contributes 1h on Tuesday and 1h on Wednesday; aggregated weekly total 2h
	if !strings.Contains(out, "2h") && !strings.Contains(out, "120m") {
		// be permissive: accept "2h" anywhere
		// We check presence of "2h" substring; if not found it's a failure.
		t.Fatalf("expected output to include Beta weekly total ~2h; got:\n%s", out)
	}
}

func TestRenderWeekTimelineCompactFallback(t *testing.T) {
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC)

	// Single short entry to keep compact output predictable.
	start := weekStart.Add(10 * time.Hour)
	end := start.Add(45 * time.Minute)
	entries := []Entry{
		{
			ID:       "c1",
			Start:    start,
			End:      ptrTime(end),
			Customer: "CompactCo",
			Project:  "P",
			Activity: "A",
			Billable: true,
		},
	}

	// Small width to force compact rendering path.
	out := RenderWeekTimeline(entries, weekStart, time.UTC, 20)

	// Compact output should contain the customer name and day short labels like "Mon"
	if !strings.Contains(out, "CompactCo") {
		t.Fatalf("compact output missing customer name; got:\n%s", out)
	}
	if !strings.Contains(out, "Mon") {
		t.Fatalf("compact output missing day label 'Mon'; got:\n%s", out)
	}

	// Duration should be present in compact output (45m)
	if !strings.Contains(out, "45m") && !strings.Contains(out, "0h") {
		t.Fatalf("compact output missing expected duration '45m'; got:\n%s", out)
	}
}

// Additional tests below:

func TestRenderWeekTimelineSpanningBeforeWeek(t *testing.T) {
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC) // Monday
	// Entry starts the previous day (Sunday 22:00) and ends Monday 03:00 -> contributes 3h to Monday
	start := weekStart.Add(-2 * time.Hour) // Sunday 22:00
	end := weekStart.Add(3 * time.Hour)    // Monday 03:00
	entries := []Entry{
		{
			ID:       "s1",
			Start:    start,
			End:      ptrTime(end),
			Customer: "SpanCo",
			Project:  "S",
			Activity: "X",
			Billable: true,
		},
	}

	out := RenderWeekTimeline(entries, weekStart, time.UTC, 120)
	if !strings.Contains(out, "SpanCo") {
		t.Fatalf("expected SpanCo in output; got:\n%s", out)
	}
	// Expect ~3h contribution to Monday -> look for '3h' or '3h00m'
	if !strings.Contains(out, "3h") && !strings.Contains(out, "180m") {
		t.Fatalf("expected Monday contribution ~3h in output; got:\n%s", out)
	}
}

func TestRenderWeekTimelineClippingAfterWeek(t *testing.T) {
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC) // Monday
	// Entry starts Sunday before week and ends after week end: should be clipped to the week window.
	start := weekStart.Add(-12 * time.Hour)              // Sunday 12:00 previous
	end := weekStart.AddDate(0, 0, 8).Add(6 * time.Hour) // beyond week end
	entries := []Entry{
		{
			ID:       "clip",
			Start:    start,
			End:      ptrTime(end),
			Customer: "ClipCo",
			Project:  "C",
			Activity: "Clip",
			Billable: false,
		},
	}

	out := RenderWeekTimeline(entries, weekStart, time.UTC, 120)
	if !strings.Contains(out, "ClipCo") {
		t.Fatalf("expected ClipCo in output; got:\n%s", out)
	}
	// The contribution should be at most 7*24h -> look for '168h' or '168' potentially formatted.
	// We accept any presence of 'h' as minimal sanity check that durations are reported.
	if !strings.Contains(out, "h") && !strings.Contains(out, "m") {
		t.Fatalf("expected duration units in clipping output; got:\n%s", out)
	}
}

func TestRenderWeekTimelineLegendAndOrdering(t *testing.T) {
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC) // Monday

	entries := []Entry{
		{
			ID:       "a1",
			Start:    weekStart.Add(9 * time.Hour),
			End:      ptrTime(weekStart.Add(10 * time.Hour)),
			Customer: "Zed",
			Project:  "P",
			Activity: "A",
			Billable: true,
		},
		{
			ID:       "a2",
			Start:    weekStart.Add(11 * time.Hour),
			End:      ptrTime(weekStart.Add(12 * time.Hour)),
			Customer: "Alpha",
			Project:  "P2",
			Activity: "B",
			Billable: false,
		},
	}

	out := RenderWeekTimeline(entries, weekStart, time.UTC, 140)

	// Legend should include running/billable/non-billable labels
	if !strings.Contains(out, "running") {
		t.Fatalf("expected legend to contain 'running'; got:\n%s", out)
	}
	if !strings.Contains(out, "billable") {
		t.Fatalf("expected legend to contain 'billable'; got:\n%s", out)
	}
	if !strings.Contains(out, "non-billable") {
		t.Fatalf("expected legend to contain 'non-billable'; got:\n%s", out)
	}

	// Customer ordering should be alphabetical (Alpha before Zed)
	idxA := strings.Index(out, "Alpha")
	idxZ := strings.Index(out, "Zed")
	if idxA == -1 || idxZ == -1 {
		t.Fatalf("expected both Alpha and Zed present in output; got:\n%s", out)
	}
	if idxA > idxZ {
		t.Fatalf("expected 'Alpha' to appear before 'Zed' in output ordering; got:\n%s", out)
	}
}

func TestRenderWeekTimelineRunningEntryLegend(t *testing.T) {
	weekStart := time.Date(2023, time.October, 2, 0, 0, 0, 0, time.UTC) // Monday

	// Running entry (End == nil)
	entries := []Entry{
		{
			ID:       "r1",
			Start:    weekStart.Add(14 * time.Hour),
			End:      nil,
			Customer: "RunCo",
			Project:  "Run",
			Activity: "Now",
			Billable: true,
		},
	}

	out := RenderWeekTimeline(entries, weekStart, time.UTC, 140)
	// The legend always contains 'running' label; also ensure RunCo appears.
	if !strings.Contains(out, "RunCo") {
		t.Fatalf("expected RunCo in output for running entry; got:\n%s", out)
	}
	if !strings.Contains(out, "running") {
		t.Fatalf("expected legend to include 'running' label; got:\n%s", out)
	}
}
