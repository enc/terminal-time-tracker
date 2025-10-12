package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"tt/internal/journal"
)

// Event represents a single immutable journal event.
type Event struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"` // start|stop|add|amend|pause|resume|note
	TS       time.Time         `json:"ts"`
	User     string            `json:"user,omitempty"`
	Customer string            `json:"customer,omitempty"`
	Project  string            `json:"project,omitempty"`
	Activity string            `json:"activity,omitempty"`
	Billable *bool             `json:"billable,omitempty"`
	Note     string            `json:"note,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Ref      string            `json:"ref,omitempty"` // link to entry id for amend/note
	Meta     map[string]string `json:"meta,omitempty"`
	PrevHash string            `json:"prev_hash,omitempty"`
	Hash     string            `json:"hash,omitempty"`
}

// canonicalPayload is used to generate deterministic hashes for events.
type canonicalPayload struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	TS       string   `json:"ts"`
	User     string   `json:"user,omitempty"`
	Customer string   `json:"customer,omitempty"`
	Project  string   `json:"project,omitempty"`
	Activity string   `json:"activity,omitempty"`
	Billable *bool    `json:"billable,omitempty"`
	Note     string   `json:"note,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Ref      string   `json:"ref,omitempty"`
	PrevHash string   `json:"prev_hash,omitempty"`
}

// Entry is a materialized timesheet entry derived from events.
type Entry struct {
	ID       string
	Start    time.Time
	End      *time.Time
	Customer string
	Project  string
	Activity string
	Billable bool
	Notes    []string
	Tags     []string
}

// journal path helpers -------------------------------------------------------

func journalDirFor(t time.Time) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tt", "journal", t.Format("2006"), t.Format("01"))
}

func journalPathFor(t time.Time) string {
	dir := journalDirFor(t)
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, t.Format("2006-01-02")+".jsonl")
}

func readLastHash(path string) string {
	b, _ := os.ReadFile(path + ".hash") // simple per-day hash anchor
	return string(b)
}

func writeLastHash(path, h string) {
	_ = os.WriteFile(path+".hash", []byte(h), 0o644)
}

// EventWriter abstracts persistence of events to enable testing and alternate backends.
type EventWriter interface {
	WriteEvent(e Event) error
}

// fileEventWriter is the default file-based EventWriter used by the CLI.
type fileEventWriter struct{}

// WriteEvent implements EventWriter by appending canonical JSON lines to the per-day journal file.
func (fw *fileEventWriter) WriteEvent(e Event) error {
	p := journalPathFor(e.TS)
	prev := readLastHash(p)
	e.PrevHash = prev

	cp := canonicalPayload{
		ID:       e.ID,
		Type:     e.Type,
		TS:       e.TS.Format(time.RFC3339Nano),
		User:     e.User,
		Customer: e.Customer,
		Project:  e.Project,
		Activity: e.Activity,
		Billable: e.Billable,
		Note:     e.Note,
		Tags:     e.Tags,
		Ref:      e.Ref,
		PrevHash: e.PrevHash,
	}
	j, _ := json.Marshal(cp)
	h := sha256.Sum256(j)
	e.Hash = hex.EncodeToString(h[:])

	line, _ := json.Marshal(e)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	writeLastHash(p, e.Hash)
	return nil
}

// Writer is the package-level EventWriter in use. Tests may replace this with a fake.
var Writer EventWriter = &fileEventWriter{}

// NowProvider provides the current time. It can be injected in tests for determinism.
type NowProvider func() time.Time

// IDProvider provides unique IDs for events. It can be injected in tests.
type IDProvider func() string

// Default injectable providers. Tests may override these for deterministic behaviour.
var (
	Now   NowProvider = func() time.Time { return nowLocal() }
	IDGen IDProvider  = func() string { return fmt.Sprintf("tt_%d", time.Now().UnixNano()) }
)

// Convenience wrapper to maintain backwards compatibility with callers that use writeEvent.
func writeEvent(e Event) error { return Writer.WriteEvent(e) }

// small helpers --------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

// fmtBillable returns the boolean value for a possibly-nil *bool, defaulting to true when nil.
func fmtBillable(b *bool) bool {
	if b == nil {
		return true
	}
	return *b
}

// nowLocal returns the current time in the configured timezone.
func nowLocal() time.Time {
	loc := time.Local
	if tz := viper.GetString("timezone"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	return time.Now().In(loc)
}

// Factory helpers for creating Events to reduce duplication across commands.
//
// The helpers take explicit ts/id inputs to make testing deterministic. For
// production callers, use Now() and IDGen().
func NewStartEvent(id, customer, project, activity string, billable *bool, note string, tags []string, ts time.Time) Event {
	return Event{
		ID:       id,
		Type:     "start",
		TS:       ts,
		Customer: customer,
		Project:  project,
		Activity: activity,
		Billable: billable,
		Note:     note,
		Tags:     tags,
	}
}

func NewStopEvent(id string, ts time.Time) Event {
	return Event{
		ID:   id,
		Type: "stop",
		TS:   ts,
	}
}

func NewAddEvent(id, customer, project, activity string, billable *bool, note string, tags []string, start, end time.Time) Event {
	ref := start.Format(time.RFC3339) + ".." + end.Format(time.RFC3339)
	return Event{
		ID:       id,
		Type:     "add",
		TS:       nowLocal(), // keep original semantic of add timestamp being "now" (journal timestamp)
		Customer: customer,
		Project:  project,
		Activity: activity,
		Billable: billable,
		Note:     note,
		Tags:     tags,
		Ref:      ref,
	}
}

// Materialize entries from events for a given date range
// Refactored to use the internal/journal parser to centralize parsing logic.
func loadEntries(from, to time.Time) ([]Entry, error) {
	// Normalize to local day boundaries
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, to.Location())

	// Create a journal parser using configured timezone (falls back to Local inside the parser).
	p := journal.NewParser(viper.GetString("timezone"))

	var entries []Entry
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		pth := journalPathFor(d)
		ents, err := p.ParseFile(pth)
		if err != nil {
			// Preserve previous behaviour of skipping missing/malformed files in non-strict mode.
			continue
		}
		// Map journal.Entry -> cmd.Entry
		for _, je := range ents {
			e := Entry{
				ID:       je.ID,
				Start:    je.Start,
				End:      je.End,
				Customer: je.Customer,
				Project:  je.Project,
				Activity: je.Activity,
				Billable: je.Billable,
				Notes:    je.Notes,
				Tags:     je.Tags,
			}
			entries = append(entries, e)
		}
	}

	// Ensure deterministic ordering across days
	sort.Slice(entries, func(i, j int) bool { return entries[i].Start.Before(entries[j].Start) })

	return entries, nil
}

func durationMinutes(e Entry) int {
	if e.End == nil {
		return 0
	}
	d := e.End.Sub(e.Start)
	return int(d.Minutes())
}

type Rounding struct {
	Strategy     string // up|down|nearest
	QuantumMin   int
	MinimumEntry int // minimum billable per entry (minutes)
}

func getRounding() Rounding {
	q := viper.GetInt("rounding.quantum_min")
	if q == 0 {
		q = 15
	}
	min := viper.GetInt("rounding.minimum_billable_min")
	return Rounding{Strategy: viper.GetString("rounding.strategy"), QuantumMin: q, MinimumEntry: min}
}

func roundMinutes(min int, r Rounding) int {
	if min <= 0 {
		return 0
	}
	q := r.QuantumMin
	if q <= 0 {
		q = 15
	}
	switch r.Strategy {
	case "down":
		min = (min / q) * q
	case "nearest":
		rem := min % q
		// Use integer half (q/2) as threshold so odd quantum values behave deterministically
		// (e.g. QuantumMin=15 => threshold 7, so 22 -> rounds up to 30 to match expectations)
		if rem >= q/2 {
			min = ((min / q) + 1) * q
		} else {
			min = (min / q) * q
		}
	default: // up
		if min%q != 0 {
			min = ((min / q) + 1) * q
		}
	}
	if r.MinimumEntry > 0 && min < r.MinimumEntry {
		min = r.MinimumEntry
	}
	return min
}

func parseRangeFlags(today bool, week bool, rng string) (time.Time, time.Time) {
	now := nowLocal()
	if rng != "" {
		parts := strings.Split(rng, "..")
		if len(parts) != 2 {
			cobra.CheckErr(fmt.Errorf("invalid --range; expected A..B"))
		}
		from := mustParseTimeLocal(parts[0])
		to := mustParseTimeLocal(parts[1])
		return from, to
	}
	if today {
		return now, now
	}
	if week {
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		} // make Monday=1..Sunday=7
		monday := now.AddDate(0, 0, -(wd - 1))
		return monday, monday.AddDate(0, 0, 6)
	}
	// default: today
	return now, now
}

func fmtHHMM(min int) string {
	h := min / 60
	m := min % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}
