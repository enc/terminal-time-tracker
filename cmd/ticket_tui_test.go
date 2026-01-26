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
	if len(m.customers) != 1 {
		t.Fatalf("expected 1 customer, got %d", len(m.customers))
	}
	if len(m.currentProjects()) != 1 {
		t.Fatalf("expected 1 project, got %d", len(m.currentProjects()))
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
	for _, p := range model.currentProjects() {
		if p.Name == "API" && p.Ticket == "NEW-2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ticket not reflected in list")
	}
}

func TestTicketTUI_ToggleViewAll(t *testing.T) {
	origLoad := loadTicketsFn
	origObs := observedProjectsFn
	defer func() {
		loadTicketsFn = origLoad
		observedProjectsFn = origObs
	}()

	loadTicketsFn = func() ticketStore {
		return ticketStore{
			"Acme": {"API": "ABC-1"},
			"Gamma": {
				"Legacy": "LEG-9",
			},
		}
	}
	observedProjectsFn = func() []observedProject {
		return []observedProject{
			{Customer: "Acme", Project: "API", LastSeen: time.Now()},
			{Customer: "Beta", Project: "App", LastSeen: time.Now()},
		}
	}

	m := newTicketUI()
	if len(m.customers) != 2 {
		t.Fatalf("expected 2 active customers, got %d", len(m.customers))
	}
	// toggle view to include inactive
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model := m2.(ticketUIModel)
	if len(model.customers) != 3 {
		t.Fatalf("expected 3 customers after toggle, got %d", len(model.customers))
	}
}
