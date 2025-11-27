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
	selected int
	mode     ticketTUIMode
	inputs   []textinput.Model
	focus    int
	status   string
	err      string
}

func newTicketTUIModel() ticketTUIModel {
	items := ticketStoreToEntries(loadTicketsFn())
	inputs := make([]textinput.Model, 3)
	labels := []string{"Customer", "Project", "Ticket"}
	for i := range inputs {
		ti := textinput.New()
		ti.Placeholder = labels[i]
		ti.CharLimit = 64
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	return ticketTUIModel{
		items:  items,
		mode:   modeList,
		inputs: inputs,
		status: "n: new  e/enter: edit  del: delete  q: quit  ctrl+s: save (in form)",
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
		m.focus = 0
		m.err = ""
		for i := range m.inputs {
			m.inputs[i].SetValue("")
			if i == 0 {
				m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
		}
	case "enter", "e":
		if len(m.items) == 0 {
			return m, nil
		}
		m.mode = modeForm
		m.focus = 0
		m.err = ""
		// populate inputs
		cur := m.items[m.selected]
		values := []string{cur.Customer, cur.Project, cur.Ticket}
		for i := range m.inputs {
			m.inputs[i].SetValue(values[i])
			if i == 0 {
				m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
		}
	case "delete":
		if len(m.items) == 0 {
			return m, nil
		}
		item := m.items[m.selected]
		if err := deleteTicketFn(item.Customer, item.Project); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.items = ticketStoreToEntries(loadTicketsFn())
		if m.selected >= len(m.items) && m.selected > 0 {
			m.selected--
		}
		m.status = fmt.Sprintf("Removed %s / %s", item.Customer, item.Project)
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
		cust := strings.TrimSpace(m.inputs[0].Value())
		proj := strings.TrimSpace(m.inputs[1].Value())
		tick := strings.TrimSpace(m.inputs[2].Value())
		if cust == "" || proj == "" || tick == "" {
			m.err = "customer, project, and ticket are required"
			return m, nil
		}
		if err := setTicketFn(cust, proj, tick); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.items = ticketStoreToEntries(loadTicketsFn())
		m.mode = modeList
		m.err = ""
		m.status = fmt.Sprintf("Saved %s / %s [%s]", cust, proj, tick)
		// focus selection to saved item
		for i, it := range m.items {
			if strings.EqualFold(it.Customer, cust) && strings.EqualFold(it.Project, proj) {
				m.selected = i
				break
			}
		}
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		m.inputs[m.focus].Blur()
		if msg.Type == tea.KeyTab {
			m.focus = (m.focus + 1) % len(m.inputs)
		} else {
			m.focus = (m.focus - 1 + len(m.inputs)) % len(m.inputs)
		}
		m.inputs[m.focus].Focus()
		return m, nil
	}

	// Route remaining keys to focused input only (so navigation keys don't affect list).
	cmd := m.updateFocusedInput(msg)
	return m, cmd
}

func (m *ticketTUIModel) updateFocusedInput(msg tea.KeyMsg) tea.Cmd {
	for i := range m.inputs {
		if i == m.focus {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			return cmd
		}
	}
	return nil
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
	if len(m.items) == 0 {
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
	b.WriteString("\nKeys: n new | e/enter edit | del delete | q quit\n")
	return b.String()
}

func (m ticketTUIModel) renderForm() string {
	var b strings.Builder
	b.WriteString("Edit ticket (ctrl+s save, esc cancel, tab to move)\n\n")
	labels := []string{"Customer", "Project", "Ticket"}
	for i, in := range m.inputs {
		label := labels[i]
		cursor := " "
		if i == m.focus {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s: %s\n", cursor, label, in.View())
	}
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
