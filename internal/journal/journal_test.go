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

// Additional tests to exercise correction behaviour: idempotency of merges, deterministic ordering,
// and that totals remain consistent before/after corrections.

func TestMergeIdempotency(t *testing.T) {
	// Create two base entries mA and mB and two merge events that attempt to merge them twice.
	input := strings.Join([]string{
		`{"id":"mA","type":"add","ts":"2025-02-01T09:00:00Z","ref":"2025-02-01T09:00:00Z..2025-02-01T10:00:00Z","customer":"X","project":"p"}`,
		`{"id":"mB","type":"add","ts":"2025-02-01T10:00:00Z","ref":"2025-02-01T10:00:00Z..2025-02-01T11:00:00Z","customer":"X","project":"p"}`,
		// first merge (should produce mg)
		`{"id":"mg","type":"merge","ts":"2025-02-01T12:00:00Z","meta":{"targets":"mA,mB"},"note":"first"}`,
		// second merge targeting the same ids (later timestamp) - should be a no-op because targets removed
		`{"id":"mg2","type":"merge","ts":"2025-02-01T13:00:00Z","meta":{"targets":"mA,mB"},"note":"second"}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	// Expect only one merged entry mg and no mA/mB
	ids := map[string]bool{}
	for _, e := range ents {
		ids[e.ID] = true
	}
	if !ids["mg"] {
		t.Fatalf("expected merged entry mg present")
	}
	if ids["mg2"] {
		t.Fatalf("mg2 should not produce a second merged entry")
	}
	if ids["mA"] || ids["mB"] {
		t.Fatalf("mA/mB should be removed after merge")
	}
}

func TestCorrections_OrderingDeterministic(t *testing.T) {
	// Ensure that corrections are applied in chronological order regardless of input order.
	// We'll craft two inputs with the same events in different orders but same timestamps and expect same result.
	base := []string{
		`{"id":"x1","type":"add","ts":"2025-03-10T08:00:00Z","ref":"2025-03-10T08:00:00Z..2025-03-10T10:00:00Z","customer":"C","project":"P"}`,
	}
	// amend then split (amend ts earlier than split)
	eventsA := []string{
		`{"id":"amx","type":"amend","ts":"2025-03-10T10:00:00Z","ref":"x1","note":"amended","meta":{"start":"2025-03-10T08:15:00Z"}}`,
		`{"id":"spx","type":"split","ts":"2025-03-10T11:00:00Z","ref":"x1","meta":{"split_at":"2025-03-10T09:00:00Z","left_note":"L","right_note":"R"}}`,
	}
	// same events but in reverse order (split appears before amend in the file)
	eventsB := []string{
		`{"id":"spx","type":"split","ts":"2025-03-10T11:00:00Z","ref":"x1","meta":{"split_at":"2025-03-10T09:00:00Z","left_note":"L","right_note":"R"}}`,
		`{"id":"amx","type":"amend","ts":"2025-03-10T10:00:00Z","ref":"x1","note":"amended","meta":{"start":"2025-03-10T08:15:00Z"}}`,
	}

	inputA := strings.Join(append(base, eventsA...), "\n")
	inputB := strings.Join(append(base, eventsB...), "\n")

	p := NewParser("")
	entsA, err := p.ParseReader(strings.NewReader(inputA))
	if err != nil {
		t.Fatalf("ParseReader A error: %v", err)
	}
	entsB, err := p.ParseReader(strings.NewReader(inputB))
	if err != nil {
		t.Fatalf("ParseReader B error: %v", err)
	}

	// Normalize by mapping id->entry and comparing windows and notes for expected derived ids.
	mapA := map[string]Entry{}
	for _, e := range entsA {
		mapA[e.ID] = e
	}
	mapB := map[string]Entry{}
	for _, e := range entsB {
		mapB[e.ID] = e
	}

	// Both should contain split outputs spx.L and spx.R and an amended start applied to the left part.
	aLeft, aLok := mapA["spx.L"]
	bLeft, bLok := mapB["spx.L"]
	if !aLok || !bLok {
		t.Fatalf("expected spx.L present in both parses")
	}
	// Compare starts first
	if !aLeft.Start.Equal(bLeft.Start) {
		t.Fatalf("left split start mismatch between parses: A %v B %v", aLeft.Start, bLeft.Start)
	}
	// Safely compare optional end times
	if (aLeft.End == nil) != (bLeft.End == nil) {
		t.Fatalf("left split end nil mismatch between parses: A %v B %v", aLeft.End, bLeft.End)
	}
	if aLeft.End != nil && bLeft.End != nil {
		if !(*aLeft.End).Equal(*bLeft.End) {
			t.Fatalf("left split end mismatch between parses: A %v B %v", *aLeft.End, *bLeft.End)
		}
	}
	// The left split should reflect the amended start (08:15)
	if !aLeft.Start.Equal(mustParse(t, "2025-03-10T08:15:00Z")) {
		t.Fatalf("expected amended start applied to left split, got %v", aLeft.Start)
	}
}

func TestReportingTotalsBeforeAfterCorrections(t *testing.T) {
	// Build base entries: a1 (08:00-09:00), a2 (09:00-11:00)
	// Apply a split on a2 and a merge of a1 with one of resulting parts; totals should remain identical.
	base := []string{
		`{"id":"a1","type":"add","ts":"2025-04-01T08:00:00Z","ref":"2025-04-01T08:00:00Z..2025-04-01T09:00:00Z","customer":"X","project":"p"}`,
		`{"id":"a2","type":"add","ts":"2025-04-01T09:00:00Z","ref":"2025-04-01T09:00:00Z..2025-04-01T11:00:00Z","customer":"X","project":"p"}`,
	}
	corrections := []string{
		// split a2 into 09:00..10:00 and 10:00..11:00
		`{"id":"sp","type":"split","ts":"2025-04-01T12:00:00Z","ref":"a2","meta":{"split_at":"2025-04-01T10:00:00Z","left_note":"part1","right_note":"part2"}}`,
		// merge a1 and sp.L into mg (consolidation) - note this should not change total time
		`{"id":"mg","type":"merge","ts":"2025-04-01T13:00:00Z","meta":{"targets":"a1,sp.L"},"note":"consol"}`,
	}

	// Parse base-only to compute baseline total
	baseInput := strings.Join(base, "\n")
	p := NewParser("")
	baseEntries, err := p.ParseReader(strings.NewReader(baseInput))
	if err != nil {
		t.Fatalf("ParseReader base error: %v", err)
	}
	totalBase := 0
	for _, e := range baseEntries {
		if e.End != nil {
			totalBase += int(e.End.Sub(e.Start).Minutes())
		}
	}

	// Parse base+corrections to compute post-correction total
	allInput := strings.Join(append(base, corrections...), "\n")
	allEntries, err := p.ParseReader(strings.NewReader(allInput))
	if err != nil {
		t.Fatalf("ParseReader all error: %v", err)
	}
	totalAfter := 0
	for _, e := range allEntries {
		if e.End != nil {
			totalAfter += int(e.End.Sub(e.Start).Minutes())
		}
	}

	if totalBase != totalAfter {
		t.Fatalf("expected totals unchanged by corrections, before=%d after=%d", totalBase, totalAfter)
	}
}

// Edge case: amend an open (running) entry. The amend should be able to set an end time
// and update metadata for an entry created by a `start` event that lacks a stop.
func TestAmendOpenEntry(t *testing.T) {
	input := strings.Join([]string{
		// running start (no stop)
		`{"id":"r1","type":"start","ts":"2025-05-01T08:00:00Z","project":"p","activity":"dev"}`,
		// amend the running entry to add an end and change project
		`{"id":"amr","type":"amend","ts":"2025-05-01T12:00:00Z","ref":"r1","project":"p2","meta":{"end":"2025-05-01T11:00:00Z"}}`,
	}, "\n")

	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entry after amend, got %d", len(ents))
	}
	e := ents[0]
	if e.Project != "p2" {
		t.Fatalf("expected project amended to p2, got %q", e.Project)
	}
	if e.End == nil || !e.End.Equal(mustParse(t, "2025-05-01T11:00:00Z")) {
		t.Fatalf("expected end set by amend to 11:00, got %v", e.End)
	}
}

// Edge case: split at exact boundaries must be rejected (split_at must be strictly between start and end).
func TestSplitAtBoundaries(t *testing.T) {
	// ab1 is 09:00..10:00
	base := `{"id":"ab1","type":"add","ts":"2025-06-01T09:00:00Z","ref":"2025-06-01T09:00:00Z..2025-06-01T10:00:00Z","project":"p"}`
	// attempt split at start boundary
	spStart := `{"id":"spb1","type":"split","ts":"2025-06-01T11:00:00Z","ref":"ab1","meta":{"split_at":"2025-06-01T09:00:00Z"}}`
	// attempt split at end boundary
	spEnd := `{"id":"spb2","type":"split","ts":"2025-06-01T11:05:00Z","ref":"ab1","meta":{"split_at":"2025-06-01T10:00:00Z"}}`

	// Non-strict parser should ignore these invalid splits and keep the original entry
	input := strings.Join([]string{base, spStart, spEnd}, "\n")
	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseReader non-strict error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected original entry preserved when splits at boundaries ignored, got %d entries", len(ents))
	}

	// Strict mode should error when encountering invalid split_at
	pStrict := NewParser("")
	pStrict.Strict = true
	_, err = pStrict.ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatalf("expected error in strict mode for splits at boundaries")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError in strict mode for invalid split_at, got %T: %v", err, err)
	}
}

// Edge case: merge with missing targets in strict mode should cause an error; non-strict skips it.
func TestMergeMissingTargetsStrictMode(t *testing.T) {
	// base entry mX
	base := `{"id":"mX","type":"add","ts":"2025-07-01T09:00:00Z","ref":"2025-07-01T09:00:00Z..2025-07-01T10:00:00Z"}`
	// merge referencing non-existent ids foo,bar
	mergeEv := `{"id":"mErr","type":"merge","ts":"2025-07-01T12:00:00Z","meta":{"targets":"foo,bar"}}`

	// Non-strict parser should skip the problematic merge (no error) and keep base entry
	input := strings.Join([]string{base, mergeEv}, "\n")
	p := NewParser("")
	ents, err := p.ParseReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("expected non-strict parser to skip bad merge, got error: %v", err)
	}
	if len(ents) != 1 || ents[0].ID != "mX" {
		t.Fatalf("expected base entry preserved when merge targets missing in non-strict mode, got: %+v", ents)
	}

	// Strict parser should return a ParseError when merge targets missing
	ps := NewParser("")
	ps.Strict = true
	_, err = ps.ParseReader(strings.NewReader(input))
	if err == nil {
		t.Fatalf("expected strict parser to error on merge with missing targets")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError for missing merge targets in strict mode, got %T: %v", err, err)
	}
}
