package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Event represents a single JSONL event written by the CLI journal.
// This mirrors the structure used across the repository for journal files.
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
	Ref      string            `json:"ref,omitempty"` // e.g. "startISO..endISO" for add events
	Meta     map[string]string `json:"meta,omitempty"`
	PrevHash string            `json:"prev_hash,omitempty"`
	Hash     string            `json:"hash,omitempty"`
}

// Entry is the reconstructed time entry built from a sequence of events.
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
	Source   string // optional path where the entry originated
}

// Parser configures how journal files are parsed.
type Parser struct {
	Location *time.Location // timezone (if empty, Local is used)
	Strict   bool           // if true, parsing errors abort with an error
}

// ParseError represents a parsing error with optional file/line context.
type ParseError struct {
	Path string
	Line int
	Err  error
}

func (p *ParseError) Error() string {
	if p == nil || p.Err == nil {
		return "<nil parse error>"
	}
	if p.Path != "" && p.Line > 0 {
		return fmt.Sprintf("parse error %s:%d: %v", p.Path, p.Line, p.Err)
	}
	if p.Path != "" {
		return fmt.Sprintf("parse error %s: %v", p.Path, p.Err)
	}
	if p.Line > 0 {
		return fmt.Sprintf("parse error line %d: %v", p.Line, p.Err)
	}
	return fmt.Sprintf("parse error: %v", p.Err)
}

var ErrInvalidRef = errors.New("invalid ref format; expected startISO..endISO")

// NewParser returns a Parser. If timezone is empty or invalid, local timezone is used.
func NewParser(timezone string) *Parser {
	loc := time.Local
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	return &Parser{Location: loc}
}

// ParseFile opens the given path and parses it as a journal JSONL file.
// The returned entries will have the Entry.Source set to the provided path.
func (p *Parser) ParseFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ents, err := p.parseReaderWithPath(f, path)
	if err != nil {
		// if it's a ParseError and has no Path set, attach it
		if pe, ok := err.(*ParseError); ok && pe.Path == "" {
			pe.Path = path
		}
		return nil, err
	}

	// set Source on each entry
	for i := range ents {
		ents[i].Source = path
	}
	return ents, nil
}

// ParseReader parses entries from an io.Reader containing JSONL events.
// If the receiver (p) is nil, a default parser (local timezone, non-strict) is used.
func (p *Parser) ParseReader(r io.Reader) ([]Entry, error) {
	return p.parseReaderWithPath(r, "")
}

// parseReaderWithPath is the internal implementation that can attach a path to ParseErrors.
func (p *Parser) parseReaderWithPath(r io.Reader, path string) ([]Entry, error) {
	if p == nil {
		p = NewParser("")
	}
	scanner := bufio.NewScanner(r)
	// keep default buffer; should be sufficient for typical JSONL lines in this project
	var events []Event
	line := 0
	for scanner.Scan() {
		line++
		txt := strings.TrimSpace(scanner.Text())
		if txt == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(txt), &ev); err != nil {
			pe := &ParseError{Path: path, Line: line, Err: err}
			if p.Strict {
				return nil, pe
			}
			// skip malformed lines when not strict
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Sort events chronologically to ensure deterministic reconstruction.
	sort.Slice(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })

	var entries []Entry
	var current *Entry
	for _, ev := range events {
		switch ev.Type {
		case "start":
			if current != nil {
				// auto-stop previous at this event timestamp
				cur := ev.TS
				current.End = &cur
				entries = append(entries, *current)
			}
			billable := true
			if ev.Billable != nil {
				billable = *ev.Billable
			}
			current = &Entry{
				ID:       ev.ID,
				Start:    ev.TS,
				Customer: ev.Customer,
				Project:  ev.Project,
				Activity: ev.Activity,
				Billable: billable,
				Notes:    []string{},
				Tags:     ev.Tags,
			}
			if ev.Note != "" {
				current.Notes = append(current.Notes, ev.Note)
			}
		case "note":
			if current != nil {
				current.Notes = append(current.Notes, ev.Note)
			}
		case "stop":
			if current != nil {
				cur := ev.TS
				current.End = &cur
				entries = append(entries, *current)
				current = nil
			}
		case "add":
			billable := true
			if ev.Billable != nil {
				billable = *ev.Billable
			}
			parts := strings.Split(ev.Ref, "..")
			if len(parts) == 2 {
				st, err1 := time.Parse(time.RFC3339, parts[0])
				en, err2 := time.Parse(time.RFC3339, parts[1])
				if err1 == nil && err2 == nil {
					entries = append(entries, Entry{
						ID:       ev.ID,
						Start:    st,
						End:      &en,
						Customer: ev.Customer,
						Project:  ev.Project,
						Activity: ev.Activity,
						Billable: billable,
						Notes:    []string{ev.Note},
						Tags:     ev.Tags,
					})
				} else {
					pe := &ParseError{Path: path, Err: ErrInvalidRef}
					if p.Strict {
						return nil, pe
					}
					// otherwise ignore this malformed add
				}
			} else {
				pe := &ParseError{Path: path, Err: ErrInvalidRef}
				if p.Strict {
					return nil, pe
				}
			}
		// other event types are ignored by reconstruction (amend, pause, resume, etc.)
		default:
			// no-op
		}
	}

	return entries, nil
}

// ParseStream parses entries and returns a channel that emits entries as they are reconstructed.
// The returned error channel receives at most one error (parsing/opening error). Both channels are closed
// when done. If parsing fails, the error is delivered and the entries channel is closed without items.
func (p *Parser) ParseStream(r io.Reader) (<-chan Entry, <-chan error) {
	out := make(chan Entry)
	errc := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errc)

		entries, err := p.ParseReader(r)
		if err != nil {
			errc <- err
			return
		}
		for _, e := range entries {
			out <- e
		}
	}()

	return out, errc
}
