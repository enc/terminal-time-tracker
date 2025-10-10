package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
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

	// completion candidates and state (for customer/project)
	custCandidates []string
	projCandidates []string

	custMatches []string
	projMatches []string
	matchIndex  int

	// visible list for completion dropdown
	matchList list.Model
	listOpen  bool

	// debounce tracking for live suggestions
	debounceID int
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

	fm := &formModel{
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

	// Preload completion candidates synchronously so the UI can use them immediately.
	// This reads unique customer/project strings from the journal root (~/.tt/journal).
	fm.loadCandidatesFromJournal()

	// If the journal provided no candidates, seed from the suggestions slice
	// and the provided last entry to avoid an empty completion set (helps new users).
	if len(fm.custCandidates) == 0 {
		cset := map[string]struct{}{}
		for _, s := range suggestions {
			if s.Customer != "" {
				cset[s.Customer] = struct{}{}
			}
		}
		if last != nil && last.Customer != "" {
			cset[last.Customer] = struct{}{}
		}
		if len(cset) > 0 {
			custs := make([]string, 0, len(cset))
			for k := range cset {
				custs = append(custs, k)
			}
			sort.Strings(custs)
			fm.custCandidates = custs
		}
	}
	if len(fm.projCandidates) == 0 {
		pset := map[string]struct{}{}
		for _, s := range suggestions {
			if s.Project != "" {
				pset[s.Project] = struct{}{}
			}
		}
		if last != nil && last.Project != "" {
			pset[last.Project] = struct{}{}
		}
		if len(pset) > 0 {
			projs := make([]string, 0, len(pset))
			for k := range pset {
				projs = append(projs, k)
			}
			sort.Strings(projs)
			fm.projCandidates = projs
		}
	}

	// Initialize visible match list with empty items; size will be adjusted on WindowSizeMsg.
	items := itemsFromStrings([]string{})
	fm.matchList = list.New(items, list.NewDefaultDelegate(), max(20, fm.width/3), 6)
	fm.matchList.SetShowStatusBar(false)
	fm.matchList.SetFilteringEnabled(false)
	fm.matchList.SetShowHelp(false)

	return fm
}

// Init implements tea.Model.Init
func (f *formModel) Init() tea.Cmd {
	// No async startup required; keep textinput blinking cursor alive.
	return textinput.Blink
}

// suggestMsg is emitted when a debounced suggestion computation completes.
type suggestMsg struct {
	id      int
	focused int
	term    string
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
		// adjust match list size to a reasonable width/height
		// list.SetSize expects width,height
		h := 6
		if l := len(f.matchList.Items()); l > 0 && l < h {
			h = l
		}
		// Always set the size (matchList is a value type and always initialized).
		f.matchList.SetSize(max(20, f.width/3), h)
		return f, nil

	case suggestMsg:
		// Only apply this suggestion result if it matches the latest debounce ID.
		if msg.id != f.debounceID {
			return f, nil
		}
		term := strings.ToLower(strings.TrimSpace(msg.term))
		var matches []string

		// Candidate selection strategy:
		// 1) prefix matches (fast and precise)
		// 2) contains (for short prefixes to match 1-2 char searches)
		// 3) fuzzy matches (ranked approximate matches)
		// 4) fallback to top candidates (so dropdown is useful even with no exact matches)
		if term == "" {
			if msg.focused == 0 {
				matches = filterPrefix(f.custCandidates, "")
			} else {
				matches = filterPrefix(f.projCandidates, "")
			}
		} else {
			if msg.focused == 0 {
				// 1) prefix
				matches = filterPrefix(f.custCandidates, term)
				// 2) contains for short terms (helpful for 1-2 char input)
				if len(matches) == 0 && len(term) <= 2 {
					matches = filterContains(f.custCandidates, term)
				}
				// 3) fuzzy as a fallback
				if len(matches) == 0 {
					matches = fuzzyMatches(f.custCandidates, term)
				}
				// 4) fallback to top candidates if still empty
				if len(matches) == 0 {
					matches = filterPrefix(f.custCandidates, "")
				}
			} else {
				// 1) prefix
				matches = filterPrefix(f.projCandidates, term)
				// 2) contains for short terms
				if len(matches) == 0 && len(term) <= 2 {
					matches = filterContains(f.projCandidates, term)
				}
				// 3) fuzzy fallback
				if len(matches) == 0 {
					matches = fuzzyMatches(f.projCandidates, term)
				}
				// 4) fallback to top candidates
				if len(matches) == 0 {
					matches = filterPrefix(f.projCandidates, "")
				}
			}
		}
		if len(matches) > 0 {
			f.matchList.SetItems(itemsFromStrings(matches))
			h := min(10, len(matches))
			f.matchList.SetSize(max(20, f.width/3), max(3, h))
			f.listOpen = true
		} else {
			f.listOpen = false
		}
		return f, nil

	case tea.KeyMsg:
		// If the match list is open, route keys to it first so the user can
		// navigate/select matches with ↑/↓ and Enter; Esc closes the list.
		if f.listOpen {
			switch msg.String() {
			case "enter":
				// Accept selected item into the focused input.
				if it := f.matchList.SelectedItem(); it != nil {
					if li, ok := it.(listItem); ok {
						val := li.Title()
						if f.focused == 0 {
							f.customerInput.SetValue(val)
						} else if f.focused == 1 {
							f.projectInput.SetValue(val)
						}
					}
				}
				f.listOpen = false
				return f, nil
			case "esc":
				// Close the list without changing the input.
				f.listOpen = false
				return f, nil
			default:
				// Let the list handle navigation keys.
				var cmd tea.Cmd
				f.matchList, cmd = f.matchList.Update(msg)
				return f, cmd
			}
		}

		switch msg.String() {
		case "tab", "shift+tab", "enter", "up", "k", "down", "j", "ctrl+space":
			// Handle navigation/focus cycling and open visible completion list on ctrl+space.
			s := msg.String()

			// If ctrl+space triggered completion for the focused field, handle it by
			// building a ranked candidate list and opening the visible dropdown.
			if s == "ctrl+space" {
				// If focused on customer
				if f.focused == 0 {
					prefix := strings.ToLower(strings.TrimSpace(f.customerInput.Value()))
					matches := filterPrefix(f.custCandidates, prefix)
					// If no prefix matches, try contains for short prefixes (1-2 chars),
					// then fuzzy matches as a fallback, and finally top candidates.
					if len(matches) == 0 {
						if prefix != "" && len(prefix) <= 2 {
							matches = filterContains(f.custCandidates, prefix)
						}
					}
					if len(matches) == 0 && prefix != "" {
						matches = fuzzyMatches(f.custCandidates, prefix)
					}
					if len(matches) == 0 {
						// fallback to showing top candidates
						matches = filterPrefix(f.custCandidates, "")
					}
					if len(matches) > 0 {
						f.matchList.SetItems(itemsFromStrings(matches))
						// size: width ~ 1/3 of screen, height = min(10,len(matches))
						h := min(10, len(matches))
						f.matchList.SetSize(max(20, f.width/3), max(3, h))
						f.listOpen = true
					}
					return f, nil
				}
				// If focused on project
				if f.focused == 1 {
					prefix := strings.ToLower(strings.TrimSpace(f.projectInput.Value()))
					matches := filterPrefix(f.projCandidates, prefix)
					if len(matches) == 0 {
						if prefix != "" && len(prefix) <= 2 {
							matches = filterContains(f.projCandidates, prefix)
						}
					}
					if len(matches) == 0 && prefix != "" {
						matches = fuzzyMatches(f.projCandidates, prefix)
					}
					if len(matches) == 0 {
						matches = filterPrefix(f.projCandidates, "")
					}
					if len(matches) > 0 {
						f.matchList.SetItems(itemsFromStrings(matches))
						h := min(10, len(matches))
						f.matchList.SetSize(max(20, f.width/3), max(3, h))
						f.listOpen = true
					}
					return f, nil
				}
			}

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
		// Pass typing to the focused textinput, and schedule debounced suggestions.
		if f.focused >= 0 && f.focused < len(f.inputs) {
			ti := f.inputs[f.focused]
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)

			// Schedule a debounced suggestion update for customer/project fields.
			if f.focused == 0 || f.focused == 1 {
				f.debounceID++
				id := f.debounceID
				term := ti.Value()
				focused := f.focused
				debounceCmd := func() tea.Msg {
					time.Sleep(120 * time.Millisecond)
					return suggestMsg{id: id, focused: focused, term: term}
				}
				return f, tea.Batch(cmd, debounceCmd)
			}

			return f, cmd
		}
		return f, nil

	default:
		// Let the focused input react to non-key messages if needed.
		if f.focused >= 0 && f.focused < len(f.inputs) {
			ti := f.inputs[f.focused]
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)
			// schedule debounced suggestion based on the updated input value
			if f.focused == 0 || f.focused == 1 {
				f.debounceID++
				id := f.debounceID
				term := ti.Value()
				focused := f.focused
				debounceCmd := func() tea.Msg {
					time.Sleep(120 * time.Millisecond)
					return suggestMsg{id: id, focused: focused, term: term}
				}
				return f, tea.Batch(cmd, debounceCmd)
			}
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
	// Suggestions status: show counts to help debug visibility of dropdown/matches.
	// When focused on customer (0) or project (1), show matches vs total candidates and whether the dropdown is open.
	var suggStatus string
	if f.focused == 0 {
		total := len(f.custCandidates)
		matches := len(f.matchList.Items())
		if f.listOpen {
			suggStatus = fmt.Sprintf("Suggestions: %d matches (open) / %d total customers", matches, total)
		} else {
			suggStatus = fmt.Sprintf("Suggestions: %d matches / %d total customers", matches, total)
		}
	} else if f.focused == 1 {
		total := len(f.projCandidates)
		matches := len(f.matchList.Items())
		if f.listOpen {
			suggStatus = fmt.Sprintf("Suggestions: %d matches (open) / %d total projects", matches, total)
		} else {
			suggStatus = fmt.Sprintf("Suggestions: %d matches / %d total projects", matches, total)
		}
	} else {
		suggStatus = fmt.Sprintf("Candidates: customers=%d projects=%d", len(f.custCandidates), len(f.projCandidates))
	}
	b.WriteString(MutedStyle.Render(suggStatus))
	b.WriteString("\n\n")

	// Helper to render a labeled input line
	renderLine := func(label string, ti *textinput.Model) {
		labelR := EmphStyle.Render(label)
		// Align with some spacing; rely on SectionBoxStyle to provide padding.
		b.WriteString(fmt.Sprintf("%-10s %s\n", labelR, ti.View()))
	}

	renderLine("Customer", f.customerInput)
	// If the list is open and customer is focused, show the dropdown under the input.
	if f.listOpen && f.focused == 0 {
		b.WriteString("\n")
		// Provide safe fallback: if list is empty show a muted note.
		listView := f.matchList.View()
		if listView == "" {
			b.WriteString(MutedStyle.Render("(no suggestions)\n"))
		} else {
			b.WriteString(listView)
			b.WriteString("\n")
		}
	}
	renderLine("Project", f.projectInput)
	if f.listOpen && f.focused == 1 {
		b.WriteString("\n")
		listView := f.matchList.View()
		if listView == "" {
			b.WriteString(MutedStyle.Render("(no suggestions)\n"))
		} else {
			b.WriteString(listView)
			b.WriteString("\n")
		}
	}
	renderLine("Activity", f.activityInput)
	renderLine("Tags", f.tagsInput)
	renderLine("Note", f.noteInput)

	// Billable hint & instructions
	b.WriteString("\n")
	b.WriteString(MutedStyle.Render(fmt.Sprintf("Billable: %v  (toggle with 'b' in dashboard form mode)", f.defaultBill)))
	b.WriteString("\n\n")
	b.WriteString(MutedStyle.Render("Tab: next field • Shift+Tab: prev • Enter on Note: submit • Esc: cancel • Ctrl+Space: complete"))

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

// Helper: load unique candidates from journal files
func (f *formModel) loadCandidatesFromJournal() {
	// Load unique customers and projects from ~/.tt/journal (non-strict, best-effort).
	root := DefaultJournalRoot()
	custSet := map[string]struct{}{}
	projSet := map[string]struct{}{}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		fh, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer fh.Close()
		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var ev map[string]interface{}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if v, ok := ev["customer"].(string); ok {
				if s := strings.TrimSpace(v); s != "" {
					custSet[s] = struct{}{}
				}
			}
			if v, ok := ev["project"].(string); ok {
				if s := strings.TrimSpace(v); s != "" {
					projSet[s] = struct{}{}
				}
			}
		}
		return nil
	})

	custs := make([]string, 0, len(custSet))
	for k := range custSet {
		custs = append(custs, k)
	}
	sort.Strings(custs)

	projs := make([]string, 0, len(projSet))
	for k := range projSet {
		projs = append(projs, k)
	}
	sort.Strings(projs)

	f.custCandidates = custs
	f.projCandidates = projs
}

func filterPrefix(list []string, prefix string) []string {
	if prefix == "" {
		// return top candidates (limit to 20)
		if len(list) > 20 {
			return list[:20]
		}
		return list
	}
	lower := strings.ToLower(prefix)
	out := make([]string, 0)
	for _, it := range list {
		if strings.HasPrefix(strings.ToLower(it), lower) {
			out = append(out, it)
		}
	}
	return out
}

func filterContains(list []string, term string) []string {
	// If no search term, return top candidates (limit to 20)
	if strings.TrimSpace(term) == "" {
		if len(list) > 20 {
			return list[:20]
		}
		return list
	}
	lower := strings.ToLower(term)
	out := make([]string, 0)
	for _, it := range list {
		if strings.Contains(strings.ToLower(it), lower) {
			out = append(out, it)
		}
	}
	return out
}

func fuzzyMatches(list []string, term string) []string {
	term = strings.TrimSpace(term)
	if term == "" || len(list) == 0 {
		return nil
	}
	// fuzzy.Find returns matches in order of relevance
	matches := fuzzy.Find(term, list)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// m.Str is the candidate string
		out = append(out, m.Str)
	}
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

// listItem wraps a string as a list.Item for bubbles/list
type listItem struct {
	s string
}

func (l listItem) Title() string       { return l.s }
func (l listItem) Description() string { return "" }
func (l listItem) FilterValue() string { return l.s }

func itemsFromStrings(in []string) []list.Item {
	out := make([]list.Item, 0, len(in))
	for _, s := range in {
		out = append(out, listItem{s: s})
	}
	return out
}

type formCancelledMsg struct{}

// Methods to let callers configure/run the form submodel.
func (f *formModel) SetMode(m string)          { f.mode = m }
func (f *formModel) SetDefaultBillable(b bool) { f.defaultBill = b }
func (f *formModel) SetWriter(w EventWriter)   { f.writer = w }

// small helpers
// min moved to internal/tui/style.go; use min(...) from there.
// This placeholder indicates the helper was intentionally removed from this file.

// Duplicate max helper removed from this file.
// Use the shared max() helper defined in internal/tui/style.go instead.
