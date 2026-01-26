package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type ticketUIModel struct {
	customers []ticketCustomer
	observed  []observedProject
	store     ticketStore
	focus     ticketFocus
	view      ticketView
	custIdx   int
	projIdx   int

	editing   bool
	editInput textinput.Model
	status    string
	errMsg    string

	width  int
	height int
}

type ticketFocus int

const (
	ticketFocusCustomers ticketFocus = iota
	ticketFocusProjects
)

type ticketView int

const (
	viewActiveOnly ticketView = iota
	viewAll
)

type ticketCustomer struct {
	Name     string
	LastSeen time.Time
	Projects []ticketProject
}

type ticketProject struct {
	Name     string
	Ticket   string
	LastSeen time.Time
	Active   bool
}

var ticketUIStyles = struct {
	header      lipgloss.Style
	subtle      lipgloss.Style
	status      lipgloss.Style
	err         lipgloss.Style
	tableHeader lipgloss.Style
	row         lipgloss.Style
	rowSelected lipgloss.Style
	editLabel   lipgloss.Style
	editInput   lipgloss.Style
	filter      lipgloss.Style
}{
	header:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6")).Background(lipgloss.Color("#111827")).Padding(0, 1),
	subtle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")),
	status:      lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")),
	err:         lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true),
	tableHeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C084FC")),
	row:         lipgloss.NewStyle().Padding(0, 1),
	rowSelected: lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#E5E7EB")).Background(lipgloss.Color("#312E81")),
	editLabel:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C084FC")),
	editInput:   lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Background(lipgloss.Color("#1F2937")).Padding(0, 1),
	filter:      lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Bold(true),
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
		customers: buildTicketCustomers(store, obs, viewActiveOnly),
		focus:     ticketFocusProjects,
		view:      viewActiveOnly,
		status:    "tab switch • ↑/↓ move • enter edit • f active/all • q quit",
		editInput: ti,
	}
}

// tea.Model compliance
func (m ticketUIModel) Init() tea.Cmd { return nil }

func (m ticketUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
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
	case "tab", "right", "l":
		if len(m.currentProjects()) > 0 {
			m.focus = ticketFocusProjects
		}
	case "left", "h":
		m.focus = ticketFocusCustomers
	case "up", "k":
		switch m.focus {
		case ticketFocusCustomers:
			if m.custIdx > 0 {
				m.custIdx--
				m.projIdx = 0
			}
		case ticketFocusProjects:
			if m.projIdx > 0 {
				m.projIdx--
			}
		}
	case "down", "j":
		switch m.focus {
		case ticketFocusCustomers:
			if m.custIdx < len(m.customers)-1 {
				m.custIdx++
				m.projIdx = 0
			}
		case ticketFocusProjects:
			if m.projIdx < len(m.currentProjects())-1 {
				m.projIdx++
			}
		}
	case "f":
		if m.view == viewActiveOnly {
			m.view = viewAll
		} else {
			m.view = viewActiveOnly
		}
		curCust, curProj := m.currentSelection()
		m.customers = buildTicketCustomers(m.store, m.observed, m.view)
		m.reselect(curCust, curProj)
		m.status = fmt.Sprintf("View: %s", m.viewLabel())
	case "enter":
		if len(m.currentProjects()) == 0 {
			return m, nil
		}
		m.editing = true
		cur := m.currentProject()
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
		m.status = "tab switch • ↑/↓ move • enter edit • f active/all • q quit"
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.editInput.Value())
		if val == "" {
			m.errMsg = "ticket cannot be empty"
			return m, nil
		}
		cur := m.currentProject()
		if err := setTicketFn(cur.Customer, cur.Project, val); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.store = loadTicketsFn()
		m.customers = buildTicketCustomers(m.store, m.observed, m.view)
		m.reselect(cur.Customer, cur.Project)
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
	fmt.Fprintf(&b, "%s %s\n", ticketUIStyles.header.Render(title), ticketUIStyles.filter.Render("["+m.viewLabel()+"]"))
	if m.errMsg != "" {
		fmt.Fprintf(&b, "%s\n", ticketUIStyles.err.Render("ERR: "+m.errMsg))
	}
	b.WriteString(ticketUIStyles.status.Render(m.status) + "\n\n")

	if len(m.customers) == 0 {
		b.WriteString(ticketUIStyles.subtle.Render("No active projects found.") + "\n")
		return b.String()
	}

	customerW, projectW := m.columnWidths()
	customersCol := renderCustomerColumn(m.customers, m.custIdx, m.focus == ticketFocusCustomers, customerW)
	projects := m.currentProjects()
	projectsCol := renderProjectColumn(projects, m.projIdx, m.focus == ticketFocusProjects, projectW)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, customersCol, projectsCol))

	if m.editing {
		cur := m.currentProject()
		b.WriteString("\n" + ticketUIStyles.editLabel.Render("Editing: ") + cur.Customer + " / " + cur.Project + "\n")
		b.WriteString(ticketUIStyles.editInput.Render(m.editInput.View()) + "\n")
	}
	return b.String()
}

type ticketRecord struct {
	Customer string
	Project  string
	Ticket   string
	LastSeen time.Time
	Active   bool
}

func buildTicketCustomers(store ticketStore, observed []observedProject, view ticketView) []ticketCustomer {
	seen := map[string]ticketRecord{}
	for _, op := range observed {
		key := strings.ToLower(strings.TrimSpace(op.Customer)) + "||" + strings.ToLower(strings.TrimSpace(op.Project))
		seen[key] = ticketRecord{
			Customer: op.Customer,
			Project:  op.Project,
			LastSeen: op.LastSeen,
			Active:   true,
		}
	}
	for cust, pm := range store {
		for proj, t := range pm {
			key := strings.ToLower(strings.TrimSpace(cust)) + "||" + strings.ToLower(strings.TrimSpace(proj))
			row := seen[key]
			if row.Customer == "" {
				row.Customer = cust
			}
			if row.Project == "" {
				row.Project = proj
			}
			row.Ticket = t
			seen[key] = row
		}
	}

	customers := map[string]*ticketCustomer{}
	for _, r := range seen {
		if view == viewActiveOnly && !r.Active {
			continue
		}
		custKey := strings.ToLower(strings.TrimSpace(r.Customer))
		cust := customers[custKey]
		if cust == nil {
			cust = &ticketCustomer{Name: r.Customer, LastSeen: r.LastSeen}
			customers[custKey] = cust
		}
		if r.Active && r.LastSeen.After(cust.LastSeen) {
			cust.LastSeen = r.LastSeen
		}
		cust.Projects = append(cust.Projects, ticketProject{
			Name:     r.Project,
			Ticket:   r.Ticket,
			LastSeen: r.LastSeen,
			Active:   r.Active,
		})
	}

	var list []ticketCustomer
	for _, cust := range customers {
		sort.Slice(cust.Projects, func(i, j int) bool {
			if cust.Projects[i].LastSeen.Equal(cust.Projects[j].LastSeen) {
				return cust.Projects[i].Name < cust.Projects[j].Name
			}
			return cust.Projects[i].LastSeen.After(cust.Projects[j].LastSeen)
		})
		list = append(list, *cust)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].LastSeen.Equal(list[j].LastSeen) {
			return list[i].Name < list[j].Name
		}
		return list[i].LastSeen.After(list[j].LastSeen)
	})
	return list
}

func (m ticketUIModel) viewLabel() string {
	if m.view == viewAll {
		return "all"
	}
	return "active only"
}

func (m ticketUIModel) currentSelection() (string, string) {
	cust := m.currentCustomer()
	if cust == nil {
		return "", ""
	}
	proj := m.currentProject()
	if proj.Project == "" {
		return cust.Name, ""
	}
	return cust.Name, proj.Project
}

func (m ticketUIModel) currentCustomer() *ticketCustomer {
	if m.custIdx < 0 || m.custIdx >= len(m.customers) {
		return nil
	}
	return &m.customers[m.custIdx]
}

func (m ticketUIModel) currentProjects() []ticketProject {
	cust := m.currentCustomer()
	if cust == nil {
		return nil
	}
	return cust.Projects
}

type projectSelection struct {
	Customer string
	Project  string
	Ticket   string
}

func (m ticketUIModel) currentProject() projectSelection {
	cust := m.currentCustomer()
	if cust == nil {
		return projectSelection{}
	}
	projects := cust.Projects
	if m.projIdx < 0 || m.projIdx >= len(projects) {
		return projectSelection{Customer: cust.Name}
	}
	p := projects[m.projIdx]
	return projectSelection{Customer: cust.Name, Project: p.Name, Ticket: p.Ticket}
}

func (m *ticketUIModel) reselect(customer, project string) {
	custIdx := 0
	projIdx := 0
	for i, c := range m.customers {
		if strings.EqualFold(c.Name, customer) {
			custIdx = i
			for j, p := range c.Projects {
				if strings.EqualFold(p.Name, project) {
					projIdx = j
					break
				}
			}
			break
		}
	}
	m.custIdx = custIdx
	m.projIdx = projIdx
	if m.focus == ticketFocusProjects && len(m.currentProjects()) == 0 {
		m.focus = ticketFocusCustomers
	}
}

func renderCustomerColumn(customers []ticketCustomer, selected int, focused bool, width int) string {
	var lines []string
	header := ticketUIStyles.tableHeader.Render(padWidth("Customers", width))
	lines = append(lines, header)
	for i, c := range customers {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		line := padWidth(prefix+" "+c.Name, width)
		if i == selected && focused {
			lines = append(lines, ticketUIStyles.rowSelected.Render(line))
		} else {
			lines = append(lines, ticketUIStyles.row.Render(line))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderProjectColumn(projects []ticketProject, selected int, focused bool, width int) string {
	var lines []string
	header := ticketUIStyles.tableHeader.Render(padWidth("Projects", width))
	lines = append(lines, header)
	nameW := max(10, width-14)
	for i, p := range projects {
		prefix := " "
		if i == selected {
			prefix = ">"
		}
		ticket := strings.TrimSpace(p.Ticket)
		if ticket == "" {
			ticket = "(none)"
		}
		if !p.Active {
			ticket = ticket + " (inactive)"
		}
		row := fmt.Sprintf("%s %-*s %s", prefix, nameW, p.Name, ticket)
		row = padWidth(row, width)
		if i == selected && focused {
			lines = append(lines, ticketUIStyles.rowSelected.Render(row))
		} else {
			lines = append(lines, ticketUIStyles.row.Render(row))
		}
	}
	if len(projects) == 0 {
		lines = append(lines, ticketUIStyles.subtle.Render(padWidth("No projects.", width)))
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m ticketUIModel) columnWidths() (int, int) {
	if m.width <= 0 {
		return 24, 50
	}
	customerW := clamp(m.width/3, 16, 30)
	projectW := m.width - customerW - 2
	if projectW < 24 {
		projectW = 24
	}
	return customerW, projectW
}

func padWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	if lipgloss.Width(s) >= width {
		return lipgloss.NewStyle().Width(width).Render(s)
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
