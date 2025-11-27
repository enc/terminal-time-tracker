package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTicketTUI_SaveUpdatesListAndSelection(t *testing.T) {
	origSet := setTicketFn
	origLoad := loadTicketsFn
	defer func() {
		setTicketFn = origSet
		loadTicketsFn = origLoad
	}()

	var saved []ticketEntry
	setTicketFn = func(cust, proj, ticket string) error {
		saved = append(saved, ticketEntry{Customer: cust, Project: proj, Ticket: ticket})
		return nil
	}
	loadTicketsFn = func() ticketStore {
		return ticketStore{
			"Acme": {"Web": "A-1"},
		}
	}

	m := newTicketTUIModel()
	if m.selected != 0 {
		t.Fatalf("expected selected=0, got %d", m.selected)
	}

	// enter form with new values
	m.mode = modeForm
	m.inputs[0].SetValue("Beta")
	m.inputs[1].SetValue("App")
	m.inputs[2].SetValue("B-2")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model := m2.(ticketTUIModel)
	if len(saved) != 1 || saved[0].Customer != "Beta" || saved[0].Project != "App" || saved[0].Ticket != "B-2" {
		t.Fatalf("unexpected saved: %+v", saved)
	}
	if model.mode != modeList {
		t.Fatalf("expected modeList after save")
	}
}

func TestTicketTUI_ListNavigationUnaffectedInForm(t *testing.T) {
	m := newTicketTUIModel()
	m.items = []ticketEntry{
		{Customer: "A", Project: "One", Ticket: "1"},
		{Customer: "B", Project: "Two", Ticket: "2"},
	}
	m.selected = 1
	m.mode = modeForm
	m.focus = 0
	m.inputs[0].SetValue("Editing")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := m2.(ticketTUIModel)
	if model.selected != 1 {
		t.Fatalf("list selection should not change in form mode; got %d", model.selected)
	}
}
