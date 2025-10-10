package tui

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeWriter captures Start/Switch calls for assertions.
type fakeWriter struct {
	startParams  *StartParams
	switchParams *SwitchParams
	startCalled  bool
	switchCalled bool
	startErr     error
	switchErr    error
}

func (f *fakeWriter) Start(ctx context.Context, p StartParams) error {
	f.startCalled = true
	cp := p
	f.startParams = &cp
	return f.startErr
}

func (f *fakeWriter) Stop(ctx context.Context) error {
	return nil
}

func (f *fakeWriter) Note(ctx context.Context, text string) error {
	return nil
}

func (f *fakeWriter) Add(ctx context.Context, p AddParams) error {
	return nil
}

func (f *fakeWriter) Switch(ctx context.Context, p SwitchParams) error {
	f.switchCalled = true
	cp := p
	f.switchParams = &cp
	return f.switchErr
}

func newTestForm() *formModel {
	// Use the public constructor for convenience, then override candidates to avoid
	// brittle filesystem dependencies in tests.
	f := NewStartSwitchForm(nil, nil, false, nil)
	// Provide deterministic candidate lists for tests.
	f.custCandidates = []string{"acme", "acmecorp", "alphabet", "beta", "gamma"}
	f.projCandidates = []string{"mobilize:foundation", "mobilize:web", "infra", "docs"}
	// Ensure a sensible width for matchList sizing logic.
	f.width = 80
	return f
}

func TestFilterPrefix(t *testing.T) {
	list := []string{"Alpha", "beta", "alphabet", "gamma", "Alpine"}
	got := filterPrefix(list, "al")
	want := []string{"Alpha", "alphabet", "Alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterPrefix(case-insensitive) = %v; want %v", got, want)
	}

	// empty prefix returns top candidates (limit 20)
	longList := make([]string, 25)
	for i := range longList {
		longList[i] = "item" + strconv.Itoa(i)
	}
	gotEmpty := filterPrefix(longList, "")
	if len(gotEmpty) != 20 {
		t.Fatalf("filterPrefix empty prefix returned %d items; want 20", len(gotEmpty))
	}
}

func TestFilterContains(t *testing.T) {
	list := []string{"Alpha", "beta", "alphabet", "gamma", "Alpine"}
	got := filterContains(list, "ph")
	want := []string{"Alpha", "alphabet"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterContains = %v; want %v", got, want)
	}

	// whitespace/empty returns top candidates (limit 20)
	longList := make([]string, 30)
	for i := range longList {
		longList[i] = "x" + strconv.Itoa(i)
	}
	gotEmpty := filterContains(longList, " ")
	if len(gotEmpty) != 20 {
		t.Fatalf("filterContains empty term returned %d items; want 20", len(gotEmpty))
	}
}

func TestFuzzyMatches(t *testing.T) {
	list := []string{"design", "development", "deploy", "doc", "debug", "demo"}
	got := fuzzyMatches(list, "dev")
	if len(got) == 0 {
		t.Fatalf("fuzzyMatches returned no results for 'dev'; want at least 1")
	}
	// Expect "development" to be the top match for "dev"
	if got[0] != "development" {
		t.Fatalf("fuzzyMatches top result = %q; want %q", got[0], "development")
	}

	// empty term or empty list yields nil
	if res := fuzzyMatches(nil, "x"); res != nil {
		t.Fatalf("fuzzyMatches on nil list = %v; want nil", res)
	}
	if res := fuzzyMatches(list, ""); res != nil {
		t.Fatalf("fuzzyMatches on empty term = %v; want nil", res)
	}
}

func TestFocusNextPrev(t *testing.T) {
	f := newTestForm()
	orig := f.focused
	// focusNext should advance by 1
	f.focusNext()
	if f.focused != (orig+1)%len(f.inputs) {
		t.Fatalf("focusNext: focused = %d; want %d", f.focused, (orig+1)%len(f.inputs))
	}
	// focusPrev should go back
	f.focusPrev()
	if f.focused != orig {
		t.Fatalf("focusPrev: focused = %d; want %d", f.focused, orig)
	}
	// wrap-around behavior: set focused to last and next -> 0
	f.focused = len(f.inputs) - 1
	f.focusNext()
	if f.focused != 0 {
		t.Fatalf("focusNext wrap: focused = %d; want 0", f.focused)
	}
	// prev from 0 should wrap to last
	f.focusPrev()
	if f.focused != len(f.inputs)-1 {
		t.Fatalf("focusPrev wrap: focused = %d; want %d", f.focused, len(f.inputs)-1)
	}
}

func TestSubmitCmdStartAndSwitch(t *testing.T) {
	f := newTestForm()
	f.SetDefaultBillable(true)
	fmWriter := &fakeWriter{}
	f.SetWriter(fmWriter)

	// populate inputs
	f.customerInput.SetValue("  Acme  ")
	f.projectInput.SetValue(" mobilize:foundation ")
	f.activityInput.SetValue("design")
	f.tagsInput.SetValue("tag1, tag2")
	f.noteInput.SetValue("note here")

	// Start mode (default)
	cmd := f.submitCmd()
	msg := cmd()
	if _, ok := msg.(startDoneMsg); !ok {
		t.Fatalf("submitCmd start returned %T; want startDoneMsg", msg)
	}
	if !fmWriter.startCalled {
		t.Fatalf("writer.Start was not called")
	}
	if fmWriter.startParams == nil {
		t.Fatalf("writer.Start params nil")
	}
	if fmWriter.startParams.Customer != "Acme" {
		t.Fatalf("Start Customer = %q; want %q", fmWriter.startParams.Customer, "Acme")
	}
	if !fmWriter.startParams.Billable {
		t.Fatalf("Start Billable = false; want true")
	}
	if len(fmWriter.startParams.Tags) != 2 || fmWriter.startParams.Tags[0] != "tag1" {
		t.Fatalf("Start Tags = %v; want [tag1 tag2]", fmWriter.startParams.Tags)
	}
	if fmWriter.startParams.Note != "note here" {
		t.Fatalf("Start Note = %q; want %q", fmWriter.startParams.Note, "note here")
	}

	// Switch mode should call Switch on writer
	fmWriter.startCalled = false
	fmWriter.switchCalled = false
	f.SetMode("switch")
	cmd2 := f.submitCmd()
	msg2 := cmd2()
	if _, ok := msg2.(startDoneMsg); !ok {
		t.Fatalf("submitCmd switch returned %T; want startDoneMsg", msg2)
	}
	if !fmWriter.switchCalled {
		t.Fatalf("writer.Switch was not called")
	}
	if fmWriter.switchParams == nil {
		t.Fatalf("writer.Switch params nil")
	}
	if fmWriter.switchParams.Project != "mobilize:foundation" {
		t.Fatalf("Switch Project = %q; want %q", fmWriter.switchParams.Project, "mobilize:foundation")
	}
}

func TestSuggestMsgHandlingAndListSelection(t *testing.T) {
	f := newTestForm()
	// Ensure deterministic debounceID; set to 1 and use a matching suggestMsg id.
	f.debounceID = 1

	// Focus customer (0) and send suggestMsg for term "ac"
	f.focused = 0
	_, _ = f.Update(suggestMsg{id: 1, focused: 0, term: "ac"})
	// cmd may be nil; but Update should have populated matchList and opened it.
	if !f.listOpen {
		t.Fatalf("suggestMsg handling did not open list for 'ac'")
	}
	if len(f.matchList.Items()) == 0 {
		t.Fatalf("suggestMsg produced zero matches; want >0")
	}

	// Now simulate selecting the first item via Enter key while list is open.
	// Use a tea.KeyMsg value representing Enter.
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	// Set focused to customer to ensure acceptance into customer input.
	f.focused = 0
	// Ensure the first item is selected (list defaults to index 0)
	_, _ = f.Update(enterKey)
	// After accepting selection, list should be closed and customer input set.
	if f.listOpen {
		t.Fatalf("after Enter, listOpen = true; want false")
	}
	if f.customerInput.Value() == "" {
		t.Fatalf("after selecting from list, customer input empty; want selected value")
	}

	// Test suggestMsg ignored when id doesn't match debounceID
	// set debounceID to 99 and send msg with id 1 -> should be ignored
	f.listOpen = false
	f.debounceID = 99
	_, _ = f.Update(suggestMsg{id: 1, focused: 0, term: "ac"})
	if f.listOpen {
		t.Fatalf("suggestMsg with stale id should be ignored but opened list")
	}
}
