package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeJournal struct {
	entries  []Entry
	loadFrom time.Time
	loadTo   time.Time
}

func (f *fakeJournal) LoadEntries(ctx context.Context, from, to time.Time) ([]Entry, error) {
	f.loadFrom = from
	f.loadTo = to
	return f.entries, nil
}

func (f *fakeJournal) FindActiveAndLast(ctx context.Context, from, to time.Time) (*Entry, *Entry, error) {
	return nil, nil, nil
}

type fakeConfig struct {
	tz *time.Location
}

func (f fakeConfig) Timezone() *time.Location { return f.tz }
func (f fakeConfig) Rounding() RoundingConfig { return RoundingConfig{} }

func TestDashboardToggleTimelinesLoadsWeek(t *testing.T) {
	j := &fakeJournal{entries: []Entry{{Customer: "Acme", Start: time.Now()}}}
	d := newDashboardModel(Services{
		Journal: j,
		Config:  fakeConfig{tz: time.UTC},
	})

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}
	var cmd tea.Cmd
	d, cmd = d.Update(msg)
	if !d.showTimelines {
		t.Fatalf("expected timelines to be enabled after 't'")
	}
	if d.timelineWeekStart.Weekday() != time.Monday {
		t.Fatalf("timelineWeekStart weekday = %v; want Monday", d.timelineWeekStart.Weekday())
	}
	if d.timelineWeekStart.Hour() != 0 || d.timelineWeekStart.Minute() != 0 {
		t.Fatalf("timelineWeekStart not at midnight: %v", d.timelineWeekStart)
	}
	if cmd == nil {
		t.Fatalf("expected loadWeekEntries command on toggle")
	}
	if _, ok := cmd().(weekLoadedMsg); !ok {
		t.Fatalf("toggle command did not return weekLoadedMsg")
	}
}

func TestDashboardTimelinePrevNextWeek(t *testing.T) {
	d := newDashboardModel(Services{
		Journal: &fakeJournal{},
		Config:  fakeConfig{tz: time.UTC},
	})
	d.showTimelines = true
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC) // Monday
	d.timelineWeekStart = start

	var cmd tea.Cmd
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if d.timelineWeekStart != start.AddDate(0, 0, -7) {
		t.Fatalf("prev week start = %v; want %v", d.timelineWeekStart, start.AddDate(0, 0, -7))
	}
	if cmd == nil {
		t.Fatalf("expected loadWeekEntries command on prev week")
	}

	start = d.timelineWeekStart
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if d.timelineWeekStart != start.AddDate(0, 0, 7) {
		t.Fatalf("next week start = %v; want %v", d.timelineWeekStart, start.AddDate(0, 0, 7))
	}
	if cmd == nil {
		t.Fatalf("expected loadWeekEntries command on next week")
	}
}
