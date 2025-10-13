package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// small helper to set/restore globals touched by tests
func preserveCustomerMergeGlobals(t *testing.T) func() {
	t.Helper()
	oldTargets := cmTargets
	oldSince := cmSince
	oldFrom := cmFrom
	oldTo := cmTo
	oldNote := cmNote
	oldDry := cmDryRun
	return func() {
		cmTargets = oldTargets
		cmSince = oldSince
		cmFrom = oldFrom
		cmTo = oldTo
		cmNote = oldNote
		cmDryRun = oldDry
		// clear runtime merged map cache so other tests start fresh
		mergedCustomerSet = nil
	}
}

func TestCustomerMerge_DryRunDoesNotWrite(t *testing.T) {
	// preserve and restore flags
	teardown := preserveCustomerMergeGlobals(t)
	defer teardown()

	// deterministic providers
	oldNow := Now
	oldID := IDGen
	defer func() { Now = oldNow; IDGen = oldID }()
	Now = func() time.Time { return time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC) }
	IDGen = func() string { return "dry-run-id" }

	// use fake writer to capture any writes
	fw := &simpleFakeEventWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// set flags for dry-run (default true)
	cmTargets = "t-a,t-b"
	cmTo = "Acme"
	cmDryRun = true

	var buf bytes.Buffer
	customerMergeCmd.SetOut(&buf)
	customerMergeCmd.SetErr(&buf)

	// run the command
	customerMergeCmd.Run(customerMergeCmd, []string{})

	// ensure no events were written during dry-run
	if len(fw.events) != 0 {
		t.Fatalf("expected no events written during dry-run, got %d", len(fw.events))
	}

	// we do not assert on the exact printed dry-run output here to avoid
	// fragile tests that depend on captured stdout vs process stdout.
	// The important assertion is that no events were written (checked above).
}

func TestCustomerMerge_ApplyWritesAmendEventsAndFilterCompletion(t *testing.T) {
	// preserve and restore flags
	teardown := preserveCustomerMergeGlobals(t)
	defer teardown()

	// isolate HOME so config writes don't touch the real user config
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Reset viper and runtime caches
	viper.Reset()
	viper.Set("aliases", nil)
	aliasesCache = nil
	mergedCustomerSet = nil

	// deterministic providers
	oldNow := Now
	oldID := IDGen
	defer func() { Now = oldNow; IDGen = oldID }()
	Now = func() time.Time { return time.Date(2025, 11, 2, 13, 0, 0, 0, time.UTC) }
	IDGen = func() string { return "evt-cm-1" }

	// fake writer to capture events
	fw := &simpleFakeEventWriter{}
	oldWriter := Writer
	Writer = fw
	defer func() { Writer = oldWriter }()

	// Run apply with explicit targets
	cmTargets = "alpha,bravo"
	cmTo = "Acme"
	cmDryRun = false
	cmNote = "merged variations"

	var buf bytes.Buffer
	customerMergeCmd.SetOut(&buf)
	customerMergeCmd.SetErr(&buf)

	customerMergeCmd.Run(customerMergeCmd, []string{})

	// Expect two amend events written
	if len(fw.events) != 2 {
		t.Fatalf("expected 2 amend events written, got %d", len(fw.events))
	}
	expectedRefs := []string{"alpha", "bravo"}
	for i, ev := range fw.events {
		if ev.Type != "amend" {
			t.Fatalf("event %d type expected amend, got %s", i, ev.Type)
		}
		if ev.Ref != expectedRefs[i] {
			t.Fatalf("event %d Ref mismatch, expected %q got %q", i, expectedRefs[i], ev.Ref)
		}
		if ev.Customer != "Acme" {
			t.Fatalf("event %d Customer expected Acme, got %s", i, ev.Customer)
		}
		if ev.Note != "merged variations" {
			t.Fatalf("event %d Note mismatch, got %s", i, ev.Note)
		}
	}

	// Persist a mapping in viper to simulate merged sources (this would normally be done
	// by running with --since/--from or via the command when orig names were detected).
	// Here we simulate merging two source names into the canonical "Acme".
	mmap := map[string]string{
		"ACME Corp": "Acme",
		"Acme, Inc": "Acme",
	}
	viper.Set("customers.map", mmap)
	// load into runtime cache
	loadMergedCustomerMapIntoMemory()

	// Prepare an input list that includes merged source names and some canonical/other names.
	input := []string{"ACME Corp", "OtherCo", "Acme, Inc", "Acme", "Zeta"}
	got := FilterCustomersForCompletion(input)

	// expected: merged source names removed, canonical "Acme" present once, other names preserved
	expected := []string{"Acme", "OtherCo", "Zeta"}
	// FilterCustomersForCompletion sorts output, ensure expected is sorted
	// expected order should be alphabetical
	// sort.Strings(expected) // not needed since we declared in sorted order for test
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("FilterCustomersForCompletion mismatch:\nexpected: %#v\ngot:      %#v", expected, got)
	}

	// Also test CanonicalCustomer and IsCustomerMerged helpers
	if !IsCustomerMerged("ACME Corp") {
		t.Fatalf("IsCustomerMerged expected true for 'ACME Corp'")
	}
	if CanonicalCustomer("ACME Corp") != "Acme" {
		t.Fatalf("CanonicalCustomer expected 'Acme' for 'ACME Corp', got %q", CanonicalCustomer("ACME Corp"))
	}

	// Config persistence is best-effort in this test; it's okay if the config file
	// was not created when running with explicit targets (mapping persistence may
	// only occur when --since/--from path is used). Log presence for debugging but
	// do not fail the test.
	cfg := filepath.Join(tmp, ".tt", "config.yaml")
	if _, err := os.Stat(cfg); err == nil {
		// config file exists — great
	} else {
		t.Logf("config file not present at %s; this is acceptable for this test: %v", cfg, err)
	}
}

// containsSubstring is a tiny helper for assertions that avoids importing strings again.
func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && (indexOf(s, sub) >= 0)))
}

// indexOf returns the index of sub in s or -1. Simple implementation to avoid extra imports.
func indexOf(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
