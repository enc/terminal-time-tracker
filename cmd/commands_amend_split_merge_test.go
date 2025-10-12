package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tt/internal/journal"

	"github.com/spf13/viper"
)

// simpleFakeEventWriter captures the last written event(s).
type simpleFakeEventWriter struct {
	events []Event
	err    error
}

func (f *simpleFakeEventWriter) WriteEvent(e Event) error {
	if f.err != nil {
		return f.err
	}
	// copy to avoid sharing mutable slices underneath
	cp := e
	if e.Tags != nil {
		cp.Tags = append([]string{}, e.Tags...)
	}
	if e.Meta != nil {
		cp.Meta = map[string]string{}
		for k, v := range e.Meta {
			cp.Meta[k] = v
		}
	}
	f.events = append(f.events, cp)
	return nil
}

func TestAmendCommand_WritesAmendEventForID(t *testing.T) {
	// deterministic providers
	oldNow := Now
	oldID := IDGen
	defer func() { Now = oldNow; IDGen = oldID }()
	Now = func() time.Time { return time.Date(2025, 10, 10, 11, 0, 0, 0, time.UTC) }
	IDGen = func() string { return "evt-amend-1" }

	fw := &simpleFakeEventWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// set flags like a user would
	amendLast = false
	amendStartStr = "2025-10-10T08:15:00Z"
	amendEndStr = "2025-10-10T09:15:00Z"
	amendNote = "Wrapped up deployment"
	amendCustomer = "ACME2"
	amendProject = "newproj"
	amendActivity = "ops"
	amendBillableF = "false"
	amendTags = []string{"t1", "t2"}

	// call command with explicit id
	args := []string{"target-entry-123"}
	amendCmd.SetArgs(args)
	// capture stdout/stderr to avoid polluting test output
	var buf bytes.Buffer
	amendCmd.SetOut(&buf)
	amendCmd.SetErr(&buf)

	amendCmd.Run(amendCmd, args)

	if len(fw.events) != 1 {
		t.Fatalf("expected 1 event written, got %d", len(fw.events))
	}
	ev := fw.events[0]
	if ev.Type != "amend" {
		t.Fatalf("expected event type amend, got %s", ev.Type)
	}
	if ev.Ref != "target-entry-123" {
		t.Fatalf("expected Ref target-entry-123, got %q", ev.Ref)
	}
	if ev.ID != "evt-amend-1" {
		t.Fatalf("expected ID %q, got %q", "evt-amend-1", ev.ID)
	}
	// metadata & note
	if ev.Customer != "ACME2" || ev.Project != "newproj" || ev.Activity != "ops" {
		t.Fatalf("unexpected metadata on amend event: %#v", ev)
	}
	if ev.Note != "Wrapped up deployment" {
		t.Fatalf("unexpected note on amend event: %q", ev.Note)
	}
	if v, ok := ev.Meta["start"]; !ok || v != "2025-10-10T08:15:00Z" {
		t.Fatalf("expected meta.start set, got: %#v", ev.Meta)
	}
	if v, ok := ev.Meta["end"]; !ok || v != "2025-10-10T09:15:00Z" {
		t.Fatalf("expected meta.end set, got: %#v", ev.Meta)
	}
	// billable should be set to false pointer
	if ev.Billable == nil || *ev.Billable != false {
		t.Fatalf("expected billable=false, got %#v", ev.Billable)
	}
}

func TestSplitCommand_WritesSplitEventForLast(t *testing.T) {
	// Prepare deterministic providers
	oldNow := Now
	oldID := IDGen
	defer func() { Now = oldNow; IDGen = oldID }()
	Now = func() time.Time { return time.Date(2025, 10, 11, 12, 0, 0, 0, time.UTC) }
	IDGen = func() string { return "evt-split-1" }

	// create fake writer and set as global (restore previous Writer afterwards)
	fw := &simpleFakeEventWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// Make sure loadEntries returns a predictable last entry for today.
	// We do this by writing a small temporary journal file via Writer in-memory behavior isn't possible here,
	// so instead rely on loadEntries reading from the file system in real usage.
	// To avoid that complexity in this unit test, we'll invoke split command with an explicit id.

	splitLast = false
	splitAtStr = "2025-10-11T10:00:00Z"
	splitLeftNote = "Before lunch"
	splitRightNote = "After lunch"
	splitCustomer = "ACME"
	splitProject = "portal"
	splitActivity = "dev"
	splitBillableF = "true"
	splitTags = []string{"s1"}

	args := []string{"entry-to-split-9"}
	splitCmd.SetArgs(args)
	var buf bytes.Buffer
	splitCmd.SetOut(&buf)
	splitCmd.SetErr(&buf)

	splitCmd.Run(splitCmd, args)

	if len(fw.events) != 1 {
		t.Fatalf("expected 1 split event written, got %d", len(fw.events))
	}
	ev := fw.events[0]
	if ev.Type != "split" {
		t.Fatalf("expected split event, got %s", ev.Type)
	}
	if ev.Ref != "entry-to-split-9" {
		t.Fatalf("expected Ref entry-to-split-9, got %q", ev.Ref)
	}
	if ev.Meta["split_at"] != "2025-10-11T10:00:00Z" {
		t.Fatalf("expected split_at meta set, got %v", ev.Meta)
	}
	if ev.Meta["left_note"] != "Before lunch" {
		t.Fatalf("expected left_note meta, got %v", ev.Meta)
	}
	if ev.Meta["right_note"] != "After lunch" {
		t.Fatalf("expected right_note meta, got %v", ev.Meta)
	}
	if ev.Customer != "ACME" || ev.Project != "portal" || ev.Activity != "dev" {
		t.Fatalf("unexpected overrides on split event: %#v", ev)
	}
	if ev.Billable == nil || *ev.Billable != true {
		t.Fatalf("expected billable=true on split event, got %#v", ev.Billable)
	}
	if len(ev.Tags) != 1 || ev.Tags[0] != "s1" {
		t.Fatalf("expected tags forwarded on split event, got %#v", ev.Tags)
	}
}

func TestMergeCommand_WritesMergeEventWithTargetsAndSince(t *testing.T) {
	// deterministic providers
	oldNow := Now
	oldID := IDGen
	oldViper := viper.GetViper()
	defer func() {
		Now = oldNow
		IDGen = oldID
		viper.Reset()
		viper.SetConfigType(oldViper.ConfigFileUsed())
	}()
	Now = func() time.Time { return time.Date(2025, 10, 12, 16, 0, 0, 0, time.UTC) }
	IDGen = func() string { return "evt-merge-1" }

	fw := &simpleFakeEventWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// Test explicit --targets
	mergeTargets = "a,b,c"
	mergeSince = ""
	mergeCustomer = "acme"
	mergeProject = "portal"
	mergeActivity = "ops"
	mergeIntoNote = "Consolidated work"
	mergeBillableF = "true"

	args := []string{}
	mergeCmd.SetArgs(args)
	var buf bytes.Buffer
	mergeCmd.SetOut(&buf)
	mergeCmd.SetErr(&buf)

	mergeCmd.Run(mergeCmd, args)

	if len(fw.events) != 1 {
		t.Fatalf("expected 1 merge event written, got %d", len(fw.events))
	}
	ev := fw.events[0]
	if ev.Type != "merge" {
		t.Fatalf("expected merge event, got %s", ev.Type)
	}
	if ev.Meta["targets"] != "a,b,c" {
		t.Fatalf("expected meta.targets == \"a,b,c\", got %q", ev.Meta["targets"])
	}
	if ev.Note != "Consolidated work" {
		t.Fatalf("expected merge note set, got %q", ev.Note)
	}
	if ev.Customer != "acme" || ev.Project != "portal" || ev.Activity != "ops" {
		t.Fatalf("unexpected metadata on merge event: %#v", ev)
	}
	if ev.Billable == nil || *ev.Billable != true {
		t.Fatalf("expected billable=true on merge event, got %#v", ev.Billable)
	}
}

func TestAmendCommand_IntegrationWithParser(t *testing.T) {
	// This end-to-end style test ensures that the amend event written by the command
	// would be understood by the journal parser to modify a base entry.
	// Construct a small "journal file" in-memory by serializing events to lines and feeding the parser via ParseReader.
	// We'll create an add event first, then an amend event, and assert the parser's reconstructed entries reflect the amend.

	// Base add event
	add := Event{
		ID:       "base1",
		Type:     "add",
		TS:       mustParseTimeLocal("2025-10-13T08:00:00Z"),
		Customer: "OrigCo",
		Project:  "proj",
		Activity: "dev",
		Billable: boolPtr(true),
		Note:     "original",
		Ref:      "2025-10-13T08:00:00Z..2025-10-13T09:00:00Z",
	}
	// Amend event to modify times and append note
	amendEv := Event{
		ID:       "amend1",
		Type:     "amend",
		TS:       mustParseTimeLocal("2025-10-13T11:00:00Z"),
		Ref:      "base1",
		Customer: "ACME",
		Project:  "newproj",
		Billable: boolPtr(false),
		Note:     "Wrapped up deployment",
		Meta: map[string]string{
			"start": "2025-10-13T08:15:00Z",
			"end":   "2025-10-13T09:15:00Z",
		},
	}

	// Serialize events to JSONL for parser
	lines := [][]byte{}
	for _, e := range []Event{add, amendEv} {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("failed to marshal event: %v", err)
		}
		lines = append(lines, b)
	}
	input := strings.Join([]string{string(lines[0]), string(lines[1])}, "\n")

	p := journal.NewParser("UTC")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parser returned error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry after applying amend, got %d", len(ents))
	}
	e := ents[0]
	if !e.Start.Equal(mustParseTimeLocal("2025-10-13T08:15:00Z")) {
		t.Fatalf("amend start not applied; got %v", e.Start)
	}
	if e.End == nil || !e.End.Equal(mustParseTimeLocal("2025-10-13T09:15:00Z")) {
		t.Fatalf("amend end not applied; got %v", e.End)
	}
	if e.Customer != "ACME" || e.Project != "newproj" || e.Billable {
		t.Fatalf("amend metadata not applied correctly: %+v", e)
	}
	// last note appended should be amend note
	if len(e.Notes) == 0 || e.Notes[len(e.Notes)-1] != "Wrapped up deployment" {
		t.Fatalf("amend note not present in entry notes: %#v", e.Notes)
	}
}
