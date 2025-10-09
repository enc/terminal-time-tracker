package tui

import (
	"context"
	"fmt"
	"time"

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
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		default:
			// Route to dashboard (future: view router)
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.Update(msg)
			return m, cmd
		}

	case tickMsg:
		m.now = time.Time(msg)
		// Let dashboard react to time ticks (e.g., update elapsed)
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, tea.Batch(cmd, tickEvery(time.Second))

	case fsChangeMsg:
		// Journal changed on disk; ask dashboard to refresh.
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
	header := "tt — Dashboard (q: quit)"
	body := m.dashboard.View()
	footer := fmt.Sprintf("Now: %s", m.now.Format("2006-01-02 15:04:05"))
	return header + "\n\n" + body + "\n\n" + footer
}

// ---------- Dashboard (minimal) ----------

type dashboardModel struct {
	svcs Services

	width  int
	height int

	active *Entry
	last   *Entry
	err    error
	loaded bool
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
		// No-op beyond re-rendering (elapsed time for active entry is derived in View()).
		return d, nil

	case tea.KeyMsg:
		// Future: bind keys for start/stop/switch/note
		return d, nil

	default:
		return d, nil
	}
}

func (d dashboardModel) View() string {
	if !d.loaded {
		return "Loading…"
	}
	if d.err != nil {
		return fmt.Sprintf("Error: %v", d.err)
	}

	// Active section
	activeLine := "No active session."
	if d.active != nil {
		elapsed := time.Since(d.active.Start).Truncate(time.Second)
		endStr := "(running)"
		if d.active.End != nil {
			endStr = d.active.End.Format("15:04:05")
			elapsed = d.active.End.Sub(d.active.Start)
		}
		activeLine = fmt.Sprintf(
			"Active: %s/%s [%s] billable=%v  %s → %s  (%s)",
			emptyDash(d.active.Customer),
			emptyDash(d.active.Project),
			emptyDash(d.active.Activity),
			d.active.Billable,
			d.active.Start.Format("15:04:05"),
			endStr,
			fmtHHMMSS(int(elapsed.Seconds())),
		)
	}

	// Last section
	lastLine := "No previous entry in the recent window."
	if d.last != nil {
		endStr := "(running)"
		if d.last.End != nil {
			endStr = d.last.End.Format("15:04:05")
		}
		lastLine = fmt.Sprintf(
			"Last:   %s/%s [%s] billable=%v  %s → %s  (%s)",
			emptyDash(d.last.Customer),
			emptyDash(d.last.Project),
			emptyDash(d.last.Activity),
			d.last.Billable,
			d.last.Start.Format("15:04:05"),
			endStr,
			fmtHHMMSS(durationSeconds(*d.last)),
		)
	}

	return activeLine + "\n" + lastLine
}

// ---------- Commands / messages ----------

type tickMsg time.Time
type fsChangeMsg struct{}
type statusLoadedMsg struct {
	active *Entry
	last   *Entry
	err    error
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
