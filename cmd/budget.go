package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Budget represents a configurable time budget scoped to customer/project/activity
// with optional time windows and enforcement mode.
type Budget struct {
	ID       string  `mapstructure:"id"`
	Customer string  `mapstructure:"customer"`
	Project  string  `mapstructure:"project"`
	Activity string  `mapstructure:"activity"`
	Hours    float64 `mapstructure:"hours"`
	StartsAt string  `mapstructure:"starts_at"`
	EndsAt   string  `mapstructure:"ends_at"`
	Enforce  string  `mapstructure:"enforce"` // warn|block|soft-cap (warn behaves like soft-cap)
	Note     string  `mapstructure:"note"`
}

type budgetRuntime struct {
	Budget
	Start     *time.Time
	End       *time.Time
	TargetMin int
}

// budget commands -----------------------------------------------------------

var (
	budgetSetCustomer string
	budgetSetProject  string
	budgetSetActivity string
	budgetSetHours    float64
	budgetSetRange    string
	budgetSetEnforce  string
	budgetSetNote     string
	budgetSetID       string

	budgetEditHours   float64
	budgetEditRange   string
	budgetEditEnforce string
	budgetEditNote    string
)

var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Manage budgets and alerts",
}

var budgetSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Create or update a budget",
	Run: func(cmd *cobra.Command, args []string) {
		if budgetSetCustomer == "" {
			cobra.CheckErr(fmt.Errorf("--customer is required"))
		}
		if budgetSetHours <= 0 {
			cobra.CheckErr(fmt.Errorf("--hours must be > 0"))
		}
		if budgetSetEnforce == "" {
			budgetSetEnforce = "warn"
		}
		if budgetSetEnforce != "warn" && budgetSetEnforce != "block" && budgetSetEnforce != "soft-cap" {
			cobra.CheckErr(fmt.Errorf("invalid --enforce value: %s", budgetSetEnforce))
		}

		var startStr, endStr string
		if budgetSetRange != "" {
			parts := strings.Split(budgetSetRange, "..")
			if len(parts) != 2 {
				cobra.CheckErr(fmt.Errorf("--range must be A..B"))
			}
			if parts[0] != "" {
				st := mustParseTimeLocal(parts[0])
				startStr = st.Format(time.RFC3339)
			}
			if parts[1] != "" {
				en := mustParseTimeLocal(parts[1])
				endStr = en.Format(time.RFC3339)
			}
		}

		b := Budget{
			ID:       budgetSetID,
			Customer: budgetSetCustomer,
			Project:  budgetSetProject,
			Activity: budgetSetActivity,
			Hours:    budgetSetHours,
			StartsAt: startStr,
			EndsAt:   endStr,
			Enforce:  budgetSetEnforce,
			Note:     budgetSetNote,
		}

		budgets := loadBudgets()
		if b.ID == "" {
			b.ID = fmt.Sprintf("budget-%d", time.Now().UnixNano())
		}

		replaced := false
		for i := range budgets {
			if budgets[i].ID == b.ID {
				budgets[i] = b
				replaced = true
				break
			}
		}
		if !replaced {
			budgets = append(budgets, b)
		}

		if err := saveBudgets(budgets); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to save budget: %w", err))
		}
		if replaced {
			fmt.Printf("budget %s updated\n", b.ID)
		} else {
			fmt.Printf("budget %s created\n", b.ID)
		}
	},
}

var budgetListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List budgets and progress",
	Run: func(cmd *cobra.Command, args []string) {
		budgets := loadBudgets()
		if len(budgets) == 0 {
			fmt.Println("no budgets defined")
			return
		}
		runtimeBudgets := toRuntimeBudgets(budgets)
		now := Now()
		// Determine a sensible window for loading entries: from earliest budget start (or today-30)
		// to latest budget end (or today).
		minFrom := now.AddDate(0, 0, -30)
		maxTo := now
		for _, b := range runtimeBudgets {
			if b.Start != nil && b.Start.Before(minFrom) {
				minFrom = *b.Start
			}
			if b.End != nil && b.End.After(maxTo) {
				maxTo = *b.End
			}
		}
		entries, _ := loadEntries(minFrom, maxTo)
		roundCfg := getRounding()

		// stable order by customer/project/activity then ID
		sort.Slice(runtimeBudgets, func(i, j int) bool {
			if runtimeBudgets[i].Customer != runtimeBudgets[j].Customer {
				return runtimeBudgets[i].Customer < runtimeBudgets[j].Customer
			}
			if runtimeBudgets[i].Project != runtimeBudgets[j].Project {
				return runtimeBudgets[i].Project < runtimeBudgets[j].Project
			}
			if runtimeBudgets[i].Activity != runtimeBudgets[j].Activity {
				return runtimeBudgets[i].Activity < runtimeBudgets[j].Activity
			}
			return runtimeBudgets[i].ID < runtimeBudgets[j].ID
		})

		for _, b := range runtimeBudgets {
			used := budgetUsage(b, entries, roundCfg, now, nil, nil)
			fmt.Printf("%s\n", formatBudgetLine(b, used))
		}
	},
}

var budgetRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Remove a budget",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		budgets := loadBudgets()
		var out []Budget
		removed := false
		for _, b := range budgets {
			if b.ID == id {
				removed = true
				continue
			}
			out = append(out, b)
		}
		if !removed {
			cobra.CheckErr(fmt.Errorf("budget %s not found", id))
		}
		if err := saveBudgets(out); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to save budgets: %w", err))
		}
		fmt.Printf("budget %s removed\n", id)
	},
}

var budgetShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show budget details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		budgets := loadBudgets()
		id := args[0]
		for _, b := range budgets {
			if b.ID == id {
				r := toRuntimeBudgets([]Budget{b})[0]
				from := Now().AddDate(0, 0, -30)
				to := Now()
				if r.Start != nil {
					from = *r.Start
				}
				if r.End != nil {
					to = *r.End
				}
				entries, _ := loadEntries(from, to)
				used := budgetUsage(r, entries, getRounding(), Now(), &from, &to)
				fmt.Println(formatBudgetDetail(r, used))
				return
			}
		}
		cobra.CheckErr(fmt.Errorf("budget %s not found", id))
	},
}

var budgetEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit an existing budget",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		budgets := loadBudgets()
		id := args[0]
		var found *Budget
		for i := range budgets {
			if budgets[i].ID == id {
				found = &budgets[i]
				break
			}
		}
		if found == nil {
			cobra.CheckErr(fmt.Errorf("budget %s not found", id))
		}

		changed := false
		if cmd.Flags().Changed("customer") {
			found.Customer = budgetSetCustomer
			changed = true
		}
		if cmd.Flags().Changed("project") {
			found.Project = budgetSetProject
			changed = true
		}
		if cmd.Flags().Changed("activity") {
			found.Activity = budgetSetActivity
			changed = true
		}
		if cmd.Flags().Changed("hours") {
			if budgetEditHours <= 0 {
				cobra.CheckErr(fmt.Errorf("--hours must be > 0"))
			}
			found.Hours = budgetEditHours
			changed = true
		}
		if cmd.Flags().Changed("enforce") {
			if budgetEditEnforce != "warn" && budgetEditEnforce != "block" && budgetEditEnforce != "soft-cap" {
				cobra.CheckErr(fmt.Errorf("invalid --enforce value: %s", budgetEditEnforce))
			}
			found.Enforce = budgetEditEnforce
			changed = true
		}
		if cmd.Flags().Changed("note") {
			found.Note = budgetEditNote
			changed = true
		}
		if cmd.Flags().Changed("range") {
			parts := strings.Split(budgetEditRange, "..")
			if len(parts) != 2 {
				cobra.CheckErr(fmt.Errorf("--range must be A..B"))
			}
			found.StartsAt, found.EndsAt = "", ""
			if parts[0] != "" {
				st := mustParseTimeLocal(parts[0])
				found.StartsAt = st.Format(time.RFC3339)
			}
			if parts[1] != "" {
				en := mustParseTimeLocal(parts[1])
				found.EndsAt = en.Format(time.RFC3339)
			}
			changed = true
		}

		if !changed {
			fmt.Println("no changes applied")
			return
		}
		if err := saveBudgets(budgets); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to save budgets: %w", err))
		}
		fmt.Printf("budget %s updated\n", id)
	},
}

func init() {
	rootCmd.AddCommand(budgetCmd)
	budgetCmd.AddCommand(budgetSetCmd, budgetListCmd, budgetRmCmd, budgetShowCmd, budgetEditCmd)

	budgetSetCmd.Flags().StringVar(&budgetSetCustomer, "customer", "", "customer (required)")
	budgetSetCmd.Flags().StringVar(&budgetSetProject, "project", "", "project")
	budgetSetCmd.Flags().StringVar(&budgetSetActivity, "activity", "", "activity")
	budgetSetCmd.Flags().Float64Var(&budgetSetHours, "hours", 0, "budget hours (required)")
	budgetSetCmd.Flags().StringVar(&budgetSetRange, "range", "", "time window A..B (either side optional)")
	budgetSetCmd.Flags().StringVar(&budgetSetEnforce, "enforce", "warn", "enforcement mode: warn|block|soft-cap")
	budgetSetCmd.Flags().StringVar(&budgetSetNote, "note", "", "note")
	budgetSetCmd.Flags().StringVar(&budgetSetID, "id", "", "explicit budget id (update when exists)")

	budgetEditCmd.Flags().StringVar(&budgetSetCustomer, "customer", "", "customer")
	budgetEditCmd.Flags().StringVar(&budgetSetProject, "project", "", "project")
	budgetEditCmd.Flags().StringVar(&budgetSetActivity, "activity", "", "activity")
	budgetEditCmd.Flags().Float64Var(&budgetEditHours, "hours", 0, "budget hours")
	budgetEditCmd.Flags().StringVar(&budgetEditRange, "range", "", "time window A..B (either side optional)")
	budgetEditCmd.Flags().StringVar(&budgetEditEnforce, "enforce", "", "enforcement mode: warn|block|soft-cap")
	budgetEditCmd.Flags().StringVar(&budgetEditNote, "note", "", "note")
}

// helpers -------------------------------------------------------------------

func toRuntimeBudgets(b []Budget) []budgetRuntime {
	loc := time.Local
	if tz := viper.GetString("timezone"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	var out []budgetRuntime
	for _, bud := range b {
		br := budgetRuntime{Budget: bud}
		if bud.StartsAt != "" {
			if t, err := time.Parse(time.RFC3339, bud.StartsAt); err == nil {
				br.Start = timePtr(t.In(loc))
			}
		}
		if bud.EndsAt != "" {
			if t, err := time.Parse(time.RFC3339, bud.EndsAt); err == nil {
				br.End = timePtr(t.In(loc))
			}
		}
		br.TargetMin = int(math.Round(bud.Hours * 60))
		if br.TargetMin < 0 {
			br.TargetMin = 0
		}
		if br.Enforce == "" {
			br.Enforce = "warn"
		}
		out = append(out, br)
	}
	return out
}

func timePtr(t time.Time) *time.Time { return &t }

func budgetWindowString(b budgetRuntime) string {
	if b.Start != nil && b.End != nil {
		return fmt.Sprintf("%s..%s", b.Start.Format("2006-01-02"), b.End.Format("2006-01-02"))
	}
	if b.Start != nil {
		return fmt.Sprintf("%s..", b.Start.Format("2006-01-02"))
	}
	if b.End != nil {
		return fmt.Sprintf("..%s", b.End.Format("2006-01-02"))
	}
	return "open"
}

func formatBudgetLine(b budgetRuntime, used int) string {
	remaining := b.TargetMin - used
	if remaining < 0 {
		remaining = 0
	}
	status := "on track"
	if used >= b.TargetMin {
		status = fmt.Sprintf("over by %s", fmtHHMM(used-b.TargetMin))
	}
	return fmt.Sprintf("%s %s/%s [%s] target=%.2fh used=%s remaining=%s window=%s enforce=%s (%s)",
		b.ID,
		b.Customer,
		b.Project,
		b.Activity,
		b.Hours,
		fmtHHMM(used),
		fmtHHMM(remaining),
		budgetWindowString(b),
		b.Enforce,
		status,
	)
}

func formatBudgetDetail(b budgetRuntime, used int) string {
	remaining := b.TargetMin - used
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("id: %s\nscope: %s / %s [%s]\nwindow: %s\nhours: %.2f\nenforce: %s\nnote: %s\nused: %s\nremaining: %s",
		b.ID, b.Customer, b.Project, b.Activity, budgetWindowString(b), b.Hours, b.Enforce, b.Note, fmtHHMM(used), fmtHHMM(remaining))
}

func loadBudgets() []Budget {
	// prefer current viper state
	var budgets []Budget
	if raw := viper.Get("budgets"); raw != nil {
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok {
					budgets = append(budgets, decodeBudgetMap(m))
				}
			}
		}
	}
	if len(budgets) > 0 {
		return budgets
	}

	// fallback to reading config file directly if not already loaded
	cf := configFilePath()
	if _, err := os.Stat(cf); err == nil {
		v := viper.New()
		v.SetConfigFile(cf)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err == nil {
			if raw := v.Get("budgets"); raw != nil {
				if arr, ok := raw.([]interface{}); ok {
					for _, v := range arr {
						if m, ok := v.(map[string]interface{}); ok {
							budgets = append(budgets, decodeBudgetMap(m))
						}
					}
				}
			}
		}
	}
	return budgets
}

func decodeBudgetMap(m map[string]interface{}) Budget {
	b := Budget{}
	if v, ok := m["id"].(string); ok {
		b.ID = v
	}
	if v, ok := m["customer"].(string); ok {
		b.Customer = v
	}
	if v, ok := m["project"].(string); ok {
		b.Project = v
	}
	if v, ok := m["activity"].(string); ok {
		b.Activity = v
	}
	if v, ok := m["hours"].(float64); ok {
		b.Hours = v
	} else if v, ok := m["hours"].(int); ok {
		b.Hours = float64(v)
	}
	if v, ok := m["starts_at"].(string); ok {
		b.StartsAt = v
	}
	if v, ok := m["ends_at"].(string); ok {
		b.EndsAt = v
	}
	if v, ok := m["enforce"].(string); ok {
		b.Enforce = v
	}
	if v, ok := m["note"].(string); ok {
		b.Note = v
	}
	return b
}

func saveBudgets(b []Budget) error {
	var out []map[string]any
	for _, bud := range b {
		m := map[string]any{
			"id":       bud.ID,
			"customer": bud.Customer,
			"project":  bud.Project,
			"activity": bud.Activity,
			"hours":    bud.Hours,
			"enforce":  bud.Enforce,
		}
		if bud.StartsAt != "" {
			m["starts_at"] = bud.StartsAt
		}
		if bud.EndsAt != "" {
			m["ends_at"] = bud.EndsAt
		}
		if bud.Note != "" {
			m["note"] = bud.Note
		}
		out = append(out, m)
	}
	viper.Set("budgets", out)
	return saveViperConfig()
}

// budget evaluation ---------------------------------------------------------

type budgetCandidate struct {
	Customer string
	Project  string
	Activity string
	Start    time.Time
	End      *time.Time
}

// checkBudgets evaluates matching budgets and returns warnings or a blocking error.
func checkBudgets(budgets []budgetRuntime, entries []Entry, cand budgetCandidate, rounding Rounding, now time.Time) ([]string, error) {
	var warnings []string

	for _, b := range budgets {
		match, _ := budgetMatchesScope(b, cand.Customer, cand.Project, cand.Activity)
		if !match {
			continue
		}
		if !budgetWindowOverlaps(b, cand.Start, candEndOrStart(cand)) {
			continue
		}

		used := budgetUsage(b, entries, rounding, now, nil, nil)
		candMin := 0
		if cand.End != nil {
			candMin = roundMinutes(int(cand.End.Sub(cand.Start).Minutes()), rounding)
		}
		projected := used + candMin

		if b.TargetMin <= 0 {
			continue
		}

		remaining := b.TargetMin - projected
		if projected >= b.TargetMin && strings.EqualFold(b.Enforce, "block") {
			return warnings, fmt.Errorf("budget %s exceeded for %s/%s [%s]: target=%s projected=%s remaining=%s", b.ID, b.Customer, b.Project, b.Activity, fmtHHMM(b.TargetMin), fmtHHMM(projected), fmtHHMM(remaining))
		}
		if projected >= b.TargetMin {
			msg := fmt.Sprintf("budget %s near/exceeded: %s/%s [%s] target=%s projected=%s remaining=%s", b.ID, b.Customer, b.Project, b.Activity, fmtHHMM(b.TargetMin), fmtHHMM(projected), fmtHHMM(remaining))
			warnings = append(warnings, msg)
		}
	}

	return warnings, nil
}

// runBudgetChecks is a convenience that filters relevant budgets, loads entries over the
// combined window, and evaluates enforcement. It returns warnings (if any) and a blocking
// error when enforce=block is hit.
func runBudgetChecks(budgets []budgetRuntime, cand budgetCandidate, rounding Rounding, now time.Time) ([]string, error) {
	// filter relevant budgets by scope and window
	relevant := filterBudgetsForCandidate(budgets, cand)
	if len(relevant) == 0 {
		return nil, nil
	}

	from, to := windowForBudgets(relevant, cand)

	entries, err := loadEntries(from, to)
	if err != nil {
		return nil, err
	}
	return checkBudgets(relevant, entries, cand, rounding, now)
}

func filterBudgetsForCandidate(budgets []budgetRuntime, cand budgetCandidate) []budgetRuntime {
	var relevant []budgetRuntime
	for _, b := range budgets {
		match, _ := budgetMatchesScope(b, cand.Customer, cand.Project, cand.Activity)
		if !match {
			continue
		}
		if !budgetWindowOverlaps(b, cand.Start, candEndOrStart(cand)) {
			continue
		}
		relevant = append(relevant, b)
	}
	return relevant
}

func windowForBudgets(budgets []budgetRuntime, cand budgetCandidate) (time.Time, time.Time) {
	from := cand.Start
	to := candEndOrStart(cand)
	for _, b := range budgets {
		if b.Start != nil && b.Start.Before(from) {
			from = *b.Start
		}
		if b.End != nil && b.End.After(to) {
			to = *b.End
		}
	}
	return from, to
}

func candEndOrStart(c budgetCandidate) time.Time {
	if c.End != nil {
		return *c.End
	}
	return c.Start
}

// budgetMatchesScope returns true when the budget matches the provided scope. Specificity
// (higher is more specific) can be used by callers to pick preferred budgets.
func budgetMatchesScope(b budgetRuntime, customer, project, activity string) (bool, int) {
	score := 0
	if b.Customer != "" {
		if b.Customer != customer {
			return false, 0
		}
		score++
	}
	if b.Project != "" {
		if b.Project != project {
			return false, 0
		}
		score++
	}
	if b.Activity != "" {
		if b.Activity != activity {
			return false, 0
		}
		score++
	}
	return true, score
}

func budgetWindowOverlaps(b budgetRuntime, start, end time.Time) bool {
	if b.Start != nil && end.Before(*b.Start) {
		return false
	}
	if b.End != nil && start.After(*b.End) {
		return false
	}
	return true
}

func budgetUsage(b budgetRuntime, entries []Entry, rounding Rounding, now time.Time, windowStart, windowEnd *time.Time) int {
	used := 0
	for _, e := range entries {
		if ok, _ := budgetMatchesScope(b, e.Customer, e.Project, e.Activity); !ok {
			continue
		}
		end := e.End
		if end == nil {
			e := now
			end = &e
		}
		if b.Start != nil && end.Before(*b.Start) {
			continue
		}
		if b.End != nil && e.Start.After(*b.End) {
			continue
		}
		segStart := e.Start
		segEnd := *end
		if b.Start != nil && segStart.Before(*b.Start) {
			segStart = *b.Start
		}
		if b.End != nil && segEnd.After(*b.End) {
			segEnd = *b.End
		}
		if windowStart != nil && segEnd.Before(*windowStart) {
			continue
		}
		if windowEnd != nil && segStart.After(*windowEnd) {
			continue
		}
		if windowStart != nil && segStart.Before(*windowStart) {
			segStart = *windowStart
		}
		if windowEnd != nil && segEnd.After(*windowEnd) {
			segEnd = *windowEnd
		}
		if !segEnd.After(segStart) {
			continue
		}
		mins := int(segEnd.Sub(segStart).Minutes())
		if mins <= 0 {
			continue
		}
		used += roundMinutes(mins, rounding)
	}
	return used
}

func printBudgetWarnings(ws []string) {
	for _, w := range ws {
		fmt.Printf("%sBUDGET:%s %s\n", ansiWarn, ansiReset, w)
	}
}

// persistence reuse ---------------------------------------------------------

// saveViperConfig lives in alias.go. Keep a small wrapper here to avoid import cycles.
func init() {
	// ensure config directory exists for budgets as well
	cf := configFilePath()
	_ = os.MkdirAll(filepath.Dir(cf), 0o755)
}
