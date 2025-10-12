package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

// helper to create *bool
func boolptr(b bool) *bool { return &b }

// ensure an isolated viper state for tests that manipulate HOME
func setupTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	old := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", old)
		// best-effort clear of viper keys we touched
		viper.Set("aliases", nil)
	})
	// also clear any in-memory aliases to avoid cross-test pollution
	viper.Set("aliases", nil)
	return tmp
}

func TestSetGetDeleteAlias_Single(t *testing.T) {
	home := setupTempHome(t)

	// ensure no preexisting config
	cfgPath := filepath.Join(home, ".tt", "config.yaml")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("expected no config at %s before test", cfgPath)
	}

	name := "quick"
	a := Alias{
		Customer: "Acme",
		Project:  "Website",
		Activity: "dev",
		Billable: boolptr(true),
		Tags:     []string{"frontend", "urgent"},
		Note:     "use for quick tasks",
	}

	// set alias
	if err := setAlias(name, a); err != nil {
		t.Fatalf("setAlias failed: %v", err)
	}

	// config file should have been created under $HOME/.tt/config.yaml
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config file at %s after setAlias, stat error: %v", cfgPath, err)
	}

	// getAlias should retrieve the same values
	got, ok := getAlias(name)
	if !ok {
		t.Fatalf("getAlias did not find alias %q", name)
	}
	if !reflect.DeepEqual(got.Customer, a.Customer) ||
		!reflect.DeepEqual(got.Project, a.Project) ||
		!reflect.DeepEqual(got.Activity, a.Activity) ||
		!reflect.DeepEqual(got.Note, a.Note) ||
		!reflect.DeepEqual(got.Tags, a.Tags) {
		t.Fatalf("retrieved alias mismatch:\nexpected: %+v\nactual:   %+v", a, got)
	}
	// billable pointer equality check (value)
	if got.Billable == nil || a.Billable == nil || *got.Billable != *a.Billable {
		t.Fatalf("billable mismatch: expected %v got %v", a.Billable, got.Billable)
	}

	// loadAliases map should include our alias
	all := loadAliases()
	if _, ok := all[name]; !ok {
		t.Fatalf("loadAliases missing alias %q", name)
	}

	// delete it
	if err := deleteAlias(name); err != nil {
		t.Fatalf("deleteAlias failed: %v", err)
	}

	// after deletion getAlias should not find it
	if _, ok := getAlias(name); ok {
		t.Fatalf("expected alias %q to be deleted", name)
	}
	// ensure config file still present but without alias (we don't assert exact file content)
}

func TestAliasTableDriven_Multiple(t *testing.T) {
	setupTempHome(t)

	cases := []struct {
		name string
		a    Alias
	}{
		{
			name: "default-bill",
			a: Alias{
				Customer: "C1",
				Project:  "P1",
				Activity: "analysis",
				Billable: nil, // unspecified
				Tags:     []string{"r1"},
			},
		},
		{
			name: "explicit-nonbill",
			a: Alias{
				Customer: "C2",
				Project:  "P2",
				Activity: "meeting",
				Billable: boolptr(false),
				Tags:     []string{},
				Note:     "client sync",
			},
		},
		{
			name: "minimal",
			a: Alias{
				Activity: "break",
			},
		},
	}

	// set all aliases
	for _, c := range cases {
		if err := setAlias(c.name, c.a); err != nil {
			t.Fatalf("setAlias %q failed: %v", c.name, err)
		}
	}

	// verify via loadAliases (map-based)
	loaded := loadAliases()
	for _, c := range cases {
		got, ok := loaded[c.name]
		if !ok {
			t.Fatalf("expected alias %q in loaded map", c.name)
		}
		// Compare fields individually to give clearer failure messages
		if got.Customer != c.a.Customer {
			t.Fatalf("alias %q customer mismatch: expected %q got %q", c.name, c.a.Customer, got.Customer)
		}
		if got.Project != c.a.Project {
			t.Fatalf("alias %q project mismatch: expected %q got %q", c.name, c.a.Project, got.Project)
		}
		if got.Activity != c.a.Activity {
			t.Fatalf("alias %q activity mismatch: expected %q got %q", c.name, c.a.Activity, got.Activity)
		}
		if !reflect.DeepEqual(got.Tags, c.a.Tags) {
			t.Fatalf("alias %q tags mismatch: expected %v got %v", c.name, c.a.Tags, got.Tags)
		}
		// billable: compare nil vs nil or value equality
		if (got.Billable == nil) != (c.a.Billable == nil) {
			t.Fatalf("alias %q billable nil-ness mismatch: expected %v got %v", c.name, c.a.Billable, got.Billable)
		}
		if got.Billable != nil && c.a.Billable != nil && *got.Billable != *c.a.Billable {
			t.Fatalf("alias %q billable value mismatch: expected %v got %v", c.name, *c.a.Billable, *got.Billable)
		}
	}

	// delete one and ensure it is removed while others remain
	if err := deleteAlias(cases[1].name); err != nil {
		t.Fatalf("deleteAlias failed: %v", err)
	}
	after := loadAliases()
	if _, ok := after[cases[1].name]; ok {
		t.Fatalf("alias %q should have been deleted", cases[1].name)
	}
	// ensure others still present
	if _, ok := after[cases[0].name]; !ok {
		t.Fatalf("alias %q unexpectedly missing after deletion of another", cases[0].name)
	}
	if _, ok := after[cases[2].name]; !ok {
		t.Fatalf("alias %q unexpectedly missing after deletion of another", cases[2].name)
	}
}
