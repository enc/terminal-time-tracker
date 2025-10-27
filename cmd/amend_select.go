package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var errSelectionCancelled = errors.New("selection cancelled")

var (
	nowLocalForAmend      = nowLocal
	loadEntriesForAmendFn = loadEntries
)

const (
	maxAmendSelectionEntries = 50
)

func loadRecentEntriesForAmend() ([]Entry, error) {
	now := nowLocalForAmend()
	lookbacks := []int{0, 1, 7, 30, 90, 180, 365}
	for _, days := range lookbacks {
		from := now.AddDate(0, 0, -days)
		entries, err := loadEntriesForAmendFn(from, now)
		if err != nil {
			return nil, fmt.Errorf("failed loading entries: %w", err)
		}
		if len(entries) == 0 {
			continue
		}
		if len(entries) > maxAmendSelectionEntries {
			entries = entries[len(entries)-maxAmendSelectionEntries:]
		}
		return entries, nil
	}
	return nil, fmt.Errorf("no entries found in the last year; add an entry, provide an id, or use --select with an existing id")
}

func findMostRecentEntryForAmend() (*Entry, error) {
	entries, err := loadRecentEntriesForAmend()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries available to amend; add an entry first")
	}
	return &entries[len(entries)-1], nil
}

func selectEntryForAmend(entries []Entry) (*Entry, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries available for selection")
	}

	items := make([]list.Item, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		// reverse chronological order (latest first)
		items = append(items, entryItem{entry: &entries[i]})
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)

	l := list.New(items, delegate, 0, 0)
	l.Title = "Select entry to amend"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = l.Styles.Title.Padding(0, 1)

	model := amendSelectModel{list: l}
	program := tea.NewProgram(model, tea.WithAltScreen())
	res, err := program.Run()
	if err != nil {
		return nil, err
	}

	final, ok := res.(amendSelectModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type from selector")
	}
	if final.cancelled || final.choice == nil {
		return nil, errSelectionCancelled
	}
	return final.choice, nil
}

type entryItem struct {
	entry *Entry
}

func (e entryItem) Title() string {
	start := e.entry.Start.In(time.Local).Format("Mon Jan 2 15:04")
	customer := e.entry.Customer
	project := e.entry.Project
	activity := e.entry.Activity
	if customer == "" {
		customer = "-"
	}
	if project == "" {
		project = "-"
	}
	if activity == "" {
		activity = "-"
	}
	duration := durationMinutes(*e.entry)
	var durationPart string
	if e.entry.End == nil {
		durationPart = " (running)"
	} else if duration > 0 {
		durationPart = fmt.Sprintf(" (%dm)", duration)
	}
	return fmt.Sprintf("%s%s - %s / %s / %s", start, durationPart, customer, project, activity)
}

func (e entryItem) Description() string {
	var parts []string
	if len(e.entry.Notes) > 0 {
		parts = append(parts, strings.Join(e.entry.Notes, "; "))
	}
	if len(e.entry.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(e.entry.Tags, " #"))
	}
	return strings.Join(parts, "  ")
}

func (e entryItem) FilterValue() string {
	var fields []string
	fields = append(fields, e.entry.ID, e.entry.Customer, e.entry.Project, e.entry.Activity)
	fields = append(fields, strings.Join(e.entry.Tags, " "))
	fields = append(fields, strings.Join(e.entry.Notes, " "))
	return strings.ToLower(strings.Join(fields, " "))
}

type amendSelectModel struct {
	list      list.Model
	choice    *Entry
	cancelled bool
}

func (m amendSelectModel) Init() tea.Cmd {
	return nil
}

func (m amendSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 && msg.Height > 2 {
			m.list.SetSize(msg.Width, msg.Height-2)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(entryItem); ok {
				m.choice = item.entry
			}
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m amendSelectModel) View() string {
	instructions := "\nUse up/down to navigate, enter to amend, esc to cancel"
	return m.list.View() + instructions
}
