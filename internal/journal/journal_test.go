package journal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, v string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("parse time %q: %v", v, err)
	}
	return ts
}

func TestParseReader_Reconstruct(t *testing.T) {
	input := strings.Join([]string{
		// start event
		`{"id":"s1","type":"start","ts":"2025-01-01T09:00:00Z","customer":"ACME","project":"proj","activity":"dev","billable":true,"note":"started"}`,
		// note appended while running
		`{"id":"n1","type":"note","ts":"2025-01-01T09:30:00Z","note":"midway"}`,
		// stop event
		`{"id":"st1","type":"stop","ts":"2025-01-01T10:00:00Z"}`,
		// add event (explicit entry)
		`{"id":"a1","type":"add","ts":"2025-01-02T12:00:00Z","ref":"2025-01-02T08:00:00Z..2025-01-02T09:30:00Z","customer":"ACME","project":"proj","activity":"meeting","billable":false,"note":"ad-hoc"}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	if len(ents) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(ents))
	}

	// first reconstructed entry from start..stop
	e0 := ents[0]
	if !e0.Start.Equal(mustParse(t, "2025-01-01T09:00:00Z")) {
		t.Fatalf("entry0 start mismatch: got %v", e0.Start)
	}
	if e0.End == nil {
		t.Fatalf("entry0 missing end")
	}
	if !e0.End.Equal(mustParse(t, "2025-01-01T10:00:00Z")) {
		t.Fatalf("entry0 end mismatch: got %v", e0.End)
	}
	if e0.Customer != "ACME" || e0.Project != "proj" || e0.Activity != "dev" {
		t.Fatalf("entry0 metadata mismatch: %+v", e0)
	}
	if !e0.Billable {
		t.Fatalf("entry0 should be billable")
	}
	if len(e0.Notes) != 2 || e0.Notes[0] != "started" || e0.Notes[1] != "midway" {
		t.Fatalf("entry0 notes mismatch: %#v", e0.Notes)
	}

	// second entry from add
	e1 := ents[1]
	if !e1.Start.Equal(mustParse(t, "2025-01-02T08:00:00Z")) {
		t.Fatalf("entry1 start mismatch: got %v", e1.Start)
	}
	if e1.End == nil {
		t.Fatalf("entry1 missing end")
	}
	if !e1.End.Equal(mustParse(t, "2025-01-02T09:30:00Z")) {
		t.Fatalf("entry1 end mismatch: got %v", e1.End)
	}
	if e1.Billable {
		t.Fatalf("entry1 should NOT be billable")
	}
	if len(e1.Notes) != 1 || e1.Notes[0] != "ad-hoc" {
		t.Fatalf("entry1 notes mismatch: %#v", e1.Notes)
	}
}

func TestParseFile_SetsSource(t *testing.T) {
	content := `{"id":"s1","type":"start","ts":"2025-01-03T09:00:00Z","customer":"C","project":"P","activity":"A","billable":true}
{"id":"st1","type":"stop","ts":"2025-01-03T10:00:00Z"}`

	tmpf, err := os.CreateTemp("", "journal_test_*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := tmpf.Name()
	tmpf.Close()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	defer os.Remove(path)

	p := NewParser("")
	ents, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ents))
	}
	if ents[0].Source != path {
		t.Fatalf("expected source %q, got %q", path, ents[0].Source)
	}
}

func TestParseReader_StrictMode_MalformedJSON(t *testing.T) {
	// first line valid, second line malformed JSON
	input := strings.Join([]string{
		`{"id":"s1","type":"start","ts":"2025-01-04T09:00:00Z"}`,
		`{bad json line}`,
	}, "\n")

	p := NewParser("")
	p.Strict = true

	_, err := p.ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatalf("expected error in strict mode for malformed JSON")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
	if pe.Line != 2 {
		t.Fatalf("expected parse error line 2, got %d", pe.Line)
	}
}

func TestParseReader_StrictMode_InvalidAddRef(t *testing.T) {
	// add event with invalid ref should error in strict mode
	input := `{"id":"a1","type":"add","ts":"2025-01-05T09:00:00Z","ref":"not-a-valid-ref"}`
	p := NewParser("")
	p.Strict = true

	_, err := p.ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatalf("expected error for invalid add ref in strict mode")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
	if pe.Err != ErrInvalidRef {
		t.Fatalf("expected ErrInvalidRef inside ParseError, got %v", pe.Err)
	}
}

func TestParseStream_EmitsEntries(t *testing.T) {
	input := strings.Join([]string{
		`{"id":"s1","type":"start","ts":"2025-01-06T09:00:00Z","customer":"X"}`,
		`{"id":"st1","type":"stop","ts":"2025-01-06T10:00:00Z"}`,
	}, "\n")

	p := NewParser("")
	ch, errc := p.ParseStream(strings.NewReader(input))

	var got []Entry
	for e := range ch {
		got = append(got, e)
	}
	// ensure errc has no error
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("unexpected parse error from stream: %v", err)
		}
	default:
		// no error
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 entry from stream, got %d", len(got))
	}
	if got[0].Customer != "X" {
		t.Fatalf("unexpected customer in streamed entry: %q", got[0].Customer)
	}
}

func TestOutOfOrderEventsSorted(t *testing.T) {
	// stop appears before start; parser must sort by TS to reconstruct correctly
	input := strings.Join([]string{
		`{"id":"st1","type":"stop","ts":"2025-01-07T10:00:00Z"}`,
		`{"id":"s1","type":"start","ts":"2025-01-07T09:00:00Z","project":"p","activity":"a"}`,
	}, "\n")
	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ents))
	}
	if !ents[0].Start.Equal(mustParse(t, "2025-01-07T09:00:00Z")) || ents[0].End == nil || !ents[0].End.Equal(mustParse(t, "2025-01-07T10:00:00Z")) {
		t.Fatalf("unexpected reconstructed window: %v..%v", ents[0].Start, ents[0].End)
	}
}

func TestUnknownAndOrphanNoteIgnored(t *testing.T) {
	// Orphan note (no current running entry) should be ignored.
	// Unknown types like pause/resume are ignored by reconstruction.
	input := strings.Join([]string{
		`{"id":"n0","type":"note","ts":"2025-01-08T08:30:00Z","note":"ignored"}`,
		`{"id":"p1","type":"pause","ts":"2025-01-08T08:45:00Z"}`,
		`{"id":"s1","type":"start","ts":"2025-01-08T09:00:00Z","project":"proj","activity":"dev"}`,
		`{"id":"r1","type":"resume","ts":"2025-01-08T09:10:00Z"}`,
		`{"id":"n1","type":"note","ts":"2025-01-08T09:15:00Z","note":"kept"}`,
		`{"id":"st1","type":"stop","ts":"2025-01-08T10:00:00Z"}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ents))
	}
	if len(ents[0].Notes) != 1 || ents[0].Notes[0] != "kept" {
		t.Fatalf("expected only in-run note to be kept, got %#v", ents[0].Notes)
	}
}

func TestMultipleStartsAutoStop(t *testing.T) {
	// Second start auto-stops the first entry at its timestamp
	input := strings.Join([]string{
		`{"id":"s1","type":"start","ts":"2025-01-09T09:00:00Z","project":"p","activity":"a"}`,
		`{"id":"s2","type":"start","ts":"2025-01-09T09:30:00Z","project":"p","activity":"a"}`,
		`{"id":"st2","type":"stop","ts":"2025-01-09T10:00:00Z"}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(ents))
	}
	// First entry: 09:00..09:30
	if !ents[0].Start.Equal(mustParse(t, "2025-01-09T09:00:00Z")) || ents[0].End == nil || !ents[0].End.Equal(mustParse(t, "2025-01-09T09:30:00Z")) {
		t.Fatalf("first auto-stopped window mismatch: %v..%v", ents[0].Start, ents[0].End)
	}
	// Second entry: 09:30..10:00
	if !ents[1].Start.Equal(mustParse(t, "2025-01-09T09:30:00Z")) || ents[1].End == nil || !ents[1].End.Equal(mustParse(t, "2025-01-09T10:00:00Z")) {
		t.Fatalf("second window mismatch: %v..%v", ents[1].Start, ents[1].End)
	}
}

func TestAddInvalidRefNonStrictSkips(t *testing.T) {
	// Non-strict mode should skip bad add refs but keep valid ones
	input := strings.Join([]string{
		`{"id":"aBad","type":"add","ts":"2025-01-10T12:00:00Z","ref":"bad-ref"}`,
		`{"id":"aGood","type":"add","ts":"2025-01-10T12:05:00Z","ref":"2025-01-10T08:00:00Z..2025-01-10T09:00:00Z","project":"p","activity":"meet"}`,
	}, "\n")
	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry (skip bad add), got %d", len(ents))
	}
	if !ents[0].Start.Equal(mustParse(t, "2025-01-10T08:00:00Z")) || ents[0].End == nil || !ents[0].End.Equal(mustParse(t, "2025-01-10T09:00:00Z")) {
		t.Fatalf("valid add not reconstructed correctly: %v..%v", ents[0].Start, ents[0].End)
	}
}

func TestParseFile_StrictError_PopulatesPathAndLine(t *testing.T) {
	content := strings.Join([]string{
		`{"id":"s1","type":"start","ts":"2025-01-11T09:00:00Z"}`,
		`{bad json line}`,
	}, "\n")
	tmpf, err := os.CreateTemp("", "journal_path_line_*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	path := tmpf.Name()
	tmpf.Close()
	defer os.Remove(path)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("")
	p.Strict = true
	_, perr := p.ParseFile(path)
	if perr == nil {
		t.Fatalf("expected strict parse error")
	}
	var pe *ParseError
	if !errors.As(perr, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", perr, perr)
	}
	if pe.Path != path || pe.Line != 2 {
		t.Fatalf("expected path=%q line=2, got path=%q line=%d", path, pe.Path, pe.Line)
	}
}

func TestTagsAndBillableDefaults(t *testing.T) {
	input := strings.Join([]string{
		// start with billable omitted -> defaults to true, tags kept
		`{"id":"s1","type":"start","ts":"2025-01-12T09:00:00Z","project":"p","activity":"a","tags":["t1","t2"]}`,
		`{"id":"st1","type":"stop","ts":"2025-01-12T10:00:00Z"}`,
		// add with explicit billable=false and tags
		`{"id":"a1","type":"add","ts":"2025-01-12T12:00:00Z","ref":"2025-01-12T12:00:00Z..2025-01-12T12:30:00Z","project":"p2","activity":"b","billable":false,"tags":["u1"]}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(ents))
	}
	if !ents[0].Billable || len(ents[0].Tags) != 2 || ents[0].Tags[0] != "t1" || ents[0].Tags[1] != "t2" {
		t.Fatalf("first entry billable default or tags mismatch: billable=%v tags=%#v", ents[0].Billable, ents[0].Tags)
	}
	if ents[1].Billable || len(ents[1].Tags) != 1 || ents[1].Tags[0] != "u1" {
		t.Fatalf("second entry billable/tags mismatch: billable=%v tags=%#v", ents[1].Billable, ents[1].Tags)
	}
}

func TestParseReader_AmendSplitMerge(t *testing.T) {
	// This test exercises the append-only corrections: amend, split, merge.
	// 1. add base entry e1 (08:00-09:00) and amend it (change metadata & bounds)
	// 2. add base entry e2 (09:00-11:00) and split it into sp1.L and sp1.R at 10:00
	// 3. add m1 (13:00-14:00) and m2 (14:00-15:00) and merge them into mg1
	input := strings.Join([]string{
		// base entries
		`{"id":"e1","type":"add","ts":"2025-01-13T08:00:00Z","ref":"2025-01-13T08:00:00Z..2025-01-13T09:00:00Z","customer":"OrigCo","project":"origproj","activity":"work","billable":true,"note":"orig"}`,
		`{"id":"e2","type":"add","ts":"2025-01-13T09:00:00Z","ref":"2025-01-13T09:00:00Z..2025-01-13T11:00:00Z","customer":"ACME","project":"portal","activity":"dev","billable":true}`,
		`{"id":"m1","type":"add","ts":"2025-01-13T13:00:00Z","ref":"2025-01-13T13:00:00Z..2025-01-13T14:00:00Z","customer":"acme","project":"portal","activity":"ops"}`,
		`{"id":"m2","type":"add","ts":"2025-01-13T14:00:00Z","ref":"2025-01-13T14:00:00Z..2025-01-13T15:00:00Z","customer":"acme","project":"portal","activity":"ops"}`,
		// corrections: amend e1
		`{"id":"am1","type":"amend","ts":"2025-01-13T11:00:00Z","ref":"e1","customer":"ACME2","project":"newproj","billable":false,"note":"Wrapped up deployment","meta":{"start":"2025-01-13T08:15:00Z","end":"2025-01-13T09:15:00Z"}}`,
		// split e2 at 10:00 with left/right notes
		`{"id":"sp1","type":"split","ts":"2025-01-13T12:00:00Z","ref":"e2","meta":{"split_at":"2025-01-13T10:00:00Z","left_note":"Before lunch","right_note":"After lunch"}}`,
		// merge m1 and m2 into mg1
		`{"id":"mg1","type":"merge","ts":"2025-01-13T16:00:00Z","note":"Consolidated work","meta":{"targets":"m1,m2"}}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	// Build a map of id->entry for assertions
	m := make(map[string]Entry)
	for _, e := range ents {
		m[e.ID] = e
	}

	// Expect amended e1 to exist with updated bounds, metadata, billable=false and appended note
	e1, ok := m["e1"]
	if !ok {
		t.Fatalf("expected amended entry e1 present")
	}
	if !e1.Start.Equal(mustParse(t, "2025-01-13T08:15:00Z")) {
		t.Fatalf("e1 start not amended: %v", e1.Start)
	}
	if e1.End == nil || !e1.End.Equal(mustParse(t, "2025-01-13T09:15:00Z")) {
		t.Fatalf("e1 end not amended: %v", e1.End)
	}
	if e1.Customer != "ACME2" || e1.Project != "newproj" {
		t.Fatalf("e1 metadata not amended: %v %v", e1.Customer, e1.Project)
	}
	if e1.Billable {
		t.Fatalf("e1 billable should be false after amend")
	}
	// notes: original "orig" + "Wrapped up deployment"
	if len(e1.Notes) < 2 || e1.Notes[len(e1.Notes)-1] != "Wrapped up deployment" {
		t.Fatalf("e1 notes expected appended amend note, got: %#v", e1.Notes)
	}

	// Expect split produced sp1.L and sp1.R and removed original e2
	if _, exists := m["e2"]; exists {
		t.Fatalf("original e2 should be removed after split")
	}
	left, lok := m["sp1.L"]
	right, rok := m["sp1.R"]
	if !lok || !rok {
		t.Fatalf("expected sp1.L and sp1.R entries present, got L=%v R=%v", lok, rok)
	}
	if !left.Start.Equal(mustParse(t, "2025-01-13T09:00:00Z")) || left.End == nil || !left.End.Equal(mustParse(t, "2025-01-13T10:00:00Z")) {
		t.Fatalf("sp1.L window incorrect: %v..%v", left.Start, left.End)
	}
	if !right.Start.Equal(mustParse(t, "2025-01-13T10:00:00Z")) || right.End == nil || !right.End.Equal(mustParse(t, "2025-01-13T11:00:00Z")) {
		t.Fatalf("sp1.R window incorrect: %v..%v", right.Start, right.End)
	}
	// notes from split meta
	if len(left.Notes) == 0 || left.Notes[0] != "Before lunch" {
		t.Fatalf("sp1.L note missing or wrong: %#v", left.Notes)
	}
	if len(right.Notes) == 0 || right.Notes[0] != "After lunch" {
		t.Fatalf("sp1.R note missing or wrong: %#v", right.Notes)
	}

	// Expect merged mg1 present and m1/m2 removed
	if _, exists := m["m1"]; exists {
		t.Fatalf("m1 should be removed after merge")
	}
	if _, exists := m["m2"]; exists {
		t.Fatalf("m2 should be removed after merge")
	}
	mg, mek := m["mg1"]
	if !mek {
		t.Fatalf("merged mg1 missing")
	}
	if !mg.Start.Equal(mustParse(t, "2025-01-13T13:00:00Z")) || mg.End == nil || !mg.End.Equal(mustParse(t, "2025-01-13T15:00:00Z")) {
		t.Fatalf("mg1 window incorrect: %v..%v", mg.Start, mg.End)
	}
	// merged note should include the merge note
	found := false
	for _, n := range mg.Notes {
		if n == "Consolidated work" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("mg1 notes should include merge note, got: %#v", mg.Notes)
	}
}
