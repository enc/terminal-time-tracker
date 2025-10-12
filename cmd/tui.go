package cmd

import (
	"context"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ui "tt/internal/tui"
)

// tuiCmd provides an interactive TUI for tt using Bubble Tea.
// This is a minimal scaffold that enters an alt-screen UI and
// renders a basic header/footer and a ticking status line.
//
// Next steps (future commits):
// - Show live active session status (customer/project/activity, elapsed)
// - Watch ~/.tt/journal for changes and refresh the view (fsnotify)
// - Add a timeline and command palette
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal UI (space: start/stop, n: note, q/Esc: quit)",
	Long:  "Launch the Bubble Tea TUI for tt. Dashboard with live status; auto-refresh on journal changes. Keys: space=start/stop, n=note, q/Esc=quit.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Step 1: Wire the internal TUI app model with stubbed services.
		svcs := ui.Services{
			Journal: stubJournal{},
			Writer:  stubWriter{},
			Watch:   ui.NewFSNotifyJournalWatch("", 0),
			Config:  stubConfig{},
		}
		m := ui.NewAppModel(svcs)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			log.Printf("tui exited with error: %v", err)
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

// -------- Stub services to back the internal TUI app model (Step 1) --------

// stubJournal adapts existing CLI reconstruction to the TUI's JournalService.
type stubJournal struct{}

func (stubJournal) LoadEntries(ctx context.Context, from, to time.Time) ([]ui.Entry, error) {
	ents, err := loadEntries(from, to)
	if err != nil {
		return nil, err
	}
	out := make([]ui.Entry, 0, len(ents))
	for _, e := range ents {
		out = append(out, ui.Entry{
			ID:       e.ID,
			Start:    e.Start,
			End:      e.End,
			Customer: e.Customer,
			Project:  e.Project,
			Activity: e.Activity,
			Billable: e.Billable,
			Notes:    e.Notes,
			Tags:     e.Tags,
		})
	}
	return out, nil
}

func (stubJournal) FindActiveAndLast(ctx context.Context, from, to time.Time) (*ui.Entry, *ui.Entry, error) {
	a, l, err := findActiveAndLast(from, to)
	if err != nil {
		return nil, nil, err
	}
	var au *ui.Entry
	var lu *ui.Entry
	if a != nil {
		x := ui.Entry{
			ID:       a.ID,
			Start:    a.Start,
			End:      a.End,
			Customer: a.Customer,
			Project:  a.Project,
			Activity: a.Activity,
			Billable: a.Billable,
			Notes:    a.Notes,
			Tags:     a.Tags,
		}
		au = &x
	}
	if l != nil {
		x := ui.Entry{
			ID:       l.ID,
			Start:    l.Start,
			End:      l.End,
			Customer: l.Customer,
			Project:  l.Project,
			Activity: l.Activity,
			Billable: l.Billable,
			Notes:    l.Notes,
			Tags:     l.Tags,
		}
		lu = &x
	}
	return au, lu, nil
}

// stubWriter writes events using the same logic as the CLI, preserving hashes and format.
type stubWriter struct{}

func (stubWriter) Start(ctx context.Context, p ui.StartParams) error {
	ev := NewStartEvent(IDGen(), p.Customer, p.Project, p.Activity, boolPtr(p.Billable), p.Note, p.Tags, Now())
	return Writer.WriteEvent(ev)
}

func (stubWriter) Stop(ctx context.Context) error {
	ev := NewStopEvent(IDGen(), Now())
	return Writer.WriteEvent(ev)
}

func (stubWriter) Note(ctx context.Context, text string) error {
	ev := Event{ID: IDGen(), Type: "note", TS: Now(), Note: text}
	return Writer.WriteEvent(ev)
}

func (stubWriter) Add(ctx context.Context, p ui.AddParams) error {
	ev := NewAddEvent(IDGen(), p.Customer, p.Project, p.Activity, boolPtr(p.Billable), p.Note, p.Tags, p.Start, p.End)
	return Writer.WriteEvent(ev)
}

func (w stubWriter) Switch(ctx context.Context, p ui.SwitchParams) error {
	if err := w.Stop(ctx); err != nil {
		return err
	}
	return w.Start(ctx, ui.StartParams{
		Customer: p.Customer,
		Project:  p.Project,
		Activity: p.Activity,
		Billable: p.Billable,
		Tags:     p.Tags,
		Note:     p.Note,
	})
}

// stubConfig sources timezone and rounding from viper / existing helpers.
type stubConfig struct{}

func (stubConfig) Timezone() *time.Location {
	tz := viper.GetString("timezone")
	if tz == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}

func (stubConfig) Rounding() ui.RoundingConfig {
	r := getRounding()
	return ui.RoundingConfig{
		Strategy:     r.Strategy,
		QuantumMin:   r.QuantumMin,
		MinimumEntry: r.MinimumEntry,
	}
}
