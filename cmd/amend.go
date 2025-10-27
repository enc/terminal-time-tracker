package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// amend command flags
var (
	amendLast      bool
	amendSelect    bool
	amendStartStr  string
	amendEndStr    string
	amendNote      string
	amendCustomer  string
	amendProject   string
	amendActivity  string
	amendBillableF string // "", "true", "false"
	amendTags      []string
)

// split command flags
var (
	splitLast      bool
	splitAtStr     string
	splitLeftNote  string
	splitRightNote string
	splitCustomer  string
	splitProject   string
	splitActivity  string
	splitBillableF string
	splitTags      []string
)

// merge command flags
var (
	mergeTargets   string // comma-separated ids
	mergeSince     string // date/time lower bound
	mergeCustomer  string
	mergeProject   string
	mergeActivity  string
	mergeIntoNote  string
	mergeBillableF string
)

var amendCmd = &cobra.Command{
	Use:   "amend [id]",
	Short: "Create an amend event that updates an existing entry (append-only)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var (
			targetID string
			err      error
		)
		switch {
		case len(args) == 1:
			targetID = args[0]
		case amendSelect:
			entries, loadErr := loadRecentEntriesForAmend()
			if loadErr != nil {
				cobra.CheckErr(fmt.Errorf("failed to load entries for selection: %w", loadErr))
			}
			if len(entries) == 0 {
				cobra.CheckErr(fmt.Errorf("no entries found to select; add an entry first"))
			}
			var entry *Entry
			entry, err = selectEntryForAmend(entries)
			if err != nil {
				if errors.Is(err, errSelectionCancelled) {
					fmt.Println("Selection cancelled")
					return
				}
				cobra.CheckErr(fmt.Errorf("failed to run selector: %w", err))
			}
			if entry == nil {
				fmt.Println("Selection cancelled")
				return
			}
			targetID = entry.ID
		case amendLast:
			var entry *Entry
			entry, err = findMostRecentEntryForAmend()
			if err != nil {
				cobra.CheckErr(err)
			}
			targetID = entry.ID
		default:
			var entry *Entry
			entry, err = findMostRecentEntryForAmend()
			if err != nil {
				cobra.CheckErr(err)
			}
			targetID = entry.ID
		}

		meta := map[string]string{}
		if amendStartStr != "" {
			ts := mustParseTimeLocal(amendStartStr)
			meta["start"] = ts.Format(time.RFC3339)
		}
		if amendEndStr != "" {
			ts := mustParseTimeLocal(amendEndStr)
			meta["end"] = ts.Format(time.RFC3339)
		}

		var billable *bool
		if amendBillableF != "" {
			v, err := parseBoolFlag(amendBillableF)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid --billable value: %v", err))
			}
			billable = v
		}

		ev := Event{
			ID:       IDGen(),
			Type:     "amend",
			TS:       Now(),
			Ref:      targetID,
			Note:     amendNote,
			Customer: amendCustomer,
			Project:  amendProject,
			Activity: amendActivity,
			Billable: billable,
			Tags:     amendTags,
			Meta:     meta,
		}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write amend event: %w", err))
		}
		fmt.Printf("Amend event written for %s\n", targetID)
	},
}

var splitCmd = &cobra.Command{
	Use:   "split [id]",
	Short: "Create a split event that splits an existing entry into two (append-only)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var targetID string
		if len(args) == 1 {
			targetID = args[0]
		} else if splitLast {
			now := nowLocal()
			from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			to := from
			ents, err := loadEntries(from, to)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed loading entries to locate last: %w", err))
			}
			if len(ents) == 0 {
				cobra.CheckErr(fmt.Errorf("no entries found for today to split"))
			}
			last := ents[0]
			for _, e := range ents {
				if e.Start.After(last.Start) {
					last = e
				}
			}
			targetID = last.ID
		} else {
			cobra.CheckErr(fmt.Errorf("either provide an id or --last"))
		}

		if splitAtStr == "" {
			cobra.CheckErr(fmt.Errorf("--at (split time) is required"))
		}
		splitAt := mustParseTimeLocal(splitAtStr)

		meta := map[string]string{
			"split_at": splitAt.Format(time.RFC3339),
		}
		if splitLeftNote != "" {
			meta["left_note"] = splitLeftNote
		}
		if splitRightNote != "" {
			meta["right_note"] = splitRightNote
		}

		var billable *bool
		if splitBillableF != "" {
			v, err := parseBoolFlag(splitBillableF)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid --billable value: %v", err))
			}
			billable = v
		}

		ev := Event{
			ID:       IDGen(),
			Type:     "split",
			TS:       Now(),
			Ref:      targetID,
			Customer: splitCustomer,
			Project:  splitProject,
			Activity: splitActivity,
			Billable: billable,
			Tags:     splitTags,
			Meta:     meta,
		}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write split event: %w", err))
		}
		fmt.Printf("Split event written for %s at %s\n", targetID, splitAt.Format(time.Kitchen))
	},
}

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Create a merge event that consolidates multiple entries into one (append-only)",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		meta := map[string]string{}

		var targetIDs []string
		if mergeTargets != "" {
			for _, p := range strings.Split(mergeTargets, ",") {
				if id := strings.TrimSpace(p); id != "" {
					targetIDs = append(targetIDs, id)
				}
			}
		} else if mergeSince != "" {
			// load entries from since..today and filter by optional customer/project
			from := mustParseTimeLocal(mergeSince)
			to := nowLocal()
			ents, err := loadEntries(from, to)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed loading entries for merge: %w", err))
			}
			for _, e := range ents {
				if mergeCustomer != "" && e.Customer != mergeCustomer {
					continue
				}
				if mergeProject != "" && e.Project != mergeProject {
					continue
				}
				targetIDs = append(targetIDs, e.ID)
			}
		} else {
			cobra.CheckErr(fmt.Errorf("either --targets or --since must be provided"))
		}

		if len(targetIDs) == 0 {
			cobra.CheckErr(fmt.Errorf("no target entries found to merge"))
		}
		meta["targets"] = strings.Join(targetIDs, ",")

		var billable *bool
		if mergeBillableF != "" {
			v, err := parseBoolFlag(mergeBillableF)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid --billable value: %v", err))
			}
			billable = v
		}

		ev := Event{
			ID:       IDGen(),
			Type:     "merge",
			TS:       Now(),
			Note:     mergeIntoNote,
			Customer: mergeCustomer,
			Project:  mergeProject,
			Activity: mergeActivity,
			Billable: billable,
			Meta:     meta,
		}
		if err := writeEvent(ev); err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write merge event: %w", err))
		}
		fmt.Printf("Merge event written, combining %d entries\n", len(targetIDs))
	},
}

func parseBoolFlag(s string) (*bool, error) {
	if s == "" {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y":
		b := true
		return &b, nil
	case "false", "0", "no", "n":
		b := false
		return &b, nil
	default:
		return nil, fmt.Errorf("invalid boolean: %s", s)
	}
}

func init() {
	// amend flags
	amendCmd.Flags().BoolVar(&amendLast, "last", false, "amend the most recent entry (default when no id is provided)")
	amendCmd.Flags().BoolVar(&amendSelect, "select", false, "choose an entry interactively when no id is provided")
	amendCmd.Flags().StringVar(&amendStartStr, "start", "", "new start time (RFC3339 or human-friendly formats)")
	amendCmd.Flags().StringVar(&amendEndStr, "end", "", "new end time (RFC3339 or human-friendly formats)")
	amendCmd.Flags().StringVar(&amendNote, "note", "", "note to append to the entry")
	amendCmd.Flags().StringVar(&amendCustomer, "customer", "", "customer override")
	amendCmd.Flags().StringVar(&amendProject, "project", "", "project override")
	amendCmd.Flags().StringVar(&amendActivity, "activity", "", "activity override")
	amendCmd.Flags().StringVar(&amendBillableF, "billable", "", "set billable: true|false (empty leaves unchanged)")
	amendCmd.Flags().StringSliceVar(&amendTags, "tag", []string{}, "replace tags (comma-separated)")

	// split flags
	splitCmd.Flags().BoolVar(&splitLast, "last", false, "split the last entry (instead of specifying an id)")
	splitCmd.Flags().StringVar(&splitAtStr, "at", "", "split at time (RFC3339 or human-friendly formats) (required)")
	splitCmd.Flags().StringVar(&splitLeftNote, "left-note", "", "note for the left split")
	splitCmd.Flags().StringVar(&splitRightNote, "right-note", "", "note for the right split")
	splitCmd.Flags().StringVar(&splitCustomer, "customer", "", "customer override for split parts")
	splitCmd.Flags().StringVar(&splitProject, "project", "", "project override for split parts")
	splitCmd.Flags().StringVar(&splitActivity, "activity", "", "activity override for split parts")
	splitCmd.Flags().StringVar(&splitBillableF, "billable", "", "set billable for split parts: true|false (empty leaves unchanged)")
	splitCmd.Flags().StringSliceVar(&splitTags, "tag", []string{}, "replace tags for split parts")

	// merge flags
	mergeCmd.Flags().StringVar(&mergeTargets, "targets", "", "comma-separated target entry ids to merge")
	mergeCmd.Flags().StringVar(&mergeSince, "since", "", "include entries since this time (RFC3339 or human-friendly)")
	mergeCmd.Flags().StringVar(&mergeCustomer, "customer", "", "filter by customer when using --since or override customer for merged entry")
	mergeCmd.Flags().StringVar(&mergeProject, "project", "", "filter by project when using --since or override project for merged entry")
	mergeCmd.Flags().StringVar(&mergeActivity, "activity", "", "override activity for merged entry")
	mergeCmd.Flags().StringVar(&mergeIntoNote, "into", "", "note/summary for the merged entry")
	mergeCmd.Flags().StringVar(&mergeBillableF, "billable", "", "set billable for merged entry: true|false (empty leaves policy to resolution)")

	rootCmd.AddCommand(amendCmd)
	rootCmd.AddCommand(splitCmd)
	rootCmd.AddCommand(mergeCmd)
}
