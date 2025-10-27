package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLoadRecentEntriesForAmend(t *testing.T) {
	originalNow := nowLocalForAmend
	originalLoader := loadEntriesForAmendFn
	defer func() {
		nowLocalForAmend = originalNow
		loadEntriesForAmendFn = originalLoader
	}()

	now := time.Date(2025, 5, 20, 15, 4, 0, 0, time.UTC)
	nowLocalForAmend = func() time.Time { return now }

	tests := []struct {
		name           string
		responsesByDay map[int][]Entry
		errDay         *int
		wantIDs        []string
		wantLen        int
		wantFirstID    string
		wantLastID     string
		wantErrSubstr  string
	}{
		{
			name: "returns entries from current day when available",
			responsesByDay: map[int][]Entry{
				0: {
					makeEntry("a-1", now.Add(-2*time.Hour)),
					makeEntry("a-2", now.Add(-time.Hour)),
				},
			},
			wantIDs: []string{"a-1", "a-2"},
		},
		{
			name: "falls back to older lookback when recent days empty",
			responsesByDay: map[int][]Entry{
				7: {
					makeEntry("b-1", now.AddDate(0, 0, -7).Add(2*time.Hour)),
					makeEntry("b-2", now.AddDate(0, 0, -7).Add(3*time.Hour)),
				},
			},
			wantIDs: []string{"b-1", "b-2"},
		},
		{
			name: "limits results to last fifty entries when more available",
			responsesByDay: map[int][]Entry{
				30: buildSequentialEntries(now.AddDate(0, 0, -30), 60),
			},
			wantLen:     maxAmendSelectionEntries,
			wantFirstID: "entry-10",
			wantLastID:  "entry-59",
		},
		{
			name: "propagates loader error",
			responsesByDay: map[int][]Entry{
				0: nil,
			},
			errDay:        ptr(0),
			wantErrSubstr: "boom",
		},
		{
			name:           "returns error when no entries found across lookbacks",
			responsesByDay: map[int][]Entry{},
			wantErrSubstr:  "no entries found",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				loadEntriesForAmendFn = originalLoader
			})

			responses := make(map[string][]Entry)
			for days, entries := range tc.responsesByDay {
				from := now.AddDate(0, 0, -days)
				responses[keyForTime(from)] = entries
			}

			loadEntriesForAmendFn = func(from, to time.Time) ([]Entry, error) {
				key := keyForTime(from)
				if tc.errDay != nil && key == keyForTime(now.AddDate(0, 0, -*tc.errDay)) {
					return nil, fmt.Errorf("boom")
				}
				if entries, ok := responses[key]; ok {
					return entries, nil
				}
				return []Entry{}, nil
			}

			got, err := loadRecentEntriesForAmend()
			if tc.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrSubstr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("loadRecentEntriesForAmend() unexpected error: %v", err)
			}

			if tc.wantLen > 0 && len(got) != tc.wantLen {
				t.Fatalf("expected %d entries, got %d", tc.wantLen, len(got))
			}

			if tc.wantFirstID != "" && got[0].ID != tc.wantFirstID {
				t.Fatalf("expected first entry %q, got %q", tc.wantFirstID, got[0].ID)
			}
			if tc.wantLastID != "" && got[len(got)-1].ID != tc.wantLastID {
				t.Fatalf("expected last entry %q, got %q", tc.wantLastID, got[len(got)-1].ID)
			}

			if tc.wantIDs != nil {
				if len(got) != len(tc.wantIDs) {
					t.Fatalf("expected %d entries, got %d", len(tc.wantIDs), len(got))
				}
				for i, want := range tc.wantIDs {
					if got[i].ID != want {
						t.Fatalf("expected entry %d to have id %q, got %q", i, want, got[i].ID)
					}
				}
			}
		})
	}
}

func TestFindMostRecentEntryForAmend(t *testing.T) {
	originalNow := nowLocalForAmend
	originalLoader := loadEntriesForAmendFn
	defer func() {
		nowLocalForAmend = originalNow
		loadEntriesForAmendFn = originalLoader
	}()

	now := time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC)
	nowLocalForAmend = func() time.Time { return now }

	t.Run("returns pointer to latest entry", func(t *testing.T) {
		loadEntriesForAmendFn = func(from, to time.Time) ([]Entry, error) {
			return []Entry{
				makeEntry("first", now.Add(-6*time.Hour)),
				makeEntry("second", now.Add(-3*time.Hour)),
				makeEntry("third", now.Add(-time.Hour)),
			}, nil
		}

		entry, err := findMostRecentEntryForAmend()
		if err != nil {
			t.Fatalf("findMostRecentEntryForAmend() error: %v", err)
		}
		if entry == nil || entry.ID != "third" {
			t.Fatalf("expected entry with id %q, got %+v", "third", entry)
		}
	})

	t.Run("propagates error from loader", func(t *testing.T) {
		loadEntriesForAmendFn = func(from, to time.Time) ([]Entry, error) {
			return nil, errors.New("cannot read entries")
		}

		_, err := findMostRecentEntryForAmend()
		if err == nil || !strings.Contains(err.Error(), "cannot read entries") {
			t.Fatalf("expected loader error, got %v", err)
		}
	})
}

func TestEntryItemFormatting(t *testing.T) {
	end := time.Date(2025, 5, 19, 17, 0, 0, 0, time.Local)
	start := end.Add(-90 * time.Minute)

	item := entryItem{
		entry: &Entry{
			ID:       "xyz",
			Start:    start,
			End:      &end,
			Customer: "Acme",
			Project:  "Website",
			Activity: "Planning",
			Notes:    []string{"Initial sync", "Follow-up"},
			Tags:     []string{"client", "design"},
		},
	}

	title := item.Title()
	if !strings.Contains(title, "Acme") || !strings.Contains(title, "Website") || !strings.Contains(title, "90m") {
		t.Fatalf("title did not include expected details: %q", title)
	}

	desc := item.Description()
	if !strings.Contains(desc, "Initial sync; Follow-up") {
		t.Fatalf("description missing notes: %q", desc)
	}
	if !strings.Contains(desc, "#client #design") {
		t.Fatalf("description missing tags: %q", desc)
	}

	filter := item.FilterValue()
	for _, want := range []string{"xyz", "acme", "website", "planning", "client", "initial sync"} {
		if !strings.Contains(filter, want) {
			t.Fatalf("filter value missing %q: %q", want, filter)
		}
	}
}

func makeEntry(id string, start time.Time) Entry {
	end := start.Add(time.Hour)
	return Entry{
		ID:    id,
		Start: start,
		End:   &end,
	}
}

func buildSequentialEntries(start time.Time, count int) []Entry {
	entries := make([]Entry, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("entry-%d", i)
		entries = append(entries, makeEntry(id, start.Add(time.Duration(i)*time.Minute)))
	}
	return entries
}

func keyForTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ptr[T any](v T) *T {
	return &v
}
