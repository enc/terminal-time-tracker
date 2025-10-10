package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Services aggregates the dependencies the TUI needs. Provide your concrete
// implementations to wire the app to the real journal and event writer.
type Services struct {
	Journal JournalService
	Writer  EventWriter
	Watch   JournalWatch
	Config  ConfigService
}

// JournalService loads entries from the append-only JSONL journal and can
// compute the current active entry and the most recent closed entry.
type JournalService interface {
	LoadEntries(ctx context.Context, from, to time.Time) ([]Entry, error)
	FindActiveAndLast(ctx context.Context, from, to time.Time) (*Entry, *Entry, error)
}

// EventWriter writes journal events (start/stop/note/add/switch). The TUI
// should share the same implementation as the CLI to ensure identical hashes
// and file formats.
type EventWriter interface {
	Start(ctx context.Context, p StartParams) error
	Stop(ctx context.Context) error
	Note(ctx context.Context, text string) error
	Add(ctx context.Context, p AddParams) error
	Switch(ctx context.Context, p SwitchParams) error
}

// JournalWatch emits a signal whenever journal files change on disk
// (e.g., via fsnotify). The TUI listens and refreshes the dashboard.
type JournalWatch interface {
	Changes(ctx context.Context) <-chan struct{}
}

// ConfigService provides runtime configuration like timezone and rounding.
type ConfigService interface {
	Timezone() *time.Location
	Rounding() RoundingConfig
}

// RoundingConfig mirrors the CLI's rounding configuration.
type RoundingConfig struct {
	Strategy     string // up|down|nearest
	QuantumMin   int
	MinimumEntry int
}

// Domain types used by the dashboard and views.

type Entry struct {
	ID       string
	Start    time.Time
	End      *time.Time
	Customer string
	Project  string
	Activity string
	Billable bool
	Notes    []string
	Tags     []string
}

type StartParams struct {
	Customer string
	Project  string
	Activity string
	Billable bool
	Tags     []string
	Note     string
}

type AddParams struct {
	Start    time.Time
	End      time.Time
	Customer string
	Project  string
	Activity string
	Billable bool
	Tags     []string
	Note     string
}

type SwitchParams struct {
	Customer string
	Project  string
	Activity string
	Billable bool
	Tags     []string
	Note     string
}

// NewAppModel constructs a minimal Bubble Tea app model that shows
// a dashboard with a ticking clock and (if available) the active and
// last entries loaded from the journal service.
func NewAppModel(svcs Services) tea.Model {
	return appModel{
		services:  svcs,
		now:       time.Now(),
		dashboard: newDashboardModel(svcs),
	}
}

type appModel struct {
	services Services

	width  int
	height int
	now    time.Time
	err    error

	dashboard dashboardModel
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tickEvery(time.Second),
		m.dashboard.Init(),
		listenJournal(m.services.Watch),
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.dashboard.setSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		// If a subview is capturing text (e.g., note input or start/switch form),
		// let it handle keys first to avoid global shortcut interference.
		if m.dashboard.isCapturing() {
			if msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.Update(msg)
			return m, cmd
		}

	case tickMsg:
		m.now = time.Time(msg)
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, tea.Batch(cmd, tickEvery(time.Second))

	case fsChangeMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	default:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	}
}

func (m appModel) View() string {
	// Header with current time on the right.
	header := RenderHeader("tt — Dashboard", m.now.Format("2006-01-02 15:04:05"), m.width)

	body := m.dashboard.View()

	// Footer with context-aware hints; status (if any) on the right.
	hints, right := m.dashboard.hints(), ""
	footer := RenderFooter(hints, right, m.width)

	return header + "\n" + body + "\n" + footer
}

// ---------- Dashboard (note input + start/switch form + suggestions) ----------

type suggestion struct {
	Customer string
	Project  string
	Activity string
	Billable bool
}

func (s suggestion) label() string {
	c := emptyDash(s.Customer)
	p := emptyDash(s.Project)
	a := emptyDash(s.Activity)
	b := "billable=true"
	if !s.Billable {
		b = "billable=false"
	}
	return fmt.Sprintf("%s/%s [%s]  %s", c, p, a, b)
}

type dashboardModel struct {
	svcs Services

	width  int
	height int

	active *Entry
	last   *Entry
	err    error
	loaded bool

	// Note input mode state
	noteMode bool
	noteBuf  string

	// Start/Switch form state
	formMode bool
	form     *formModel

	// Timeline view state (toggleable via key)
	showTimelines     bool
	timelineWeekStart time.Time
	timelineEntries   []Entry
	timelineLoaded    bool
	timelineErr       error

	status string // simple transient status line (e.g., errors)
}

func newDashboardModel(svcs Services) dashboardModel {
	return dashboardModel{svcs: svcs}
}

func (d dashboardModel) Init() tea.Cmd {
	return loadStatus(d.svcs.Journal)
}

func (d *dashboardModel) setSize(w, h int) {
	d.width, d.height = w, h
}

// isCapturing indicates the dashboard is currently capturing text input
// or in selection form so global shortcuts (like q/esc/space) should not interfere.
func (d dashboardModel) isCapturing() bool {
	return d.noteMode || d.formMode
}

func (d dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		d.active = msg.active
		d.last = msg.last
		d.err = msg.err
		d.loaded = true
		return d, nil

	case fsChangeMsg:
		// Reload on external changes.
		return d, loadStatus(d.svcs.Journal)

	case tickMsg:
		// Re-render for elapsed time updates.
		return d, nil

	case startDoneMsg:
		if msg.err != nil {
			d.status = RenderStatus("err", "Failed to start: "+msg.err.Error())
		} else {
			d.status = RenderStatus("ok", "Started")
		}
		// If a form was active, close it after successful start/switch.
		d.formMode = false
		d.form = nil
		return d, nil

	case stopDoneMsg:
		if msg.err != nil {
			d.status = RenderStatus("err", "Failed to stop: "+msg.err.Error())
		} else {
			d.status = RenderStatus("ok", "Stopped")
		}
		return d, nil

	case noteSavedMsg:
		if msg.err != nil {
			d.status = RenderStatus("err", "Failed to save note: "+msg.err.Error())
		} else {
			d.status = RenderStatus("ok", "Note saved")
		}
		return d, nil

	case weekLoadedMsg:
		// Weekly entries loaded for the timelines view.
		d.timelineEntries = msg.entries
		d.timelineErr = msg.err
		d.timelineLoaded = true
		return d, nil

	case tea.KeyMsg:
		// Note input mode: scoped handling
		if d.noteMode {
			switch msg.Type {
			case tea.KeyEnter:
				text := d.noteBuf
				d.noteMode = false
				d.noteBuf = ""
				return d, tea.Batch(saveNote(d.svcs.Writer, text), loadStatus(d.svcs.Journal))
			case tea.KeyEsc:
				d.noteMode = false
				d.noteBuf = ""
				return d, nil
			case tea.KeyBackspace, tea.KeyCtrlH:
				d.noteBuf = backspace(d.noteBuf)
				return d, nil
			default:
				if len(msg.Runes) > 0 {
					d.noteBuf += string(msg.Runes)
					return d, nil
				}
				if msg.String() == "space" {
					d.noteBuf += " "
					return d, nil
				}
				return d, nil
			}
		}

		// Start/Switch form mode: delegate to the editable form submodel when active.
		if d.formMode && d.form != nil {
			// Let the form handle navigation/typing; it will emit messages (e.g., formCancelledMsg or startDoneMsg)
			var cmd tea.Cmd
			var m tea.Model
			m, cmd = d.form.Update(msg)
			// Update pointer to new typed-back model (type assert)
			if fm, ok := m.(*formModel); ok {
				d.form = fm
			}
			return d, cmd
		}

		// Normal view mode: action shortcuts
		switch msg.String() {
		case " ":
			// Toggle start/stop
			if d.active != nil && d.active.End == nil {
				return d, tea.Batch(stopEntry(d.svcs.Writer), loadStatus(d.svcs.Journal))
			}
			return d, tea.Batch(startEntry(d.svcs.Writer, d.last), loadStatus(d.svcs.Journal))
		case "n":
			// Enter note input mode
			d.noteMode = true
			d.noteBuf = ""
			return d, nil
		case "s":
			// Open start/switch form with quick suggestions
			d.openForm()
			return d, nil
		case "t":
			// Toggle timelines view. When enabling, load entries for the week starting on Monday.
			if !d.showTimelines {
				// pick timezone and compute weekStart at Monday midnight
				tz := d.svcs.Config.Timezone()
				if tz == nil {
					tz = time.Local
				}
				today := time.Now().In(tz)
				weekStart := startOfWeekMonday(today)
				d.timelineWeekStart = weekStart
				d.timelineLoaded = false
				d.timelineErr = nil
				d.showTimelines = true
				return d, loadWeekEntries(d.svcs.Journal, weekStart)
			}
			// hide timelines
			d.showTimelines = false
			return d, nil
		case "h", "<":
			// When timelines are visible, move to previous week.
			if d.showTimelines {
				d.timelineWeekStart = d.timelineWeekStart.AddDate(0, 0, -7)
				d.timelineLoaded = false
				d.timelineErr = nil
				return d, loadWeekEntries(d.svcs.Journal, d.timelineWeekStart)
			}
			return d, nil
		case "l", ">":
			// When timelines are visible, move to next week (but prevent moving into future weeks).
			if d.showTimelines {
				tz := d.svcs.Config.Timezone()
				if tz == nil {
					tz = time.Local
				}
				// Candidate week is the next week's Monday.
				candidate := d.timelineWeekStart.AddDate(0, 0, 7)
				// Do not allow navigating to a week that starts after the current week (Monday of now).
				currentWeekStart := startOfWeekMonday(time.Now().In(tz))
				if candidate.After(currentWeekStart) {
					// Avoid navigation into the future. Provide a brief status hint.
					d.status = RenderStatus("warn", "Cannot navigate into future weeks")
					return d, nil
				}
				d.timelineWeekStart = candidate
				d.timelineLoaded = false
				d.timelineErr = nil
				return d, loadWeekEntries(d.svcs.Journal, d.timelineWeekStart)
			}
			return d, nil
		default:
			return d, nil
		}

	default:
		return d, nil
	}
}

func (d dashboardModel) View() string {
	if !d.loaded {
		return SectionBoxStyle.Render("Loading…")
	}
	if d.err != nil {
		return SectionBoxStyle.Render(fmt.Sprintf("Error: %v", d.err))
	}

	// Active section content
	activeLines := ""
	if d.active != nil {
		elapsed := time.Since(d.active.Start).Truncate(time.Second)
		endStr := "(running)"
		if d.active.End != nil {
			endStr = d.active.End.Format("15:04:05")
			elapsed = d.active.End.Sub(d.active.Start)
		}
		kv := [][2]string{
			{"When", fmt.Sprintf("%s → %s", d.active.Start.Format("15:04:05"), endStr)},
			{"What", fmt.Sprintf("%s/%s [%s]", emptyDash(d.active.Customer), emptyDash(d.active.Project), emptyDash(d.active.Activity))},
			{"Billable", fmt.Sprintf("%v", d.active.Billable)},
			{"Elapsed", fmtHHMMSS(int(elapsed.Seconds()))},
		}
		activeLines = RenderKeyValueList(kv, max(20, d.width-6))
	} else {
		activeLines = MutedStyle.Render("No active session.")
	}

	// Last section content
	lastLines := ""
	if d.last != nil {
		endStr := "(running)"
		if d.last.End != nil {
			endStr = d.last.End.Format("15:04:05")
		}
		kv := [][2]string{
			{"When", fmt.Sprintf("%s → %s", d.last.Start.Format("15:04:05"), endStr)},
			{"What", fmt.Sprintf("%s/%s [%s]", emptyDash(d.last.Customer), emptyDash(d.last.Project), emptyDash(d.last.Activity))},
			{"Billable", fmt.Sprintf("%v", d.last.Billable)},
			{"Duration", fmtHHMMSS(durationSeconds(*d.last))},
		}
		lastLines = RenderKeyValueList(kv, max(20, d.width-6))
	} else {
		lastLines = MutedStyle.Render("No previous entry in the recent window.")
	}

	// Compose sections
	activeSec := RenderSection("Active", activeLines, d.width)
	lastSec := RenderSection("Last", lastLines, d.width)

	// Note input overlay or form overlay
	extra := ""
	if d.noteMode {
		n := fmt.Sprintf("Note: %s\n(Enter to save, Esc to cancel)", d.noteBuf)
		extra = RenderSection("Add Note", n, d.width)
	} else if d.formMode && d.form != nil {
		// Render the editable form provided by the form submodel.
		extra = d.form.View()
	} else {
		// Quick suggestions preview (top 3) when idle
		preview := d.previewSuggestions()
		if preview != "" {
			extra = RenderSection("Suggestions (press s)", preview, d.width)
		}
	}

	statusLine := ""
	if d.status != "" {
		statusLine = "\n" + d.status
	}

	// If the timelines view is toggled on, render it instead of the quick suggestions.
	if d.showTimelines {
		body := ""
		if !d.timelineLoaded {
			body = SectionBoxStyle.Render("Loading timelines…")
		} else if d.timelineErr != nil {
			body = SectionBoxStyle.Render(fmt.Sprintf("Error loading timelines: %v", d.timelineErr))
		} else {
			tz := d.svcs.Config.Timezone()
			if tz == nil {
				tz = time.Local
			}
			// Compute week range for the header: Monday → Sunday
			weekStart := d.timelineWeekStart.In(tz)
			weekEnd := d.timelineWeekStart.AddDate(0, 0, 6).In(tz)
			weekRange := fmt.Sprintf("%s → %s", weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02"))
			// Render a compact week-range header above the timeline section.
			rangeHeader := SectionTitleStyle.Render("Week: "+weekRange) + "\n"
			body = rangeHeader + RenderWeekTimeline(d.timelineEntries, d.timelineWeekStart, tz, d.width)
		}
		return activeSec + "\n" + lastSec + "\n" + body + statusLine
	}

	return activeSec + "\n" + lastSec + "\n" + extra + statusLine
}

// hints returns footer hints depending on current mode.
func (d dashboardModel) hints() []Hint {
	if d.noteMode {
		return []Hint{
			{Key: "Enter", Text: "save note"},
			{Key: "Esc", Text: "cancel"},
		}
	}
	if d.formMode {
		return []Hint{
			{Key: "Tab", Text: "next field"},
			{Key: "Shift+Tab", Text: "prev field"},
			{Key: "Enter", Text: "submit"},
			{Key: "Esc", Text: "cancel"},
		}
	}
	if d.showTimelines {
		// When timelines are visible, expose navigation keys.
		return []Hint{
			{Key: "space", Text: "start/stop"},
			{Key: "n", Text: "note"},
			{Key: "s", Text: "start/switch"},
			{Key: "t", Text: "timelines"},
			{Key: "h / <", Text: "prev week"},
			{Key: "l / >", Text: "next week"},
			{Key: "q", Text: "quit"},
		}
	}
	return []Hint{
		{Key: "space", Text: "start/stop"},
		{Key: "n", Text: "note"},
		{Key: "s", Text: "start/switch"},
		{Key: "t", Text: "timelines"},
		{Key: "q", Text: "quit"},
	}
}

// ---------- Commands / messages ----------

type tickMsg time.Time
type fsChangeMsg struct{}
type statusLoadedMsg struct {
	active *Entry
	last   *Entry
	err    error
}

type startDoneMsg struct{ err error }
type stopDoneMsg struct{ err error }
type noteSavedMsg struct{ err error }

// weekLoadedMsg is emitted when weekly entries have been loaded for timelines.
type weekLoadedMsg struct {
	entries []Entry
	err     error
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func listenJournal(w JournalWatch) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for range w.Changes(ctx) {
			// Return on first change; Bubble Tea will re-run this cmd if queued again.
			return fsChangeMsg{}
		}
		return nil
	}
}

func loadStatus(j JournalService) tea.Cmd {
	return func() tea.Msg {
		if j == nil {
			// No service provided; return empty state
			return statusLoadedMsg{}
		}
		now := time.Now()
		from := now.AddDate(0, 0, -7)
		active, last, err := j.FindActiveAndLast(context.Background(), from, now)
		return statusLoadedMsg{active: active, last: last, err: err}
	}
}

// loadWeekEntries loads entries for the 7-day window starting at weekStart.
func loadWeekEntries(j JournalService, weekStart time.Time) tea.Cmd {
	return func() tea.Msg {
		if j == nil {
			return weekLoadedMsg{entries: nil, err: nil}
		}
		from := weekStart
		to := from.AddDate(0, 0, 7)
		ents, err := j.LoadEntries(context.Background(), from, to)
		return weekLoadedMsg{entries: ents, err: err}
	}
}

func startEntry(w EventWriter, last *Entry) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return startDoneMsg{}
		}
		p := StartParams{Billable: true}
		if last != nil {
			p.Customer = last.Customer
			p.Project = last.Project
			p.Activity = last.Activity
			p.Billable = last.Billable
		}
		if err := w.Start(context.Background(), p); err != nil {
			return startDoneMsg{err: err}
		}
		return startDoneMsg{}
	}
}

func stopEntry(w EventWriter) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return stopDoneMsg{}
		}
		if err := w.Stop(context.Background()); err != nil {
			return stopDoneMsg{err: err}
		}
		return stopDoneMsg{}
	}
}

func saveNote(w EventWriter, text string) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return noteSavedMsg{}
		}
		if text == "" {
			return noteSavedMsg{}
		}
		if err := w.Note(context.Background(), text); err != nil {
			return noteSavedMsg{err: err}
		}
		return noteSavedMsg{}
	}
}

// ---------- Suggestions and Form helpers ----------

func (d *dashboardModel) openForm() {
	// Build suggestions and instantiate an editable form submodel.
	sugs := d.buildSuggestions()

	// Determine a sensible default for billable based on active/last entries.
	defBill := true
	if d.active != nil {
		defBill = d.active.Billable
	} else if d.last != nil {
		defBill = d.last.Billable
	}

	// Create the editable form and seed it with the last entry values.
	f := NewStartSwitchForm(d.svcs.Writer, d.last, defBill, sugs)
	// If a session is currently running, treat the form as a "switch".
	if d.active != nil && d.active.End == nil {
		f.SetMode("switch")
	} else {
		f.SetMode("start")
	}
	d.form = f
	d.formMode = true
}

func (d dashboardModel) buildSuggestions() []suggestion {
	if d.svcs.Journal == nil {
		// Use last + active as best-effort suggestions if service is missing.
		var out []suggestion
		if d.active != nil {
			out = append(out, suggestion{
				Customer: d.active.Customer,
				Project:  d.active.Project,
				Activity: d.active.Activity,
				Billable: d.active.Billable,
			})
		}
		if d.last != nil {
			out = append(out, suggestion{
				Customer: d.last.Customer,
				Project:  d.last.Project,
				Activity: d.last.Activity,
				Billable: d.last.Billable,
			})
		}
		return out
	}

	// Look back a reasonable window and build frequency + recency stats of combos.
	now := time.Now()
	from := now.AddDate(0, 0, -30)

	ents, err := d.svcs.Journal.LoadEntries(context.Background(), from, now)
	if err != nil || len(ents) == 0 {
		// Fall back to last entry if available.
		if d.last != nil {
			return []suggestion{{
				Customer: d.last.Customer,
				Project:  d.last.Project,
				Activity: d.last.Activity,
				Billable: d.last.Billable,
			}}
		}
		return nil
	}

	type stat struct {
		s        suggestion
		count    int
		lastSeen time.Time
	}

	stats := map[string]*stat{}
	keyOf := func(c, p, a string) string {
		return strings.ToLower(strings.TrimSpace(c)) + "||" +
			strings.ToLower(strings.TrimSpace(p)) + "||" +
			strings.ToLower(strings.TrimSpace(a))
	}

	// Seed with last and active to bias sorting later.
	seed := func(e *Entry) {
		if e == nil {
			return
		}
		k := keyOf(e.Customer, e.Project, e.Activity)
		if _, ok := stats[k]; !ok {
			stats[k] = &stat{
				s: suggestion{
					Customer: e.Customer,
					Project:  e.Project,
					Activity: e.Activity,
					Billable: e.Billable,
				},
				count:    0,
				lastSeen: e.Start,
			}
		} else {
			if e.Start.After(stats[k].lastSeen) {
				stats[k].lastSeen = e.Start
			}
		}
	}
	seed(d.active)
	seed(d.last)

	for _, e := range ents {
		k := keyOf(e.Customer, e.Project, e.Activity)
		if _, ok := stats[k]; !ok {
			stats[k] = &stat{
				s: suggestion{
					Customer: e.Customer,
					Project:  e.Project,
					Activity: e.Activity,
					Billable: e.Billable,
				},
				count:    1,
				lastSeen: e.Start,
			}
		} else {
			stats[k].count++
			if e.Start.After(stats[k].lastSeen) {
				stats[k].lastSeen = e.Start
			}
		}
	}

	// Convert to slice and sort by frequency then recency.
	list := make([]stat, 0, len(stats))
	for _, v := range stats {
		list = append(list, *v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].lastSeen.After(list[j].lastSeen)
	})

	out := make([]suggestion, 0, len(list))
	for _, it := range list {
		out = append(out, it.s)
	}
	return out
}

func (d dashboardModel) previewSuggestions() string {
	list := d.buildSuggestions()
	if len(list) == 0 {
		return ""
	}
	n := len(list)
	if n > 3 {
		n = 3
	}
	lines := make([]string, n)
	for i := 0; i < n; i++ {
		lines[i] = "• " + list[i].label()
	}
	return strings.Join(lines, "\n")
}

func (d dashboardModel) selectedSuggestion() suggestion {
	list := d.buildSuggestions()
	if len(list) == 0 {
		defBill := true
		if d.active != nil {
			defBill = d.active.Billable
		} else if d.last != nil {
			defBill = d.last.Billable
		}
		return suggestion{Billable: defBill}
	}
	// Prefer the first suggestion when a selection index is not maintained here.
	sel := list[0]
	// Reflect a sensible billable default
	if d.active != nil {
		sel.Billable = d.active.Billable
	} else if d.last != nil {
		sel.Billable = d.last.Billable
	}
	return sel
}

func startWith(w EventWriter, s suggestion) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return startDoneMsg{}
		}
		p := StartParams{
			Customer: s.Customer,
			Project:  s.Project,
			Activity: s.Activity,
			Billable: s.Billable,
		}
		if err := w.Start(context.Background(), p); err != nil {
			return startDoneMsg{err: err}
		}
		return startDoneMsg{}
	}
}

func switchWith(w EventWriter, s suggestion) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return startDoneMsg{}
		}
		p := SwitchParams{
			Customer: s.Customer,
			Project:  s.Project,
			Activity: s.Activity,
			Billable: s.Billable,
		}
		if err := w.Switch(context.Background(), p); err != nil {
			return startDoneMsg{err: err}
		}
		return startDoneMsg{}
	}
}

// ---------- Helpers ----------

func durationSeconds(e Entry) int {
	if e.End == nil {
		return 0
	}
	return int(e.End.Sub(e.Start).Seconds())
}

func fmtHHMMSS(totalSec int) string {
	if totalSec < 0 {
		totalSec = 0
	}
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func backspace(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// startOfWeekMonday returns the time at midnight on Monday for the week that
// contains t (in t's location). If t is already on Monday at midnight, that
// exact moment is returned.
func startOfWeekMonday(t time.Time) time.Time {
	loc := t.Location()
	// normalize to midnight of the given day
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	wd := day.Weekday()
	// Calculate how many days to subtract to get to Monday.
	// In Go, time.Monday == 1, time.Sunday == 0.
	var daysBack int
	if wd == time.Sunday {
		// Sunday -> go back 6 days to previous Monday
		daysBack = 6
	} else {
		// e.g., Tuesday (2) -> back 1 to reach Monday (1)
		daysBack = int(wd) - int(time.Monday)
	}
	return day.AddDate(0, 0, -daysBack)
}
