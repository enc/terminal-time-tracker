package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Reuse a minimal fake writer for submit-related tests.
type simpleFakeWriter struct {
	startCalled  bool
	switchCalled bool
	startParams  StartParams
	switchParams SwitchParams
}

func (f *simpleFakeWriter) Start(_ context.Context, p StartParams) error {
	f.startCalled = true
	f.startParams = p
	return nil
}
func (f *simpleFakeWriter) Stop(_ context.Context) error             { return nil }
func (f *simpleFakeWriter) Note(_ context.Context, _ string) error   { return nil }
func (f *simpleFakeWriter) Add(_ context.Context, _ AddParams) error { return nil }
func (f *simpleFakeWriter) Switch(_ context.Context, p SwitchParams) error {
	f.switchCalled = true
	f.switchParams = p
	return nil
}

func TestInitCmdNotNil(t *testing.T) {
	f := NewStartSwitchForm(nil, nil, false, nil)
	// Init should return a non-nil command (textinput.Blink)
	cmd := f.Init()
	if cmd == nil {
		t.Fatalf("Init returned nil cmd; want non-nil (textinput.Blink)")
	}
}

func TestViewContainsExpectedSections(t *testing.T) {
	f := NewStartSwitchForm(nil, nil, false, nil)
	// ensure width non-zero for styling
	f.width = 80
	out := f.View()
	if out == "" {
		t.Fatalf("View returned empty string")
	}
	// Should include section title and labels
	want := []string{"Start / Switch", "Customer", "Project", "Activity", "Tags", "Note", "Billable"}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Fatalf("View output missing %q; got:\n%s", w, out)
		}
	}
	// The muted hint with keys should be present
	if !strings.Contains(out, "Tab: next field") {
		t.Fatalf("View output missing navigation hints")
	}
}

func TestEnterOnLastTriggersSubmit(t *testing.T) {
	fw := &simpleFakeWriter{}
	f := NewStartSwitchForm(fw, nil, false, nil)
	f.SetWriter(fw)

	// populate inputs so submit builds non-empty params
	f.customerInput.SetValue("Cust")
	f.projectInput.SetValue("Proj")
	f.activityInput.SetValue("Act")
	f.tagsInput.SetValue("t1,t2")
	f.noteInput.SetValue("note")

	// focus last field (note) and send Enter key
	f.focused = len(f.inputs) - 1
	// Use KeyMsg with Enter
	key := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := f.Update(key)
	if cmd == nil {
		// In Update the Enter at last should return a submit command; ensure we got one.
		t.Fatalf("expected a submit command when pressing Enter on last input; got nil")
	}
	// Execute the returned command to perform Start on writer
	msg := cmd()
	// Ensure it returns a startDoneMsg
	if _, ok := msg.(startDoneMsg); !ok {
		t.Fatalf("submit command did not return startDoneMsg; got %T", msg)
	}
	// Writer.Start should have been called
	if !fw.startCalled {
		t.Fatalf("expected writer.Start to be called by submitCmd")
	}
	// Validate some params
	if fw.startParams.Customer != "Cust" || fw.startParams.Project != "Proj" {
		t.Fatalf("unexpected start params: %+v", fw.startParams)
	}
}

func TestLoadCandidatesFromJournalReadsJsonl(t *testing.T) {
	// Create a temporary HOME so DefaultJournalRoot points to our isolated location.
	origHome := os.Getenv("HOME")
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	// Create journal root and a sample jsonl file with two entries.
	root := filepath.Join(tmp, ".tt", "journal", "2025", "01")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	fpath := filepath.Join(root, "test.jsonl")
	content := `{"customer":"Acme","project":"proj:alpha"}
{"customer":"BetaCo","project":"proj:beta"}` + "\n"
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	// Construct form which will call loadCandidatesFromJournal() in constructor.
	f := NewStartSwitchForm(nil, nil, false, nil)
	// Candidates should include the two customers and projects we wrote.
	foundA := false
	foundB := false
	for _, c := range f.custCandidates {
		if c == "Acme" {
			foundA = true
		}
		if c == "BetaCo" {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("custCandidates missing entries; got: %v", f.custCandidates)
	}
	// projects
	foundP1 := false
	foundP2 := false
	for _, p := range f.projCandidates {
		if p == "proj:alpha" {
			foundP1 = true
		}
		if p == "proj:beta" {
			foundP2 = true
		}
	}
	if !foundP1 || !foundP2 {
		t.Fatalf("projCandidates missing entries; got: %v", f.projCandidates)
	}
}

func TestListItemAndItemsFromStrings(t *testing.T) {
	strs := []string{"one", "two", "three"}
	items := itemsFromStrings(strs)
	if len(items) != len(strs) {
		t.Fatalf("itemsFromStrings returned %d items; want %d", len(items), len(strs))
	}
	// type assert to listItem and verify methods
	for i, it := range items {
		li, ok := it.(listItem)
		if !ok {
			t.Fatalf("item %d is not listItem (type %T)", i, it)
		}
		if li.Title() != strs[i] {
			t.Fatalf("listItem.Title = %q; want %q", li.Title(), strs[i])
		}
		if li.Description() != "" {
			t.Fatalf("listItem.Description expected empty string; got %q", li.Description())
		}
		if li.FilterValue() != strs[i] {
			t.Fatalf("listItem.FilterValue = %q; want %q", li.FilterValue(), strs[i])
		}
	}
}

func TestSettersUpdateState(t *testing.T) {
	f := NewStartSwitchForm(nil, nil, false, nil)
	// mode
	f.SetMode("switch")
	if f.mode != "switch" {
		t.Fatalf("SetMode did not set mode; got %q", f.mode)
	}
	// billable
	f.SetDefaultBillable(true)
	if !f.defaultBill {
		t.Fatalf("SetDefaultBillable did not set defaultBill to true")
	}
	// writer
	fw := &simpleFakeWriter{}
	f.SetWriter(fw)
	if !reflect.DeepEqual(f.writer, fw) {
		t.Fatalf("SetWriter did not set writer properly")
	}
}

func TestFilterHelpersPositive(t *testing.T) {
	list := []string{"Alpha", "alphabet", "Beta", "gamma", "alpine", "delta"}
	// prefix "al" should match alphabetic items starting with 'al' case-insensitive
	p := filterPrefix(list, "al")
	if len(p) == 0 {
		t.Fatalf("filterPrefix returned empty result for prefix 'al'")
	}
	// contains "ph" should match entries containing 'ph'
	c := filterContains(list, "ph")
	if len(c) == 0 || !strings.Contains(strings.ToLower(c[0]), "ph") {
		t.Fatalf("filterContains did not return expected contains matches; got %v", c)
	}
	// fuzzy should find 'alphabet' for term 'alph'
	fm := fuzzyMatches(list, "alph")
	if len(fm) == 0 {
		t.Fatalf("fuzzyMatches returned no results for term 'alph'")
	}
	found := false
	for _, s := range fm {
		if s == "alphabet" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fuzzyMatches results did not include expected 'alphabet'; got %v", fm)
	}
}

// TestCtrlSpaceCompletion verifies that pressing Ctrl+Space while focused on
// the customer or project input opens the completion dropdown and populates
// the match list with ranked candidates.
func TestCtrlSpaceCompletion(t *testing.T) {
	// Customer completion
	f := newTestForm()
	// provide predictable candidates
	f.custCandidates = []string{"acme", "acmecorp", "alpha", "beta"}
	f.projCandidates = []string{"mobilize:foundation", "mobilize:web", "infra", "docs"}
	f.width = 80

	// Focus customer and set a short prefix that will match via prefix/contains
	f.focused = 0
	f.customerInput.SetValue("ac")
	// simulate Ctrl+Space keypress
	_, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+space")})
	if !f.listOpen {
		t.Fatalf("ctrl+space did not open customer completion list")
	}
	if len(f.matchList.Items()) == 0 {
		t.Fatalf("ctrl+space opened list but matchList is empty for customer")
	}
	// Ensure the first match starts with the prefix (case-insensitive)
	first := f.matchList.Items()[0].(listItem).s
	if !strings.HasPrefix(strings.ToLower(first), "ac") {
		t.Fatalf("expected first customer match to start with 'ac', got %q", first)
	}

	// Close list and test project completion
	f.listOpen = false
	// Focus project and set a prefix
	f.focused = 1
	f.projectInput.SetValue("mobil")
	_, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+space")})
	if !f.listOpen {
		t.Fatalf("ctrl+space did not open project completion list")
	}
	if len(f.matchList.Items()) == 0 {
		t.Fatalf("ctrl+space opened list but matchList is empty for project")
	}
	firstP := f.matchList.Items()[0].(listItem).s
	if !strings.HasPrefix(strings.ToLower(firstP), "mobil") {
		t.Fatalf("expected first project match to start with 'mobil', got %q", firstP)
	}
}
