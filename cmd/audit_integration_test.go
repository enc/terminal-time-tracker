package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Helpers to craft mixed legacy/canonical journals for integration testing ---

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = orig
	}()
	fn()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	return buf.String()
}

func TestAuditIntegration_VerifyAndRepair(t *testing.T) {
	// Isolate HOME so audit walks only our test journal
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	// Build a single-day journal with mixed canonical (first) and legacy (second) hashes
	day := time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC)
	path := journalPathFor(day) // creates directories under our HOME

	// Two simple events (types don't matter for audit hashing)
	e1 := Event{
		ID:       "e1",
		Type:     "start",
		TS:       day.Add(9 * time.Hour),
		Customer: "acme",
		Project:  "p1",
		Activity: "dev",
		Tags:     []string{"t"},
	}
	e2 := Event{
		ID:       "e2",
		Type:     "note",
		TS:       day.Add(10 * time.Hour),
		Customer: "acme",
		Project:  "p1",
		Activity: "dev",
		Note:     "something",
		Tags:     []string{"t"},
	}

	// First event canonical, second event legacy using prev=first canonical hash
	h1 := canonicalHashFor(e1, "")
	e1.PrevHash = ""
	e1.Hash = h1

	h2legacy := legacyHashFor(e2, h1)
	e2.PrevHash = h1
	e2.Hash = h2legacy

	writeEventsJSONL(t, path, []Event{e1, e2})

	// Anchor equals the chain-end according to verify's advancement (legacy on second)
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	if err := os.WriteFile(anchorPath, []byte(h2legacy+"\n"), 0o644); err != nil {
		t.Fatalf("write anchor: %v", err)
	}

	// 1) Verify should pass and mention OK for our file
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"audit", "verify"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "OK") {
		t.Fatalf("expected verify to report OK; got:\n%s", out)
	}
	if !strings.Contains(out, filepath.Base(path)) {
		t.Fatalf("expected verify output to mention file; got:\n%s", out)
	}

	// 2) Repair (dry-run default) should produce .repair files
	out = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"audit", "repair"})
		_ = rootCmd.Execute()
	})
	repairPath := path + ".repair"
	repairAnchorPath := anchorPath + ".repair"
	if _, err := os.Stat(repairPath); err != nil {
		t.Fatalf("expected .repair file to be written: %v", err)
	}
	if _, err := os.Stat(repairAnchorPath); err != nil {
		t.Fatalf("expected .hash.repair file to be written: %v", err)
	}
	if !strings.Contains(out, "WROTE:") {
		t.Fatalf("expected repair to mention WROTE; got:\n%s", out)
	}

	// Inspect proposed repaired content: it should be canonical chain ending in canonical hash
	repContent := readFileString(t, repairPath)
	var repaired []Event
	sc := bufio.NewScanner(strings.NewReader(repContent))
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal([]byte(sc.Text()), &e); err != nil {
			t.Fatalf("unmarshal repaired event: %v", err)
		}
		repaired = append(repaired, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan repaired: %v", err)
	}
	prev := ""
	for i, e := range repaired {
		want := canonicalHashFor(e, prev)
		if e.Hash != want {
			t.Fatalf("repaired line %d hash mismatch: got %s want %s", i+1, e.Hash, want)
		}
		if e.PrevHash != prev {
			t.Fatalf("repaired line %d prev_hash mismatch: got %q want %q", i+1, e.PrevHash, prev)
		}
		prev = want
	}
	anchProp := strings.TrimSpace(readFileString(t, repairAnchorPath))
	if anchProp != prev {
		t.Fatalf("proposed anchor mismatch: got %s want %s", anchProp, prev)
	}

	// 3) Apply repair and ensure files/anchors are updated and backups created
	out = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"audit", "repair", "--dry-run=false", "--apply=true"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "APPLIED:") {
		t.Fatalf("expected apply to mention APPLIED; got:\n%s", out)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("expected original .jsonl backup to exist: %v", err)
	}
	if _, err := os.Stat(anchorPath + ".bak"); err != nil {
		t.Fatalf("expected original .hash backup to exist: %v", err)
	}
	// Validate the applied file is canonical from scratch and anchor matches end-of-chain
	applied := readFileString(t, path)
	repaired = repaired[:0]
	sc = bufio.NewScanner(strings.NewReader(applied))
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal([]byte(sc.Text()), &e); err != nil {
			t.Fatalf("unmarshal applied event: %v", err)
		}
		repaired = append(repaired, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan applied: %v", err)
	}
	prev = ""
	for i, e := range repaired {
		want := canonicalHashFor(e, prev)
		if e.Hash != want {
			t.Fatalf("applied line %d hash mismatch: got %s want %s", i+1, e.Hash, want)
		}
		if e.PrevHash != prev {
			t.Fatalf("applied line %d prev_hash mismatch: got %q want %q", i+1, e.PrevHash, prev)
		}
		prev = want
	}
	finalAnchor := strings.TrimSpace(readFileString(t, anchorPath))
	if finalAnchor != prev {
		t.Fatalf("anchor after apply mismatch: got %s want %s", finalAnchor, prev)
	}

	// 4) Verify again after apply; should pass
	out = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"audit", "verify"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "OK") {
		t.Fatalf("expected verify to report OK after apply; got:\n%s", out)
	}
}
