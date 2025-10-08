package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Helpers for building test journals ---

func mustWriteFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func canonicalHashFor(e Event, prev string) string {
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
		PrevHash: prev,
	}
	j, _ := json.Marshal(cp)
	h := sha256.Sum256(j)
	return hex.EncodeToString(h[:])
}

func legacyHashFor(e Event, prev string) string {
	legacy := map[string]any{
		"id":        e.ID,
		"type":      e.Type,
		"ts":        e.TS.Format(time.RFC3339Nano),
		"user":      e.User,
		"customer":  e.Customer,
		"project":   e.Project,
		"activity":  e.Activity,
		"billable":  e.Billable,
		"note":      e.Note,
		"tags":      e.Tags,
		"ref":       e.Ref,
		"prev_hash": prev,
	}
	j, _ := json.Marshal(legacy)
	h := sha256.Sum256(j)
	return hex.EncodeToString(h[:])
}

func writeEventsJSONL(t *testing.T, path string, events []Event) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	mustWriteFile(t, path, buf.String())
}

func makeBaseEvent(id string, ts time.Time) Event {
	return Event{
		ID:       id,
		Type:     "start",
		TS:       ts,
		Customer: "acme",
		Project:  "proj",
		Activity: "dev",
		Tags:     []string{"t1"},
	}
}

func buildCanonicalChain(t *testing.T, base []Event) ([]Event, string) {
	t.Helper()
	prev := ""
	out := make([]Event, 0, len(base))
	for _, e := range base {
		h := canonicalHashFor(e, prev)
		e.PrevHash = prev
		e.Hash = h
		out = append(out, e)
		prev = h
	}
	return out, prev
}

func buildLegacyChain(t *testing.T, base []Event) ([]Event, string) {
	t.Helper()
	prev := ""
	out := make([]Event, 0, len(base))
	for _, e := range base {
		h := legacyHashFor(e, prev)
		e.PrevHash = prev
		e.Hash = h
		out = append(out, e)
		prev = h
	}
	return out, prev
}

// --- Tests for verifyDay ---

func TestVerifyDay_Canonical(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-01.jsonl")
	ts := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)

	base := []Event{
		makeBaseEvent("e1", ts),
		makeBaseEvent("e2", ts.Add(time.Minute)),
	}
	evs, endHash := buildCanonicalChain(t, base)
	writeEventsJSONL(t, path, evs)
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	mustWriteFile(t, anchorPath, endHash+"\n")

	var out bytes.Buffer
	ok := verifyDay(path, &out)
	if !ok {
		t.Fatalf("verifyDay expected OK, got failure. Output:\n%s", out.String())
	}
	// ensure it compared anchor at end (informational message)
	if !strings.Contains(out.String(), "anchor present") {
		t.Fatalf("expected info about anchor, got: %s", out.String())
	}
}

func TestVerifyDay_LegacyAccepted(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-02.jsonl")
	ts := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)

	base := []Event{
		makeBaseEvent("e1", ts),
		makeBaseEvent("e2", ts.Add(2*time.Minute)),
	}
	evs, endLegacy := buildLegacyChain(t, base)
	writeEventsJSONL(t, path, evs)
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	mustWriteFile(t, anchorPath, endLegacy+"\n")

	var out bytes.Buffer
	ok := verifyDay(path, &out)
	if !ok {
		t.Fatalf("verifyDay expected OK for legacy chain, got failure. Output:\n%s", out.String())
	}
	// Should mention legacy match at least once
	if !strings.Contains(out.String(), "legacy hash matched") {
		t.Fatalf("expected legacy hash matched info, got: %s", out.String())
	}
}

func TestVerifyDay_Mismatch(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-03.jsonl")
	ts := time.Date(2025, 1, 3, 11, 0, 0, 0, time.UTC)

	e := makeBaseEvent("e1", ts)
	e.PrevHash = ""
	e.Hash = "deadbeef" // incorrect hash
	writeEventsJSONL(t, path, []Event{e})

	var out bytes.Buffer
	ok := verifyDay(path, &out)
	if ok {
		t.Fatalf("verifyDay expected failure for mismatch, got OK")
	}
	if !strings.Contains(out.String(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got: %s", out.String())
	}
}

// --- Tests for repairDay ---

func TestRepairDay_DryRun_NoChange(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-04.jsonl")
	ts := time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC)

	base := []Event{
		makeBaseEvent("e1", ts),
		makeBaseEvent("e2", ts.Add(3*time.Minute)),
	}
	evs, endCanonical := buildCanonicalChain(t, base)
	writeEventsJSONL(t, path, evs)
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	mustWriteFile(t, anchorPath, endCanonical+"\n")

	var out bytes.Buffer
	changed, wrote, err := repairDay(path, true, false, &out)
	if err != nil {
		t.Fatalf("repairDay error: %v", err)
	}
	if changed || wrote {
		t.Fatalf("expected no change and no .repair written, got changed=%v wrote=%v; out=%s", changed, wrote, out.String())
	}
	// ensure no .repair files exist
	if _, err := os.Stat(path + ".repair"); err == nil {
		t.Fatalf("unexpected .repair file written")
	}
	if _, err := os.Stat(anchorPath + ".repair"); err == nil {
		t.Fatalf("unexpected .hash.repair file written")
	}
}

func TestRepairDay_DryRun_MigrateLegacy(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-05.jsonl")
	ts := time.Date(2025, 1, 5, 13, 0, 0, 0, time.UTC)

	base := []Event{
		makeBaseEvent("e1", ts),
		makeBaseEvent("e2", ts.Add(5*time.Minute)),
	}
	evsLegacy, _ := buildLegacyChain(t, base)
	writeEventsJSONL(t, path, evsLegacy)

	var out bytes.Buffer
	changed, wrote, err := repairDay(path, true, false, &out)
	if err != nil {
		t.Fatalf("repairDay error: %v", err)
	}
	if !changed || !wrote {
		t.Fatalf("expected changes and .repair files; got changed=%v wrote=%v; out=%s", changed, wrote, out.String())
	}
	// Read proposed files and validate they are canonical and chained
	repairPath := path + ".repair"
	anchorRepairPath := strings.TrimSuffix(path, ".jsonl") + ".hash.repair"
	repContent := readFileString(t, repairPath)
	// parse events
	var got []Event
	sc := bufio.NewScanner(strings.NewReader(repContent))
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal([]byte(sc.Text()), &e); err != nil {
			t.Fatalf("unmarshal repaired event: %v", err)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan repaired: %v", err)
	}
	// validate canonical chain
	prev := ""
	for i, e := range got {
		wantH := canonicalHashFor(e, prev)
		if e.Hash != wantH {
			t.Fatalf("repaired line %d has wrong hash: got %s want %s", i+1, e.Hash, wantH)
		}
		if e.PrevHash != prev {
			t.Fatalf("repaired line %d has wrong prev_hash: got %q want %q", i+1, e.PrevHash, prev)
		}
		prev = wantH
	}
	// validate anchor proposal matches end-of-chain
	anch := strings.TrimSpace(readFileString(t, anchorRepairPath))
	if anch != prev {
		t.Fatalf(".hash.repair mismatch: got %s want %s", anch, prev)
	}
}

func TestRepairDay_Apply(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "2025-01-06.jsonl")
	ts := time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC)

	base := []Event{
		makeBaseEvent("e1", ts),
		makeBaseEvent("e2", ts.Add(7*time.Minute)),
	}
	evsLegacy, _ := buildLegacyChain(t, base)
	writeEventsJSONL(t, path, evsLegacy)

	// create an existing (wrong) anchor to test backup + overwrite
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	mustWriteFile(t, anchorPath, "OLDANCHOR\n")

	var out bytes.Buffer
	changed, wrote, err := repairDay(path, false, true, &out)
	if err != nil {
		t.Fatalf("repairDay apply error: %v", err)
	}
	if !changed || wrote {
		t.Fatalf("expected changed=true, wrote=false on apply; got changed=%v wrote=%v; out=%s", changed, wrote, out.String())
	}

	// original .jsonl should be backed up and replaced
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("expected .bak backup for original, err=%v", err)
	}
	// read applied file and verify canonical chain
	applied := readFileString(t, path)
	var got []Event
	sc := bufio.NewScanner(strings.NewReader(applied))
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal([]byte(sc.Text()), &e); err != nil {
			t.Fatalf("unmarshal applied event: %v", err)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan applied: %v", err)
	}
	prev := ""
	for i, e := range got {
		wantH := canonicalHashFor(e, prev)
		if e.Hash != wantH {
			t.Fatalf("applied line %d wrong hash: got %s want %s", i+1, e.Hash, wantH)
		}
		if e.PrevHash != prev {
			t.Fatalf("applied line %d wrong prev_hash: got %q want %q", i+1, e.PrevHash, prev)
		}
		prev = wantH
	}

	// anchor should be updated and previous anchor backed up
	if _, err := os.Stat(anchorPath + ".bak"); err != nil {
		t.Fatalf("expected .hash.bak backup, err=%v", err)
	}
	newAnch := strings.TrimSpace(readFileString(t, anchorPath))
	if newAnch != prev {
		t.Fatalf("anchor mismatch: got %s want %s", newAnch, prev)
	}
}
