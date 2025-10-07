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
)

type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"` // start|stop|add|amend|pause|resume|note
	TS        time.Time         `json:"ts"`
	User      string            `json:"user,omitempty"`
	Customer  string            `json:"customer,omitempty"`
	Project   string            `json:"project,omitempty"`
	Activity  string            `json:"activity,omitempty"`
	Billable  *bool             `json:"billable,omitempty"`
	Note      string            `json:"note,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Ref       string            `json:"ref,omitempty"` // link to entry id for amend/note
	Meta      map[string]string `json:"meta,omitempty"`
	PrevHash  string            `json:"prev_hash,omitempty"`
	Hash      string            `json:"hash,omitempty"`
}

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

func writeEvent(e Event) error {
	p := journalPathFor(e.TS)
	prev := readLastHash(p)
	e.PrevHash = prev
	// Deterministic hash over core fields + prev
	payload := map[string]any{
		"id": e.ID, "type": e.Type, "ts": e.TS.Format(time.RFC3339Nano),
		"user": e.User, "customer": e.Customer, "project": e.Project,
		"activity": e.Activity, "billable": e.Billable, "note": e.Note,
		"tags": e.Tags, "ref": e.Ref, "prev_hash": e.PrevHash,
	}
	j, _ := json.Marshal(payload)
	h := sha256.Sum256(j)
	e.Hash = hex.EncodeToString(h[:])

	line, _ := json.Marshal(e)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil { return err }
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil { return err }
	writeLastHash(p, e.Hash)
	return nil
}

func boolPtr(b bool) *bool { return &b }

func nowLocal() time.Time {
	loc := time.Local
	if tz := viper.GetString("timezone"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil { loc = l }
	}
	return time.Now().In(loc)
}

// Materialize entries from events for a given date range
func loadEntries(from, to time.Time) ([]Entry, error) {
	// Normalize to local day boundaries
	from = time.Date(from.Year(), from.Month(), from.Day(), 0,0,0,0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 23,59,59,0, to.Location())

	var events []Event
	for d := from; !d.After(to); d = d.AddDate(0,0,1) {
		p := journalPathFor(d)
		b, err := os.ReadFile(p)
		if err != nil { continue }
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if strings.TrimSpace(line) == "" { continue }
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				events = append(events, ev)
			}
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	// Reconstruct entries
	var entries []Entry
	var current *Entry
	for _, ev := range events {
		switch ev.Type {
		case "start":
			if current != nil {
				// auto-stop previous at this event ts
				cur := now := ev.TS
				current.End = &cur
				entries = append(entries, *current)
			}
			billable := true
			if ev.Billable != nil { billable = *ev.Billable }
			current = &Entry{
				ID: ev.ID, Start: ev.TS, Customer: ev.Customer, Project: ev.Project,
				Activity: ev.Activity, Billable: billable,
				Notes: []string{}, Tags: ev.Tags,
			}
			if ev.Note != "" { current.Notes = append(current.Notes, ev.Note) }
		case "note":
			if current != nil { current.Notes = append(current.Notes, ev.Note) }
		case "stop":
			if current != nil {
				cur := ev.TS
				current.End = &cur
				entries = append(entries, *current)
				current = nil
			}
		case "add":
			billable := true
			if ev.Billable != nil { billable = *ev.Billable }
			// ev.Ref is "startISO..endISO"
			parts := strings.Split(ev.Ref, "..")
			if len(parts) == 2 {
				st, _ := time.Parse(time.RFC3339, parts[0])
				en, _ := time.Parse(time.RFC3339, parts[1])
				entries = append(entries, Entry{
					ID: ev.ID, Start: st, End: &en, Customer: ev.Customer, Project: ev.Project,
					Activity: ev.Activity, Billable: billable, Notes: []string{ev.Note}, Tags: ev.Tags,
				})
			}
		}
	}
	return entries, nil
}

func durationMinutes(e Entry) int {
	if e.End == nil { return 0 }
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
	if q == 0 { q = 15 }
	min := viper.GetInt("rounding.minimum_billable_min")
	return Rounding{ Strategy: viper.GetString("rounding.strategy"), QuantumMin: q, MinimumEntry: min }
}

func roundMinutes(min int, r Rounding) int {
	if min <= 0 { return 0 }
	q := r.QuantumMin
	if q <= 0 { q = 15 }
	switch r.Strategy {
	case "down":
		min = (min / q) * q
	case "nearest":
		rem := min % q
		if rem*2 >= q { min = ((min/q)+1)*q } else { min = (min/q)*q }
	default: // up
		if min % q != 0 { min = ((min/q)+1)*q }
	}
	if r.MinimumEntry > 0 && min < r.MinimumEntry { min = r.MinimumEntry }
	return min
}

func parseRangeFlags(today bool, week bool, rng string) (time.Time, time.Time) {
	now := nowLocal()
	if rng != "" {
		parts := strings.Split(rng, "..")
		if len(parts) != 2 { cobra.CheckErr(fmt.Errorf("invalid --range; expected A..B")) }
		from := mustParseTimeLocal(parts[0])
		to := mustParseTimeLocal(parts[1])
		return from, to
	}
	if today {
		return now, now
	}
	if week {
		wd := int(now.Weekday())
		if wd == 0 { wd = 7 } // make Monday=1..Sunday=7
		monday := now.AddDate(0,0,-(wd-1))
		return monday, monday.AddDate(0,0,6)
	}
	// default: today
	return now, now
}

func fmtHHMM(min int) string {
	h := min / 60
	m := min % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}
