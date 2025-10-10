package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formModel is an editable Start/Switch form using bubbles/textinput.
// It can be used as a submodel by the dashboard. On submit it issues
// a tea.Cmd that calls the provided EventWriter with the Start or Switch
// parameters and returns a startDoneMsg (consistent with dashboard handling).
type formModel struct {
	// inputs in logical order
	customerInput *textinput.Model
	projectInput  *textinput.Model
	activityInput *textinput.Model
	tagsInput     *textinput.Model
	noteInput     *textinput.Model

	inputs []*textinput.Model

	// UI state
	focused int
	width   int

	// behavior
	mode        string // "start" or "switch" (informational only; both produce startDoneMsg)
	defaultBill bool

	// suggestions presence (read-only preview; not used to populate fields here)
	suggestions []suggestion

	// writer to perform Start/Switch
	writer EventWriter
}

// NewStartSwitchForm constructs a form model wired to the provided writer.
// last is used to seed the initial values when available. defaultBillable
// sets the initial Billable toggle. suggestions are offered as a preview.
func NewStartSwitchForm(w EventWriter, last *Entry, defaultBillable bool, suggestions []suggestion) *formModel {
	customer := textinput.NewModel()
	customer.Placeholder = "customer (e.g. acme)"
	customer.CharLimit = 128
	customer.Width = 30

	project := textinput.NewModel()
	project.Placeholder = "project (e.g. mobilize:foundation)"
	project.CharLimit = 128
	project.Width = 30

	activity := textinput.NewModel()
	activity.Placeholder = "activity (design, meeting, docs)"
	activity.CharLimit = 64
	activity.Width = 24

	tags := textinput.NewModel()
	tags.Placeholder = "comma-separated tags (opt)"
	tags.CharLimit = 128
	tags.Width = 30

	note := textinput.NewModel()
	note.Placeholder = "note (optional)"
	note.CharLimit = 256
	note.Width = 40

	// Seed from last entry if provided
	if last != nil {
		customer.SetValue(last.Customer)
		project.SetValue(last.Project)
		activity.SetValue(last.Activity)
		if len(last.Tags) > 0 {
			tags.SetValue(strings.Join(last.Tags, ","))
		}
	}

	inputs := []*textinput.Model{&customer, &project, &activity, &tags, &note}

	// Focus the first field by default
	inputs[0].Focus()

	return &formModel{
		customerInput: &customer,
		projectInput:  &project,
		activityInput: &activity,
		tagsInput:     &tags,
		noteInput:     &note,
		inputs:        inputs,
		focused:       0,
		width:         80,
		mode:          "start",
		defaultBill:   defaultBillable,
		suggestions:   suggestions,
		writer:        w,
	}
}

// Init implements tea.Model.Init
func (f *formModel) Init() tea.Cmd {
	// No async startup required; keep textinput blinking cursor alive.
	return textinput.Blink
}

// Update processes messages. It scopes typing to the focused input and
// allows Tab/Shift-Tab to move focus. Enter on last input submits the form.
func (f *formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		// propagate width to inputs for nicer wrapping
		for _, ti := range f.inputs {
			ti.Width = max(10, f.width/3)
		}
		return f, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "enter", "up", "k", "down", "j":
			// Handle navigation/focus cycling
			s := msg.String()

			// If Enter pressed and focused on last field, submit.
			if s == "enter" && f.focused == len(f.inputs)-1 {
				// Submit
				return f, f.submitCmd()
			}

			// Tab / shift+tab navigation
			if s == "tab" || s == "down" || s == "j" {
				f.focusNext()
				return f, nil
			}
			if s == "shift+tab" || s == "up" || s == "k" {
				f.focusPrev()
				return f, nil
			}
			return f, nil

		case "esc":
			// Cancel the form. Emit a no-op command; caller should handle formMode exit.
			return f, func() tea.Msg { return formCancelledMsg{} }

		case "ctrl+c":
			return f, tea.Quit
		}
		// Pass typing to the focused textinput
		if f.focused >= 0 && f.focused < len(f.inputs) {
			ti := f.inputs[f.focused]
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)
			return f, cmd
		}
		return f, nil

	default:
		// Let the focused input react to non-key messages if needed
		if f.focused >= 0 && f.focused < len(f.inputs) {
			ti := f.inputs[f.focused]
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)
			return f, cmd
		}
		return f, nil
	}
}

// View renders the form with labels and current input Views, wrapped in a section box.
func (f *formModel) View() string {
	title := "Start / Switch (editable)"
	var b strings.Builder
	b.WriteString(SectionTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Helper to render a labeled input line
	renderLine := func(label string, ti *textinput.Model) {
		labelR := EmphStyle.Render(label)
		// Align with some spacing; rely on SectionBoxStyle to provide padding.
		b.WriteString(fmt.Sprintf("%-10s %s\n", labelR, ti.View()))
	}

	renderLine("Customer", f.customerInput)
	renderLine("Project", f.projectInput)
	renderLine("Activity", f.activityInput)
	renderLine("Tags", f.tagsInput)
	renderLine("Note", f.noteInput)

	// Billable hint & instructions
	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(fmt.Sprintf("Billable: %v  (toggle with 'b' in dashboard form mode)", f.defaultBill)))
	b.WriteString("\n\n")
	b.WriteString(MutedStyle.Render("Tab: next field • Shift+Tab: prev • Enter on Note: submit • Esc: cancel"))

	// Wrap entire content in a section box for consistent styling
	return SectionBoxStyle.Width(f.width).Render(b.String())
}

// focusNext moves focus to the next input
func (f *formModel) focusNext() {
	if f.focused < 0 {
		f.focused = 0
	}
	// Blur current
	if f.focused < len(f.inputs) {
		f.inputs[f.focused].Blur()
	}
	f.focused = (f.focused + 1) % len(f.inputs)
	f.inputs[f.focused].Focus()
}

// focusPrev moves focus to the previous input
func (f *formModel) focusPrev() {
	if f.focused < 0 {
		f.focused = 0
	}
	if f.focused < len(f.inputs) {
		f.inputs[f.focused].Blur()
	}
	f.focused = (f.focused - 1 + len(f.inputs)) % len(f.inputs)
	f.inputs[f.focused].Focus()
}

// submitCmd constructs the StartParams/SwitchParams from the form and
// returns a tea.Cmd that calls the writer and returns startDoneMsg.
func (f *formModel) submitCmd() tea.Cmd {
	// Capture values
	cust := strings.TrimSpace(f.customerInput.Value())
	proj := strings.TrimSpace(f.projectInput.Value())
	act := strings.TrimSpace(f.activityInput.Value())
	tagsRaw := strings.TrimSpace(f.tagsInput.Value())
	note := strings.TrimSpace(f.noteInput.Value())

	var tags []string
	if tagsRaw != "" {
		for _, t := range strings.Split(tagsRaw, ",") {
			if s := strings.TrimSpace(t); s != "" {
				tags = append(tags, s)
			}
		}
	}

	// Build params
	p := StartParams{
		Customer: cust,
		Project:  proj,
		Activity: act,
		Billable: f.defaultBill,
		Tags:     tags,
		Note:     note,
	}

	// Return command that performs Start on writer.
	return func() tea.Msg {
		if f.writer == nil {
			return startDoneMsg{}
		}
		// If mode == "switch" and there is a Switch method expected, call Switch.
		// Use Switch for active->new transitions; still report startDoneMsg for dashboard.
		if f.mode == "switch" {
			sp := SwitchParams{
				Customer: p.Customer,
				Project:  p.Project,
				Activity: p.Activity,
				Billable: p.Billable,
				Tags:     p.Tags,
				Note:     p.Note,
			}
			if err := f.writer.Switch(context.Background(), sp); err != nil {
				return startDoneMsg{err: err}
			}
			return startDoneMsg{}
		}
		if err := f.writer.Start(context.Background(), p); err != nil {
			return startDoneMsg{err: err}
		}
		return startDoneMsg{}
	}
}

// Helper message types used by the form
type formCancelledMsg struct{}

// Utility: set mode and billable defaults after construction if caller prefers.
func (f *formModel) SetMode(m string)          { f.mode = m }
func (f *formModel) SetDefaultBillable(b bool) { f.defaultBill = b }
func (f *formModel) SetWriter(w EventWriter)   { f.writer = w }
