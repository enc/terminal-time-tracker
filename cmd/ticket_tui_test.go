package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTicketTUI_SaveUpdatesListAndSelection(t *testing.T) {
	origSet := setTicketFn
	origLoad := loadTicketsFn
	origObserved := observedProjectsFn
	defer func() {
		setTicketFn = origSet
		loadTicketsFn = origLoad
		observedProjectsFn = origObserved
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
	observedProjectsFn = func() []observedProject {
		return []observedProject{
			{Customer: "Acme", Project: "API"},
		}
	}

	m := newTicketTUIModel()
	// trigger new flow
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model := m2.(ticketTUIModel)
	if model.mode != modeForm || !model.formIsNew {
		t.Fatalf("expected form mode for new ticket")
	}
	if len(model.missingList) == 0 {
		t.Fatalf("expected missing list to be populated")
	}
	model.formTicket.SetValue("B-2")
	// move focus to ticket input if needed
	if model.selectionFocus {
		mnext, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model = mnext.(ticketTUIModel)
	}

	m3, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = m3.(ticketTUIModel)
	if len(saved) != 1 || saved[0].Customer != "Acme" || saved[0].Project != "API" || saved[0].Ticket != "B-2" {
		t.Fatalf("unexpected saved: %+v", saved)
	}
	if model.mode != modeList {
		t.Fatalf("expected modeList after save")
	}
}

func TestTicketTUI_ListNavigationUnaffectedInForm(t *testing.T) {
	m := newTicketTUIModel()
	m.store = ticketStore{}
	m.items = []ticketEntry{
		{Customer: "A", Project: "One", Ticket: "1"},
		{Customer: "B", Project: "Two", Ticket: "2"},
	}
	m.observed = []observedProject{
		{Customer: "A", Project: "One"},
		{Customer: "B", Project: "Two"},
	}
	m.selected = 1
	m.mode = modeForm
	m.formIsNew = false
	m.formTarget = m.items[m.selected]
	m.formTicket.SetValue("Editing")
	m.selectionFocus = false

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := m2.(ticketTUIModel)
	if model.selected != 1 {
		t.Fatalf("list selection should not change in form mode; got %d", model.selected)
	}
}
