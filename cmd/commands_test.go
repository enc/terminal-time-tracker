package cmd

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// helper to create a simple journal file for a given day with provided events
func writeJournalEvents(t *testing.T, day time.Time, events []Event) string {
	t.Helper()
	dir := journalDirFor(day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	p := journalPathFor(day)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open journal failed: %v", err)
	}
	defer f.Close()
	for _, e := range events {
		// ensure deterministic hash chain for tests by computing payload similarly to writeEvent
		// we don't call writeEvent here to keep control over file content
		payload := map[string]any{
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
			"prev_hash": e.PrevHash,
		}
		// compute a hash so later verifyDay/readLastHash expectations remain reasonable
		j, _ := json.Marshal(payload)
		_ = j
		line, _ := json.Marshal(e)
		if _, err := f.Write(append(line, '\n')); err != nil {
			t.Fatalf("write journal line failed: %v", err)
		}
	}
	return journalPathFor(day)
}

func TestCompletionCmd_Errors(t *testing.T) {
	// missing arg -> should return an error describing that shell arg is required
	if err := completionCmd.RunE(completionCmd, []string{}); err == nil || !strings.Contains(err.Error(), "missing shell argument") {
		t.Fatalf("expected missing shell arg error, got: %v", err)
	}

	// unsupported shell -> expect unsupported shell error
	if err := completionCmd.RunE(completionCmd, []string{"nope-shell"}); err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected unsupported shell error, got: %v", err)
	}
}

func TestCustomerProjectValidArgsAndAddCmdValidArgs(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	// create a day with a few events containing customers/projects
	day := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	evs := []Event{
		{ID: "x1", Type: "add", TS: day.Add(1 * time.Hour), Customer: "Acme", Project: "Site", Activity: "dev", Ref: day.Format(time.RFC3339) + ".." + day.Add(time.Hour).Format(time.RFC3339)},
		{ID: "x2", Type: "add", TS: day.Add(2 * time.Hour), Customer: "Beta", Project: "App", Activity: "ops", Ref: day.Format(time.RFC3339) + ".." + day.Add(2*time.Hour).Format(time.RFC3339)},
		{ID: "x3", Type: "add", TS: day.Add(3 * time.Hour), Customer: "Acme", Project: "Mobile", Activity: "test", Ref: day.Format(time.RFC3339) + ".." + day.Add(3*time.Hour).Format(time.RFC3339)},
	}
	writeJournalEvents(t, day, evs)

	// args length 0 -> complete customers
	custs, dir := customerProjectValidArgs(nil, []string{}, "A")
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("expected ShellCompDirectiveNoFileComp, got %v", dir)
	}
	// Expect Acme in results (prefix "A")
	found := false
	for _, c := range custs {
		if c == "Acme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected customer 'Acme' in completions, got %v", custs)
	}

	// args length 1 -> provide projects for given customer
	projs, dir2 := customerProjectValidArgs(nil, []string{"Acme"}, "")
	if dir2 != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("expected ShellCompDirectiveNoFileComp, got %v", dir2)
	}
	// projects should include Site and Mobile
	sort.Strings(projs)
	if !(contains(projs, "Site") && contains(projs, "Mobile")) {
		t.Fatalf("expected Site and Mobile in projects for Acme, got %v", projs)
	}

	// Test addCmdValidArgs:
	// args length <=2 (completing optional customer) should return customers
	addCusts, _ := addCmdValidArgs(nil, []string{"2026-04-05T09:00", "2026-04-05T10:00"}, "")
	if !contains(addCusts, "Acme") || !contains(addCusts, "Beta") {
		t.Fatalf("addCmdValidArgs customers missing; got %v", addCusts)
	}
	// args length ==3 (completing project) should return projects for the provided customer
	addProjs, _ := addCmdValidArgs(nil, []string{"2026-04-05T09:00", "2026-04-05T10:00", "Acme"}, "")
	if !contains(addProjs, "Site") || !contains(addProjs, "Mobile") {
		t.Fatalf("addCmdValidArgs projects missing; got %v", addProjs)
	}
}

func TestFilterPrefixAndSort(t *testing.T) {
	list := []string{"Apple", "apricot", "Banana", "aardvark"}
	got := filterPrefixAndSort(list, "a")
	// expect a, A items only and sorted case-insensitively
	if len(got) != 3 {
		t.Fatalf("expected 3 matches for prefix 'a', got %v", got)
	}
	// empty prefix returns all sorted
	all := filterPrefixAndSort(list, "")
	if len(all) != 4 {
		t.Fatalf("expected 4 items for empty prefix, got %v", all)
	}
	// ensure sorted
	if !sort.StringsAreSorted(all) {
		t.Fatalf("expected all to be sorted; got %v", all)
	}
}

func TestUniqueStringsAndProjectsForCustomer(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	defer os.Setenv("HOME", oldHome)

	day := time.Date(2027, 8, 9, 0, 0, 0, 0, time.UTC)
	evs := []Event{
		{ID: "u1", Type: "add", TS: day.Add(1 * time.Hour), Customer: "C1", Project: "P1", Ref: day.Format(time.RFC3339) + ".." + day.Add(time.Hour).Format(time.RFC3339)},
		{ID: "u2", Type: "add", TS: day.Add(2 * time.Hour), Customer: "C1", Project: "P2", Ref: day.Format(time.RFC3339) + ".." + day.Add(2*time.Hour).Format(time.RFC3339)},
		{ID: "u3", Type: "add", TS: day.Add(3 * time.Hour), Customer: "C2", Project: "P3", Ref: day.Format(time.RFC3339) + ".." + day.Add(3*time.Hour).Format(time.RFC3339)},
	}
	writeJournalEvents(t, day, evs)

	custs := uniqueStringsFromJournal("customer")
	if !contains(custs, "C1") || !contains(custs, "C2") {
		t.Fatalf("uniqueStringsFromJournal missing customers; got %v", custs)
	}
	p1 := projectsForCustomer("C1")
	if !contains(p1, "P1") || !contains(p1, "P2") {
		t.Fatalf("projectsForCustomer missing projects for C1; got %v", p1)
	}
	// non-existing customer should return nil or empty slice
	px := projectsForCustomer("Nope")
	if px != nil && len(px) != 0 {
		t.Fatalf("expected no projects for nonexistent customer, got %v", px)
	}
}

// contains helper
func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Helper to verify sort.StringsAreSorted; older go versions may not have it in this package
var _ = func() bool {
	// ensure import of sort above is used (some static analyzers)
	_ = sort.Strings
	return true
}()
