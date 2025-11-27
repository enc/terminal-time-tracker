package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var ticketTUICmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI to view and edit ticket numbers",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newTicketTUIModel()
		p := tea.NewProgram(m)
		_, err := p.Run()
		return err
	},
}

func init() {
	ticketCmd.AddCommand(ticketTUICmd)
}

type ticketEntry struct {
	Customer string
	Project  string
	Ticket   string
}

type ticketTUIMode int

const (
	modeList ticketTUIMode = iota
	modeForm
)

type ticketTUIModel struct {
	items    []ticketEntry
	store    ticketStore
	selected int
	mode     ticketTUIMode
	formTicket textinput.Model
	selectionFocus bool // true when selection list (new) is focused
	status   string
	err      string
	observed []observedProject
	missing  bool
	formIsNew bool
	formTarget ticketEntry
	missingList []ticketEntry
	selectionIdx int
}

func newTicketTUIModel() ticketTUIModel {
	store := loadTicketsFn()
	items := ticketStoreToEntries(store)
	ti := textinput.New()
	ti.Placeholder = "Ticket"
	ti.CharLimit = 64
	ti.Blur()
	return ticketTUIModel{
		items:         items,
		store:         store,
		mode:          modeList,
		formTicket:    ti,
		status:        "n: new  e/enter: edit  del: delete  f: filter missing  q: quit  ctrl+s: save (in form)",
		observed:      observedProjectsFn(),
		selectionFocus: true,
	}
}

// ensure we implement tea.Model
var _ tea.Model = ticketTUIModel{}

func (m ticketTUIModel) Init() tea.Cmd { return nil }

func (m ticketTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeForm:
			return m.updateForm(msg)
		}
	}
	return m, nil
}

func (m ticketTUIModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "down", "j":
		if len(m.items) == 0 {
			return m, nil
		}
		if m.selected < len(m.items)-1 {
			m.selected++
		}
	case "up", "k":
		if len(m.items) == 0 {
			return m, nil
		}
		if m.selected > 0 {
			m.selected--
		}
	case "n":
		m.mode = modeForm
		m.formIsNew = true
		m.err = ""
		m.selectionFocus = true
		m.formTicket.SetValue("")
		m.formTicket.Blur()
		m.missingList = missingTicketEntries(m.observed, m.store)
		m.selectionIdx = 0
		if len(m.missingList) == 0 {
			m.err = "no recent projects without tickets"
			m.mode = modeList
			return m, nil
		}
	case "enter", "e":
		if len(m.items) == 0 {
			return m, nil
		}
		m.mode = modeForm
		m.formIsNew = false
		m.err = ""
		cur := m.items[m.selected]
		m.formTarget = cur
		m.formTicket.SetValue(cur.Ticket)
		m.formTicket.Focus()
		m.selectionFocus = false
		m.missingList = nil
	case "delete":
		if len(m.items) == 0 {
			return m, nil
		}
		item := m.items[m.selected]
		if err := deleteTicketFn(item.Customer, item.Project); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.store = loadTicketsFn()
		m.items = ticketStoreToEntries(m.store)
		if m.selected >= len(m.items) && m.selected > 0 {
			m.selected--
		}
		m.status = fmt.Sprintf("Removed %s / %s", item.Customer, item.Project)
	case "f":
		m.missing = !m.missing
		m.items = m.computeListItems()
		if m.selected >= len(m.items) && m.selected > 0 {
			m.selected = len(m.items) - 1
		}
		if m.missing {
			m.status = "Showing only projects without tickets (toggle with f)"
		} else {
			m.status = "Showing all tickets (toggle missing with f)"
		}
	}
	return m, nil
}

func (m ticketTUIModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeList
		m.err = ""
		return m, nil
	case tea.KeyCtrlS:
		tick := strings.TrimSpace(m.formTicket.Value())
		if tick == "" {
			m.err = "ticket is required"
			return m, nil
		}
		target := m.formTarget
		if m.formIsNew {
			if len(m.missingList) == 0 {
				m.err = "no target selected"
				return m, nil
			}
			target = m.missingList[m.selectionIdx]
		}
		if err := setTicketFn(target.Customer, target.Project, tick); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.store = loadTicketsFn()
		m.items = m.computeListItems()
		m.mode = modeList
		m.err = ""
		m.status = fmt.Sprintf("Saved %s / %s [%s]", target.Customer, target.Project, tick)
		// focus selection to saved item
		for i, it := range m.items {
			if strings.EqualFold(it.Customer, target.Customer) && strings.EqualFold(it.Project, target.Project) {
				m.selected = i
				break
			}
		}
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		if m.formIsNew {
			m.selectionFocus = !m.selectionFocus
			if m.selectionFocus {
				m.formTicket.Blur()
			} else {
				m.formTicket.Focus()
			}
		} else {
			m.selectionFocus = false
			m.formTicket.Focus()
		}
		return m, nil
	}

	if m.formIsNew && m.selectionFocus {
		switch msg.String() {
		case "up", "k":
			if m.selectionIdx > 0 {
				m.selectionIdx--
			}
		case "down", "j":
			if m.selectionIdx < len(m.missingList)-1 {
				m.selectionIdx++
			}
		}
		return m, nil
	}

	// Route remaining keys to ticket input only.
	var cmd tea.Cmd
	m.formTicket, cmd = m.formTicket.Update(msg)
	return m, cmd
}

func (m ticketTUIModel) View() string {
	var b strings.Builder
	b.WriteString("Ticket Manager (q to quit)\n")
	b.WriteString(m.status)
	if m.err != "" {
		b.WriteString("\nERR: " + m.err)
	}
	b.WriteString("\n\n")

	switch m.mode {
	case modeList:
		b.WriteString(m.renderList())
	case modeForm:
		b.WriteString(m.renderForm())
	}
	return b.String()
}

func (m ticketTUIModel) renderList() string {
	m.items = m.computeListItems()
	if len(m.items) == 0 {
		if m.missing {
			return "No missing tickets found.\n"
		}
		return "No tickets.\nPress n to create one.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-20s %-12s\n", "Customer", "Project", "Ticket")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 56))
	for i, it := range m.items {
		prefix := " "
		if i == m.selected {
			prefix = ">"
		}
		fmt.Fprintf(&b, "%s %-19s %-20s %-12s\n", prefix, it.Customer, it.Project, it.Ticket)
	}
	b.WriteString("\nKeys: n new | e/enter edit | del delete | f filter missing | q quit\n")
	return b.String()
}

func (m ticketTUIModel) renderForm() string {
	var b strings.Builder
	header := "Edit ticket (ctrl+s save, esc cancel, tab to switch focus)"
	if m.formIsNew {
		header = "New ticket (select project ↑/↓, tab to ticket, ctrl+s save, esc cancel)"
	}
	b.WriteString(header + "\n\n")

	if m.formIsNew {
		b.WriteString("Projects without tickets:\n")
		if len(m.missingList) == 0 {
			b.WriteString("  (none)\n")
		} else {
			for i, op := range m.missingList {
				cursor := " "
				if m.selectionFocus && i == m.selectionIdx {
					cursor = ">"
				}
				fmt.Fprintf(&b, "%s %s / %s\n", cursor, op.Customer, op.Project)
			}
		}
	} else {
		fmt.Fprintf(&b, "Customer / Project: %s / %s\n", m.formTarget.Customer, m.formTarget.Project)
	}

	b.WriteString("\nTicket: ")
	if !m.selectionFocus || !m.formIsNew {
		b.WriteString(m.formTicket.View())
	} else {
		b.WriteString("[tab to edit]")
	}
	b.WriteString("\n")
	return b.String()
}

func ticketStoreToEntries(store ticketStore) []ticketEntry {
	var entries []ticketEntry
	for cust, pm := range store {
		for proj, t := range pm {
			entries = append(entries, ticketEntry{Customer: cust, Project: proj, Ticket: t})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Customer != entries[j].Customer {
			return entries[i].Customer < entries[j].Customer
		}
		return entries[i].Project < entries[j].Project
	})
	return entries
}

func (m ticketTUIModel) computeListItems() []ticketEntry {
	if m.missing {
		return missingTicketEntries(m.observed, m.store)
	}
	return ticketStoreToEntries(m.store)
}
