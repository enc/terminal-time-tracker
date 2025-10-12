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
	Type     string            `json:"type"` // start|stop|add|amend|split|merge|pause|resume|note
	TS       time.Time         `json:"ts"`
	User     string            `json:"user,omitempty"`
	Customer string            `json:"customer,omitempty"`
	Project  string            `json:"project,omitempty"`
	Activity string            `json:"activity,omitempty"`
	Billable *bool             `json:"billable,omitempty"`
	Note     string            `json:"note,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Ref      string            `json:"ref,omitempty"` // e.g. "startISO..endISO" for add events or target entry id for amend/split
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
// This implementation is append-only friendly: it collects base events (start/stop/add/notes)
// and also collects correction events (amend/split/merge). After building base entries it applies
// corrections in chronological order to produce the effective view without mutating historical events.
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

	var baseEntries []Entry
	var current *Entry

	// collect correction events to apply after base reconstruction
	var corrections []Event

	for _, ev := range events {
		switch ev.Type {
		case "start":
			if current != nil {
				// auto-stop previous at this event timestamp
				cur := ev.TS
				current.End = &cur
				baseEntries = append(baseEntries, *current)
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
				baseEntries = append(baseEntries, *current)
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
					baseEntries = append(baseEntries, Entry{
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
		case "amend", "split", "merge":
			// collect and apply later
			corrections = append(corrections, ev)
		default:
			// ignore other event types (pause/resume etc.)
		}
	}

	// if a start is still open at EOF, keep it open (no end)
	if current != nil {
		baseEntries = append(baseEntries, *current)
	}

	// apply corrections (amend/split/merge) in chronological order
	finalEntries, err := applyCorrections(p, path, baseEntries, corrections)
	if err != nil {
		return nil, err
	}

	// sort final entries by start time for deterministic output
	sort.Slice(finalEntries, func(i, j int) bool {
		return finalEntries[i].Start.Before(finalEntries[j].Start)
	})

	return finalEntries, nil
}

// applyCorrections applies append-only correction events to a slice of base entries.
// Supported correction semantics:
//
//   - amend: ev.Ref should hold the target entry ID (or meta["target"]). Fields present
//     on the amend event override the target's fields (customer,project,activity,billable).
//     Meta keys "start" and "end" may contain RFC3339 times to adjust boundaries.
//     ev.Note, ev.Tags will be appended/replaced respectively.
//
//   - split: ev.Ref identifies the target entry. Meta "split_at" must be an RFC3339 time
//     strictly between the target start and end. Two new entries are created with IDs
//     derived from ev.ID (suffixes ".L" and ".R"). Optional meta keys "left_note" and
//     "right_note" are attached. The original target entry is removed from the effective view.
//
//   - merge: meta["targets"] contains a comma-separated list of entry IDs to merge.
//     A new entry with ID ev.ID is created spanning min(start) .. max(end) of targets.
//     Targets are removed from the effective view. ev.Customer/Project/Activity/Billable
//     override if present; otherwise first non-empty from targets is used. Notes are concatenated.
//
// Errors encountered while parsing correction metadata will cause ParseError when p.Strict=true,
// otherwise problematic correction events are skipped.
func applyCorrections(p *Parser, path string, base []Entry, corrections []Event) ([]Entry, error) {
	// build map id -> *Entry for easy updates; copy values so we can take addresses
	entryMap := make(map[string]*Entry, len(base))
	for i := range base {
		e := base[i] // copy
		entryMap[e.ID] = &e
	}

	// helper to remove an id from map
	removeIDs := func(ids []string) {
		for _, id := range ids {
			delete(entryMap, id)
		}
	}

	// apply corrections in order
	for _, ev := range corrections {
		switch ev.Type {
		case "amend":
			target := ev.Ref
			if target == "" && ev.Meta != nil {
				target = ev.Meta["target"]
			}
			if target == "" {
				pe := &ParseError{Path: path, Err: errors.New("amend event missing target")}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			ent, ok := entryMap[target]
			if !ok {
				// nothing to amend
				pe := &ParseError{Path: path, Err: fmt.Errorf("amend target not found: %s", target)}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			// apply time adjustments from meta
			if ev.Meta != nil {
				if s, ok := ev.Meta["start"]; ok && s != "" {
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						ent.Start = t
					} else if p.Strict {
						return nil, &ParseError{Path: path, Err: err}
					}
				}
				if e, ok := ev.Meta["end"]; ok && e != "" {
					if t, err := time.Parse(time.RFC3339, e); err == nil {
						ent.End = &t
					} else if p.Strict {
						return nil, &ParseError{Path: path, Err: err}
					}
				}
			}
			// override metadata fields if present on amend event
			if ev.Customer != "" {
				ent.Customer = ev.Customer
			}
			if ev.Project != "" {
				ent.Project = ev.Project
			}
			if ev.Activity != "" {
				ent.Activity = ev.Activity
			}
			if ev.Billable != nil {
				ent.Billable = *ev.Billable
			}
			// tags: if non-empty, replace; otherwise keep existing
			if len(ev.Tags) > 0 {
				ent.Tags = ev.Tags
			}
			// append note if provided
			if ev.Note != "" {
				ent.Notes = append(ent.Notes, ev.Note)
			}
		case "split":
			target := ev.Ref
			if target == "" && ev.Meta != nil {
				target = ev.Meta["target"]
			}
			if target == "" {
				pe := &ParseError{Path: path, Err: errors.New("split event missing target")}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			ent, ok := entryMap[target]
			if !ok {
				pe := &ParseError{Path: path, Err: fmt.Errorf("split target not found: %s", target)}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			if ent.End == nil {
				pe := &ParseError{Path: path, Err: fmt.Errorf("split target has no end: %s", target)}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			var splitAtStr string
			if ev.Meta != nil {
				splitAtStr = ev.Meta["split_at"]
			}
			if splitAtStr == "" {
				pe := &ParseError{Path: path, Err: errors.New("split event missing split_at")}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			splitAt, err := time.Parse(time.RFC3339, splitAtStr)
			if err != nil {
				if p.Strict {
					return nil, &ParseError{Path: path, Err: err}
				}
				continue
			}
			if !(ent.Start.Before(splitAt) && splitAt.Before(*ent.End)) {
				pe := &ParseError{Path: path, Err: fmt.Errorf("split_at not within entry bounds for target %s", target)}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			// create left and right entries using ev.ID as base to avoid collisions
			leftID := ev.ID + ".L"
			rightID := ev.ID + ".R"

			left := Entry{
				ID:       leftID,
				Start:    ent.Start,
				End:      &splitAt,
				Customer: ent.Customer,
				Project:  ent.Project,
				Activity: ent.Activity,
				Billable: ent.Billable,
				Notes:    []string{},
				Tags:     ent.Tags,
			}
			right := Entry{
				ID:       rightID,
				Start:    splitAt,
				End:      ent.End,
				Customer: ent.Customer,
				Project:  ent.Project,
				Activity: ent.Activity,
				Billable: ent.Billable,
				Notes:    []string{},
				Tags:     ent.Tags,
			}
			// allow overrides on split event
			if ev.Customer != "" {
				left.Customer = ev.Customer
				right.Customer = ev.Customer
			}
			if ev.Project != "" {
				left.Project = ev.Project
				right.Project = ev.Project
			}
			if ev.Activity != "" {
				left.Activity = ev.Activity
				right.Activity = ev.Activity
			}
			if ev.Billable != nil {
				left.Billable = *ev.Billable
				right.Billable = *ev.Billable
			}
			// attach left/right notes from meta if provided, otherwise inherit none
			if ev.Meta != nil {
				if ln := ev.Meta["left_note"]; ln != "" {
					left.Notes = append(left.Notes, ln)
				}
				if rn := ev.Meta["right_note"]; rn != "" {
					right.Notes = append(right.Notes, rn)
				}
			}
			// remove original and add new ones
			removeIDs([]string{target})
			entryMap[left.ID] = &left
			entryMap[right.ID] = &right
		case "merge":
			// targets list must be in meta["targets"] as comma-separated ids
			if ev.Meta == nil {
				pe := &ParseError{Path: path, Err: errors.New("merge event missing targets")}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			targStr := ev.Meta["targets"]
			if targStr == "" {
				pe := &ParseError{Path: path, Err: errors.New("merge event missing targets")}
				if p.Strict {
					return nil, pe
				}
				continue
			}
			parts := strings.Split(targStr, ",")
			var found []*Entry
			for _, tid := range parts {
				tid = strings.TrimSpace(tid)
				if tid == "" {
					continue
				}
				if e, ok := entryMap[tid]; ok {
					found = append(found, e)
				}
			}
			if len(found) == 0 {
				pe := &ParseError{Path: path, Err: fmt.Errorf("merge: no targets found for event %s", ev.ID)}
				if p.Strict {
					return nil, pe
				}
				continue
			}

			// Harden validation: disallow merging entries that span different customers or projects
			// unless the merge event explicitly provides an override (ev.Customer/ev.Project).
			// We treat only differing non-empty values as a conflict; if all are empty or identical it's fine.
			if ev.Customer == "" {
				custSet := map[string]struct{}{}
				for _, e := range found {
					if e.Customer != "" {
						custSet[e.Customer] = struct{}{}
					}
				}
				if len(custSet) > 1 {
					pe := &ParseError{Path: path, Err: fmt.Errorf("merge targets have conflicting customers; provide an override via event customer")}
					if p.Strict {
						return nil, pe
					}
					// In non-strict mode, skip the problematic merge
					continue
				}
			}
			if ev.Project == "" {
				projSet := map[string]struct{}{}
				for _, e := range found {
					if e.Project != "" {
						projSet[e.Project] = struct{}{}
					}
				}
				if len(projSet) > 1 {
					pe := &ParseError{Path: path, Err: fmt.Errorf("merge targets have conflicting projects; provide an override via event project")}
					if p.Strict {
						return nil, pe
					}
					// In non-strict mode, skip the problematic merge
					continue
				}
			}

			// compute min start and max end
			minStart := found[0].Start
			var maxEnd *time.Time
			for _, e := range found {
				if e.Start.Before(minStart) {
					minStart = e.Start
				}
				if e.End != nil {
					if maxEnd == nil || e.End.After(*maxEnd) {
						// copy value
						t := *e.End
						maxEnd = &t
					}
				}
			}
			merged := Entry{
				ID:       ev.ID,
				Start:    minStart,
				End:      maxEnd,
				Customer: "",
				Project:  "",
				Activity: "",
				Billable: false,
				Notes:    []string{},
				Tags:     []string{},
			}
			// choose metadata: event overrides, otherwise first non-empty from targets
			if ev.Customer != "" {
				merged.Customer = ev.Customer
			} else {
				for _, e := range found {
					if e.Customer != "" {
						merged.Customer = e.Customer
						break
					}
				}
			}
			if ev.Project != "" {
				merged.Project = ev.Project
			} else {
				for _, e := range found {
					if e.Project != "" {
						merged.Project = e.Project
						break
					}
				}
			}
			if ev.Activity != "" {
				merged.Activity = ev.Activity
			} else {
				for _, e := range found {
					if e.Activity != "" {
						merged.Activity = e.Activity
						break
					}
				}
			}
			// billable: event override else any target billable true
			if ev.Billable != nil {
				merged.Billable = *ev.Billable
			} else {
				for _, e := range found {
					if e.Billable {
						merged.Billable = true
						break
					}
				}
			}
			// combine notes and tags
			for _, e := range found {
				merged.Notes = append(merged.Notes, e.Notes...)
				merged.Tags = append(merged.Tags, e.Tags...)
			}
			if ev.Note != "" {
				merged.Notes = append(merged.Notes, ev.Note)
			}
			// remove targets and insert merged
			var targetsToRemove []string
			for _, e := range found {
				targetsToRemove = append(targetsToRemove, e.ID)
			}
			removeIDs(targetsToRemove)
			entryMap[merged.ID] = &merged
		}
	}

	// produce final slice from map
	var out []Entry
	for _, e := range entryMap {
		out = append(out, *e)
	}
	return out, nil
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
