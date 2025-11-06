package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type reviewFocus int

const (
	focusCustomers reviewFocus = iota
	focusProjects
)

type selectionAction int

const (
	actionApprove selectionAction = iota
	actionIgnore
)

type pendingCustomer struct {
	Canonical string
	Total     int
	Variants  []string
	LastSeen  time.Time
}

type pendingProject struct {
	CustomerKey     string
	CustomerDisplay string
	Project         string
	Count           int
	LastSeen        time.Time
}

type reviewState struct {
	customers []pendingCustomer
	projects  []pendingProject
}

type projectKey struct {
	Customer string
	Project  string
}

func prepareReviewState(dec completionDecisions, idx *CompletionIndex) reviewState {
	st := reviewState{}

	for canonical, group := range idx.Customers {
		canonical = strings.TrimSpace(canonical)
		if canonical == "" {
			continue
		}
		if dec.isCustomerAllowed(canonical) || dec.isCustomerIgnored(canonical) {
			continue
		}
		st.customers = append(st.customers, pendingCustomer{
			Canonical: canonical,
			Total:     group.Total,
			Variants:  topVariants(group.Names, 3),
			LastSeen:  group.LastSeen,
		})
	}

	sort.Slice(st.customers, func(i, j int) bool {
		if st.customers[i].Total == st.customers[j].Total {
			return st.customers[i].Canonical < st.customers[j].Canonical
		}
		return st.customers[i].Total > st.customers[j].Total
	})

	for customerKey, projMap := range idx.Projects {
		canonicalCustomer := customerKey
		if customerKey == "_uncategorized" {
			canonicalCustomer = ""
		}
		display := canonicalForCompletion(canonicalCustomer)
		if display == "" && canonicalCustomer == "" {
			display = "(none)"
		}
		for projectName, stats := range projMap {
			project := strings.TrimSpace(projectName)
			if project == "" {
				continue
			}
			if dec.isProjectAllowed(canonicalCustomer, project) || dec.isProjectIgnored(canonicalCustomer, project) {
				continue
			}
			st.projects = append(st.projects, pendingProject{
				CustomerKey:     canonicalCustomer,
				CustomerDisplay: display,
				Project:         project,
				Count:           stats.Count,
				LastSeen:        stats.LastSeen,
			})
		}
	}

	sort.Slice(st.projects, func(i, j int) bool {
		if st.projects[i].Count == st.projects[j].Count {
			if st.projects[i].CustomerDisplay == st.projects[j].CustomerDisplay {
				return st.projects[i].Project < st.projects[j].Project
			}
			return st.projects[i].CustomerDisplay < st.projects[j].CustomerDisplay
		}
		return st.projects[i].Count > st.projects[j].Count
	})

	return st
}

func topVariants(names map[string]*NameStats, limit int) []string {
	if len(names) == 0 {
		return nil
	}
	variants := make([]*NameStats, 0, len(names))
	for _, v := range names {
		variants = append(variants, v)
	}
	sort.Slice(variants, func(i, j int) bool {
		if variants[i].Count == variants[j].Count {
			return variants[i].Raw < variants[j].Raw
		}
		return variants[i].Count > variants[j].Count
	})
	out := []string{}
	for idx, v := range variants {
		if limit > 0 && idx >= limit {
			break
		}
		out = append(out, v.Raw)
	}
	return out
}

type reviewModel struct {
	decisions         completionDecisions
	state             reviewState
	focus             reviewFocus
	custCursor        int
	projCursor        int
	selectedCustomers map[string]struct{}
	selectedProjects  map[projectKey]struct{}
	status            string
	width             int
	height            int
	done              bool
}

var (
	headerStyle    = lipgloss.NewStyle().Bold(true)
	focusStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	selectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	cursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
)

func newReviewModel(dec completionDecisions, state reviewState) reviewModel {
	return reviewModel{
		decisions:         dec,
		state:             state,
		focus:             focusCustomers,
		selectedCustomers: map[string]struct{}{},
		selectedProjects:  map[projectKey]struct{}{},
	}
}

func (m reviewModel) Init() tea.Cmd {
	return nil
}

func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.done = true
			return m, tea.Quit
		case "tab":
			m.toggleFocus()
		case "shift+tab":
			m.toggleFocus()
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case " ":
			m.toggleSelection()
		case "a", "A", "enter":
			return m.applySelection(actionApprove)
		case "i", "I":
			return m.applySelection(actionIgnore)
		}
	}
	return m, nil
}

func (m *reviewModel) toggleFocus() {
	if m.focus == focusCustomers {
		m.focus = focusProjects
	} else {
		m.focus = focusCustomers
	}
	m.status = ""
}

func (m *reviewModel) moveCursor(delta int) {
	switch m.focus {
	case focusCustomers:
		if len(m.state.customers) == 0 {
			return
		}
		m.custCursor = clampIndex(m.custCursor+delta, len(m.state.customers))
	case focusProjects:
		if len(m.state.projects) == 0 {
			return
		}
		m.projCursor = clampIndex(m.projCursor+delta, len(m.state.projects))
	}
}

func (m *reviewModel) toggleSelection() {
	switch m.focus {
	case focusCustomers:
		if len(m.state.customers) == 0 {
			return
		}
		item := m.state.customers[m.custCursor]
		if _, ok := m.selectedCustomers[item.Canonical]; ok {
			delete(m.selectedCustomers, item.Canonical)
		} else {
			m.selectedCustomers[item.Canonical] = struct{}{}
		}
	case focusProjects:
		if len(m.state.projects) == 0 {
			return
		}
		item := m.state.projects[m.projCursor]
		key := projectKey{Customer: item.CustomerKey, Project: item.Project}
		if _, ok := m.selectedProjects[key]; ok {
			delete(m.selectedProjects, key)
		} else {
			m.selectedProjects[key] = struct{}{}
		}
	}
}

func (m reviewModel) applySelection(action selectionAction) (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusCustomers:
		m.applyCustomers(action)
	case focusProjects:
		m.applyProjects(action)
	}
	if len(m.state.customers) == 0 && len(m.state.projects) == 0 {
		m.status = "No pending entries. Press q to exit."
	}
	return m, nil
}

func (m *reviewModel) applyCustomers(action selectionAction) {
	targets := m.selectedCustomerKeys()
	if len(targets) == 0 {
		m.status = "Nothing selected"
		return
	}
	for _, canonical := range targets {
		switch action {
		case actionApprove:
			m.decisions.allowCustomer(canonical)
		case actionIgnore:
			m.decisions.ignoreCustomer(canonical)
		}
		m.removeCustomer(canonical)
	}
	if err := m.decisions.save(); err != nil {
		m.status = fmt.Sprintf("failed to write config: %v", err)
		return
	}
	m.selectedCustomers = map[string]struct{}{}
	if action == actionApprove {
		m.status = fmt.Sprintf("Approved %d customer(s)", len(targets))
	} else {
		m.status = fmt.Sprintf("Ignored %d customer(s)", len(targets))
	}
}

func (m *reviewModel) applyProjects(action selectionAction) {
	targets := m.selectedProjectKeys()
	if len(targets) == 0 {
		m.status = "Nothing selected"
		return
	}
	for _, key := range targets {
		switch action {
		case actionApprove:
			m.decisions.allowProject(key.Customer, key.Project)
		case actionIgnore:
			m.decisions.ignoreProject(key.Customer, key.Project)
		}
		m.removeProject(key)
	}
	if err := m.decisions.save(); err != nil {
		m.status = fmt.Sprintf("failed to write config: %v", err)
		return
	}
	m.selectedProjects = map[projectKey]struct{}{}
	if action == actionApprove {
		m.status = fmt.Sprintf("Approved %d project(s)", len(targets))
	} else {
		m.status = fmt.Sprintf("Ignored %d project(s)", len(targets))
	}
}

func (m *reviewModel) removeCustomer(canonical string) {
	out := m.state.customers[:0]
	for _, item := range m.state.customers {
		if item.Canonical == canonical {
			continue
		}
		out = append(out, item)
	}
	m.state.customers = out
	delete(m.selectedCustomers, canonical)
	if m.custCursor >= len(m.state.customers) && m.custCursor > 0 {
		m.custCursor = len(m.state.customers) - 1
	}
	if len(m.state.customers) == 0 {
		m.custCursor = 0
	}
}

func (m *reviewModel) removeProject(key projectKey) {
	out := m.state.projects[:0]
	for _, item := range m.state.projects {
		if item.CustomerKey == key.Customer && item.Project == key.Project {
			continue
		}
		out = append(out, item)
	}
	m.state.projects = out
	delete(m.selectedProjects, key)
	if m.projCursor >= len(m.state.projects) && m.projCursor > 0 {
		m.projCursor = len(m.state.projects) - 1
	}
	if len(m.state.projects) == 0 {
		m.projCursor = 0
	}
}

func (m reviewModel) selectedCustomerKeys() []string {
	if len(m.selectedCustomers) > 0 {
		out := make([]string, 0, len(m.selectedCustomers))
		for k := range m.selectedCustomers {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	if len(m.state.customers) == 0 {
		return nil
	}
	idx := clampIndex(m.custCursor, len(m.state.customers))
	return []string{m.state.customers[idx].Canonical}
}

func (m reviewModel) selectedProjectKeys() []projectKey {
	if len(m.selectedProjects) > 0 {
		out := make([]projectKey, 0, len(m.selectedProjects))
		for k := range m.selectedProjects {
			out = append(out, k)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Customer == out[j].Customer {
				return out[i].Project < out[j].Project
			}
			return out[i].Customer < out[j].Customer
		})
		return out
	}
	if len(m.state.projects) == 0 {
		return nil
	}
	idx := clampIndex(m.projCursor, len(m.state.projects))
	item := m.state.projects[idx]
	return []projectKey{{Customer: item.CustomerKey, Project: item.Project}}
}

func clampIndex(idx, length int) int {
	if length == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}

func (m reviewModel) View() string {
	if len(m.state.customers) == 0 && len(m.state.projects) == 0 {
		return "Nothing to review. Press q to exit."
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Review completion suggestions"))
	b.WriteString("\n")
	b.WriteString("Tab switch • Space select • a approve • i ignore • q quit")
	b.WriteString("\n\n")

	b.WriteString(m.renderCustomers())
	b.WriteString("\n")
	b.WriteString(m.renderProjects())
	b.WriteString("\n")

	if m.status != "" {
		b.WriteString(m.status)
		b.WriteString("\n")
	}

	return b.String()
}

func (m reviewModel) renderCustomers() string {
	var b strings.Builder
	header := fmt.Sprintf("Customers (%d pending)", len(m.state.customers))
	if m.focus == focusCustomers {
		header = focusStyle.Render(header)
	}
	b.WriteString(header)
	b.WriteString("\n")

	if len(m.state.customers) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}

	for i, item := range m.state.customers {
		pointer := " "
		if m.focus == focusCustomers && i == m.custCursor {
			pointer = cursorStyle.Render(">")
		}
		marker := "[ ]"
		if _, ok := m.selectedCustomers[item.Canonical]; ok {
			marker = selectionStyle.Render("[*]")
		}
		variants := ""
		if len(item.Variants) > 0 {
			variants = fmt.Sprintf(" variants: %s", strings.Join(item.Variants, ", "))
		}
		line := fmt.Sprintf("%s %s %s (%d)%s", pointer, marker, item.Canonical, item.Total, variants)
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

func (m reviewModel) renderProjects() string {
	var b strings.Builder
	header := fmt.Sprintf("Projects (%d pending)", len(m.state.projects))
	if m.focus == focusProjects {
		header = focusStyle.Render(header)
	}
	b.WriteString(header)
	b.WriteString("\n")

	if len(m.state.projects) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}

	for i, item := range m.state.projects {
		pointer := " "
		if m.focus == focusProjects && i == m.projCursor {
			pointer = cursorStyle.Render(">")
		}
		marker := "[ ]"
		key := projectKey{Customer: item.CustomerKey, Project: item.Project}
		if _, ok := m.selectedProjects[key]; ok {
			marker = selectionStyle.Render("[*]")
		}
		line := fmt.Sprintf("%s %s %s — %s (%d)", pointer, marker, item.CustomerDisplay, item.Project, item.Count)
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

var completionReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Interactively review observed customers and projects for completion",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		decisions := loadCompletionDecisions()
		idx, err := BuildCompletionIndex("")
		if err != nil {
			return fmt.Errorf("failed to scan journal entries: %w", err)
		}
		state := prepareReviewState(decisions, idx)
		if len(state.customers) == 0 && len(state.projects) == 0 {
			fmt.Println("No pending completion entries to review.")
			return nil
		}
		model := newReviewModel(decisions, state)
		if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
			return fmt.Errorf("review session failed: %w", err)
		}
		return nil
	},
}

func init() {
	completionCmd.AddCommand(completionReviewCmd)
}
