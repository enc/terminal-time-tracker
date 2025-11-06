package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestBuildCompletionIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mergedCustomerSet = nil

	now := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	later := now.Add(2 * time.Hour)

	journalDir := filepath.Join(os.Getenv("HOME"), ".tt", "journal", "2024", "05")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}

	events := []Event{
		{ID: "a", Type: "start", TS: now, Customer: "Internal", Project: "Payroll"},
		{ID: "b", Type: "note", TS: now.Add(time.Minute), Customer: "Internal"},
		{ID: "c", Type: "start", TS: later, Customer: "Interal", Project: "Payroll"},
		{ID: "d", Type: "start", TS: now, Project: "General"},
	}

	mergedCustomerSet = map[string]string{"Interal": "Internal"}

	path := filepath.Join(journalDir, "2024-05-01.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create journal: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	f.Close()

	idx, err := BuildCompletionIndex("")
	if err != nil {
		t.Fatalf("BuildCompletionIndex: %v", err)
	}

	if len(idx.Customers) != 1 {
		t.Fatalf("expected 1 customer group, got %d", len(idx.Customers))
	}

	internal := idx.Customers["Internal"]
	if internal == nil {
		t.Fatalf("missing Internal customer group")
	}
	if internal.Total != 3 {
		t.Fatalf("expected total 3, got %d", internal.Total)
	}
	if _, ok := internal.Names["Internal"]; !ok {
		t.Fatalf("expected raw Internal in group")
	}
	if _, ok := internal.Names["Interal"]; !ok {
		t.Fatalf("expected raw Interal in group after canonicalization")
	}

	if got := idx.SortedProjects("Internal"); len(got) != 1 || got[0] != "Payroll" {
		t.Fatalf("unexpected projects for Internal: %#v", got)
	}

	uncategorized := idx.SortedProjects("")
	if len(uncategorized) != 1 || uncategorized[0] != "General" {
		t.Fatalf("unexpected uncategorized projects: %#v", uncategorized)
	}
}

func TestCompletionDecisionsLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	viper.Reset()

	viper.Set("completion.allow.customers", []string{"Internal"})
	viper.Set("completion.ignore.customers", []string{"Typo"})
	viper.Set("completion.allow.projects", map[string]any{
		"Internal": []any{"Payroll", "HR"},
		"":         []any{"General"},
	})
	viper.Set("completion.ignore.projects", map[string]any{
		"Internal": []any{"Legacy"},
	})

	raw := viper.Get("completion.allow.projects")
	if testing.Verbose() {
		t.Logf("raw allow projects: %T %#v", raw, raw)
	}

	dec := loadCompletionDecisions()

	if !dec.isCustomerAllowed("Internal") {
		t.Fatalf("expected Internal allowed")
	}
	if !dec.isCustomerIgnored("Typo") {
		t.Fatalf("expected Typo ignored")
	}
	if got := dec.allowedProjects("Internal"); len(got) != 2 {
		t.Fatalf("expected Internal allowed projects, got %#v (raw=%#v)", got, raw)
	}
	if got := dec.allowedProjects(""); len(got) != 1 || got[0] != "General" {
		t.Fatalf("unexpected uncategorized projects: %#v", got)
	}

	dec.allowCustomer("Acme")
	dec.ignoreProject("Internal", "Legacy")
	dec.allowProject("Internal", "Platform")

	if err := dec.save(); err != nil {
		t.Fatalf("save decisions: %v", err)
	}

	// Re-load to ensure persistence formatting works.
	viper.Reset()
	cfgPath := configFilePath()
	viper.SetConfigFile(cfgPath)
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}

	dec2 := loadCompletionDecisions()
	if !dec2.isCustomerAllowed("Acme") {
		t.Fatalf("expected Acme allowed after reload")
	}
	if projects := dec2.allowedProjects("Internal"); len(projects) != 3 {
		t.Fatalf("expected 3 Internal projects, got %#v", projects)
	}
	if dec2.isCustomerIgnored("Typo") == false {
		t.Fatalf("expected Typo still ignored")
	}
}
