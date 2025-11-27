package cmd

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTicketTUI_EditSavesAndRefreshes(t *testing.T) {
	origSet := setTicketFn
	origLoad := loadTicketsFn
	origObs := observedProjectsFn
	defer func() {
		setTicketFn = origSet
		loadTicketsFn = origLoad
		observedProjectsFn = origObs
	}()

	saved := []ticketEntry{}
	setTicketFn = func(c, p, ticket string) error {
		saved = append(saved, ticketEntry{Customer: c, Project: p, Ticket: ticket})
		return nil
	}
	loadTicketsFn = func() ticketStore {
		ticket := "OLD-1"
		if len(saved) > 0 {
			ticket = saved[len(saved)-1].Ticket
		}
		return ticketStore{
			"Acme": {"API": ticket},
		}
	}
	observedProjectsFn = func() []observedProject {
		return []observedProject{
			{Customer: "Acme", Project: "API", LastSeen: time.Now()},
		}
	}

	m := newTicketUI()
	if len(m.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.entries))
	}
	// enter edit
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := m2.(ticketUIModel)
	if !model.editing {
		t.Fatalf("expected editing mode")
	}
	model.editInput.SetValue("NEW-2")
	m3, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = m3.(ticketUIModel)
	if model.editing {
		t.Fatalf("expected exit edit after save")
	}
	if len(saved) != 1 || saved[0].Ticket != "NEW-2" {
		t.Fatalf("unexpected saved: %+v", saved)
	}
	found := false
	for _, e := range model.entries {
		if e.Customer == "Acme" && e.Project == "API" && e.Ticket == "NEW-2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ticket not reflected in list")
	}
}

func TestTicketTUI_FilterMissing(t *testing.T) {
	origLoad := loadTicketsFn
	origObs := observedProjectsFn
	defer func() {
		loadTicketsFn = origLoad
		observedProjectsFn = origObs
	}()

	loadTicketsFn = func() ticketStore {
		return ticketStore{
			"Acme": {"API": "ABC-1"},
		}
	}
	observedProjectsFn = func() []observedProject {
		return []observedProject{
			{Customer: "Acme", Project: "API", LastSeen: time.Now()},
			{Customer: "Beta", Project: "App", LastSeen: time.Now()},
		}
	}

	m := newTicketUI()
	if len(m.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.entries))
	}
	// toggle filter to missing
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // all -> missing
	model := m2.(ticketUIModel)
	if len(model.entries) != 1 || model.entries[0].Customer != "Beta" {
		t.Fatalf("expected only missing entry, got %+v", model.entries)
	}
}
