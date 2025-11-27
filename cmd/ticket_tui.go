package cmd

import (
	"fmt"
	"strings"
	"time"

	"sort"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var ticketTUICmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive ticket manager (list, filter, edit)",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newTicketUI()
		p := tea.NewProgram(m)
		_, err := p.Run()
		return err
	},
}

type ticketFilter int

const (
	filterAll ticketFilter = iota
	filterMissing
	filterAssigned
)

type ticketUIModel struct {
	entries  []ticketRow
	observed []observedProject
	store    ticketStore
	selected int
	filter   ticketFilter

	editing   bool
	editIdx   int
	editInput textinput.Model
	status    string
	errMsg    string
}

type ticketRow struct {
	Customer string
	Project  string
	Ticket   string
	LastSeen time.Time
}

func newTicketUI() ticketUIModel {
	store := loadTicketsFn()
	obs := observedProjectsFn()
	ti := textinput.New()
	ti.Placeholder = "Ticket"
	ti.CharLimit = 64
	return ticketUIModel{
		store:     store,
		observed:  obs,
		entries:   buildTicketRows(store, obs, filterAll),
		status:    "↑/↓ move • enter edit • f filter • q quit",
		editInput: ti,
	}
}

// tea.Model compliance
func (m ticketUIModel) Init() tea.Cmd { return nil }

func (m ticketUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing {
			return m.updateEdit(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m ticketUIModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.entries)-1 {
			m.selected++
		}
	case "f":
		m.filter = (m.filter + 1) % 3
		m.entries = buildTicketRows(m.store, m.observed, m.filter)
		if m.selected >= len(m.entries) {
			m.selected = max(0, len(m.entries)-1)
		}
		m.status = fmt.Sprintf("Filter: %s", m.filterLabel())
	case "enter":
		if len(m.entries) == 0 {
			return m, nil
		}
		m.editing = true
		m.editIdx = m.selected
		cur := m.entries[m.selected]
		m.editInput.SetValue(cur.Ticket)
		m.editInput.CursorEnd()
		m.editInput.Focus()
		m.status = fmt.Sprintf("Editing %s / %s (enter to save, esc to cancel)", cur.Customer, cur.Project)
	}
	return m, nil
}

func (m ticketUIModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.editing = false
		m.editInput.Blur()
		m.errMsg = ""
		m.status = "↑/↓ move • enter edit • f filter • q quit"
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.editInput.Value())
		if val == "" {
			m.errMsg = "ticket cannot be empty"
			return m, nil
		}
		cur := m.entries[m.editIdx]
		if err := setTicketFn(cur.Customer, cur.Project, val); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.store = loadTicketsFn()
		m.entries = buildTicketRows(m.store, m.observed, m.filter)
		// realign selection to same customer/project
		for i, e := range m.entries {
			if strings.EqualFold(e.Customer, cur.Customer) && strings.EqualFold(e.Project, cur.Project) {
				m.selected = i
				break
			}
		}
		m.editing = false
		m.errMsg = ""
		m.editInput.Blur()
		m.status = fmt.Sprintf("Saved %s / %s [%s]", cur.Customer, cur.Project, val)
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(msg)
	return m, cmd
}

func (m ticketUIModel) View() string {
	var b strings.Builder
	title := "Ticket Manager"
	if m.editing {
		title = "Ticket Editor"
	}
	fmt.Fprintf(&b, "%s (%s)\n", title, m.filterLabel())
	if m.errMsg != "" {
		fmt.Fprintf(&b, "ERR: %s\n", m.errMsg)
	}
	b.WriteString(m.status + "\n\n")

	if len(m.entries) == 0 {
		b.WriteString("No projects found.\n")
		return b.String()
	}

	for i, e := range m.entries {
		prefix := " "
		if i == m.selected {
			prefix = ">"
		}
		ticket := e.Ticket
		if ticket == "" {
			ticket = "(none)"
		}
		fmt.Fprintf(&b, "%s %-18s %-18s [%s]\n", prefix, e.Customer, e.Project, ticket)
	}

	if m.editing {
		cur := m.entries[m.editIdx]
		b.WriteString("\nEditing: " + cur.Customer + " / " + cur.Project + "\n")
		b.WriteString(m.editInput.View() + "\n")
	}
	return b.String()
}

func buildTicketRows(store ticketStore, observed []observedProject, filter ticketFilter) []ticketRow {
	seen := map[string]ticketRow{}
	for _, op := range observed {
		key := strings.ToLower(strings.TrimSpace(op.Customer)) + "||" + strings.ToLower(strings.TrimSpace(op.Project))
		seen[key] = ticketRow{
			Customer: op.Customer,
			Project:  op.Project,
			LastSeen: op.LastSeen,
		}
	}
	for cust, pm := range store {
		for proj, t := range pm {
			key := strings.ToLower(strings.TrimSpace(cust)) + "||" + strings.ToLower(strings.TrimSpace(proj))
			row := seen[key]
			row.Customer = cust
			row.Project = proj
			row.Ticket = t
			seen[key] = row
		}
	}

	var rows []ticketRow
	for _, r := range seen {
		switch filter {
		case filterMissing:
			if strings.TrimSpace(r.Ticket) != "" {
				continue
			}
		case filterAssigned:
			if strings.TrimSpace(r.Ticket) == "" {
				continue
			}
		}
		rows = append(rows, r)
	}
	// sort: newest first, then customer/project
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastSeen.Equal(rows[j].LastSeen) {
			if rows[i].Customer != rows[j].Customer {
				return rows[i].Customer < rows[j].Customer
			}
			return rows[i].Project < rows[j].Project
		}
		return rows[i].LastSeen.After(rows[j].LastSeen)
	})
	return rows
}

func (m ticketUIModel) filterLabel() string {
	switch m.filter {
	case filterMissing:
		return "missing only"
	case filterAssigned:
		return "assigned only"
	default:
		return "all"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
