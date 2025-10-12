package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestAliasFlagCompletion_FilteringAndList(t *testing.T) {
	// Isolate HOME so we don't touch user's real config
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Reset viper state and runtime cache to avoid cross-test pollution
	viper.Reset()
	viper.Set("aliases", nil)
	aliasesCache = nil

	// Create some aliases via the public helper which persists via viper
	b := true
	if err := setAlias("a1", Alias{Customer: "C", Project: "P", Activity: "x", Billable: &b}); err != nil {
		t.Fatalf("setAlias a1 failed: %v", err)
	}
	if err := setAlias("foo", Alias{Activity: "y"}); err != nil {
		t.Fatalf("setAlias foo failed: %v", err)
	}

	// Completion with prefix "f" should return only "foo"
	res, _ := aliasFlagCompletion(nil, nil, "f")
	if len(res) != 1 || res[0] != "foo" {
		t.Fatalf("expected ['foo'] for prefix 'f', got %v", res)
	}

	// Empty prefix should return both names sorted
	res2, _ := aliasFlagCompletion(nil, nil, "")
	expected := []string{"a1", "foo"}
	if !reflect.DeepEqual(res2, expected) {
		t.Fatalf("expected %v for empty prefix, got %v", expected, res2)
	}

	// Ensure config persisted to disk under $HOME/.tt/config.yaml
	cfg := filepath.Join(tmp, ".tt", "config.yaml")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("expected config file at %s, stat error: %v", cfg, err)
	}
}
