package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// customer-merge flags
var (
	cmTargets string // comma-separated ids
	cmSince   string // since time (optional)
	cmFrom    string // comma-separated source customer names to match when using --since
	cmTo      string // canonical customer name to set
	cmNote    string // note to attach to amend events
	cmDryRun  bool   // default true to avoid surprises
)

// mergedCustomerSet holds mapping source -> canonical loaded into memory for quick checks.
// It is populated from viper ("customers.map") on init and when customer-merge is run.
var mergedCustomerSet map[string]string

// customerMergeCmd implements a non-destructive merge of customers by writing amend events
// that set the canonical customer on matching entries. It also persists the mapping into
// viper under "customers.map" so tooling and completion helpers can consult it.
var customerMergeCmd = &cobra.Command{
	Use:   "customer-merge",
	Short: "Non-destructively merge customer names by writing amend events (append-only)",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate inputs
		if strings.TrimSpace(cmTo) == "" {
			cobra.CheckErr(fmt.Errorf("--to is required (target canonical customer)"))
		}
		canonical := strings.TrimSpace(cmTo)

		var targetIDs []string
		origByID := map[string]string{} // optional map of original customer for metadata

		if cmTargets != "" {
			for _, p := range strings.Split(cmTargets, ",") {
				if id := strings.TrimSpace(p); id != "" {
					targetIDs = append(targetIDs, id)
				}
			}
		} else if cmSince != "" {
			from := mustParseTimeLocal(cmSince)
			to := nowLocal()
			ents, err := loadEntries(from, to)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed loading entries for selection: %w", err))
			}

			// Build set of source customer names to match (if provided)
			matchSet := map[string]struct{}{}
			if cmFrom != "" {
				for _, name := range strings.Split(cmFrom, ",") {
					name = strings.TrimSpace(name)
					if name != "" {
						matchSet[name] = struct{}{}
					}
				}
			}

			for _, e := range ents {
				// If --from provided, only include those customers
				if len(matchSet) > 0 {
					if _, ok := matchSet[e.Customer]; !ok {
						continue
					}
				}
				// If the entry already has the canonical name, skip
				if e.Customer == canonical {
					continue
				}
				targetIDs = append(targetIDs, e.ID)
				origByID[e.ID] = e.Customer
			}
		} else {
			cobra.CheckErr(fmt.Errorf("either --targets or --since (plus optional --from) must be provided"))
		}

		if len(targetIDs) == 0 {
			cobra.CheckErr(fmt.Errorf("no target entries found"))
		}

		// Build mapping entries to persist: for each source in cmFrom (or discovered), map -> canonical.
		// If explicit --from provided, use that list; otherwise use discovered origByID values.
		newMappings := map[string]string{}
		if cmFrom != "" {
			for _, name := range strings.Split(cmFrom, ",") {
				n := strings.TrimSpace(name)
				if n != "" && n != canonical {
					newMappings[n] = canonical
				}
			}
		} else {
			for _, src := range origByID {
				if src != "" && src != canonical {
					newMappings[src] = canonical
				}
			}
		}

		// Dry-run: print plan and exit
		if cmDryRun {
			fmt.Printf("DRY RUN: would write %d amend event(s) setting customer -> %q\n", len(targetIDs), canonical)
			for _, id := range targetIDs {
				if orig, ok := origByID[id]; ok {
					fmt.Printf("  - %s : %s -> %s\n", id, orig, canonical)
				} else {
					fmt.Printf("  - %s : -> %s\n", id, canonical)
				}
			}
			if len(newMappings) > 0 {
				fmt.Println("DRY RUN: would persist customer mappings:")
				for s, t := range newMappings {
					fmt.Printf("  - %q -> %q\n", s, t)
				}
			}
			return
		}

		// Persist mappings into viper under "customers.map" so completion and other helpers can consult it.
		// Merge with existing map (do not remove prior mappings).
		existing := viper.GetStringMapString("customers.map")
		if existing == nil {
			existing = map[string]string{}
		}
		changed := false
		for s, t := range newMappings {
			if s == "" || t == "" {
				continue
			}
			// only record change if different or absent
			if cur, ok := existing[s]; !ok || cur != t {
				existing[s] = t
				changed = true
			}
		}
		if changed {
			viper.Set("customers.map", existing)
			// Persist via saveViperConfig so we follow the project's central save routine.
			if err := saveViperConfig(); err != nil {
				// If write fails, report but continue (we still wrote amend events below).
				cmd.Printf("warning: failed to persist customer mapping to config: %v\n", err)
			} else {
				// update runtime cache
				loadMergedCustomerMapIntoMemory()
			}
		}

		// Apply: write amend events
		var written int
		for _, id := range targetIDs {
			meta := map[string]string{}
			if orig, ok := origByID[id]; ok && orig != "" {
				meta["merged_from"] = orig
			}
			ev := Event{
				ID:       IDGen(),
				Type:     "amend",
				TS:       Now(),
				Ref:      id,
				Note:     cmNote,
				Customer: canonical,
				Meta:     meta,
			}
			if err := writeEvent(ev); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to write amend event for %s: %w", id, err))
			}
			written++
		}
		cmd.Printf("Wrote %d amend event(s) setting customer -> %q\n", written, canonical)
	},
}

func init() {
	customerMergeCmd.Flags().StringVar(&cmTargets, "targets", "", "comma-separated target entry ids to amend")
	customerMergeCmd.Flags().StringVar(&cmSince, "since", "", "include entries since this time (uses same parse rules as other commands)")
	customerMergeCmd.Flags().StringVar(&cmFrom, "from", "", "comma-separated source customer names to match when using --since (optional)")
	customerMergeCmd.Flags().StringVar(&cmTo, "to", "", "canonical customer name to set (required)")
	customerMergeCmd.Flags().StringVar(&cmNote, "note", "", "note to append to each amend event")
	customerMergeCmd.Flags().BoolVar(&cmDryRun, "dry-run", true, "perform a dry-run (default true); use --dry-run=false to actually write amend events")

	rootCmd.AddCommand(customerMergeCmd)

	// Initialize in-memory merged map from config so completion helpers can consult it.
	loadMergedCustomerMapIntoMemory()
}

// loadMergedCustomerMapIntoMemory loads the "customers.map" key from viper into mergedCustomerSet
// (a simple string->string map). It's safe to call multiple times.
func loadMergedCustomerMapIntoMemory() {
	m := viper.GetStringMapString("customers.map")
	if m == nil {
		mergedCustomerSet = map[string]string{}
		return
	}
	mergedCustomerSet = map[string]string{}
	for k, v := range m {
		if k == "" || v == "" {
			continue
		}
		mergedCustomerSet[k] = v
	}
}

// IsCustomerMerged reports whether the provided customer name has been merged (is a source).
// This is useful for completion helpers to hide merged source names.
func IsCustomerMerged(name string) bool {
	if name == "" {
		return false
	}
	if mergedCustomerSet == nil {
		loadMergedCustomerMapIntoMemory()
	}
	_, ok := mergedCustomerSet[name]
	return ok
}

// CanonicalCustomer returns the canonical name for the given source customer if known,
// otherwise returns the input unchanged.
func CanonicalCustomer(name string) string {
	if name == "" {
		return name
	}
	if mergedCustomerSet == nil {
		loadMergedCustomerMapIntoMemory()
	}
	if c, ok := mergedCustomerSet[name]; ok && c != "" {
		return c
	}
	return name
}

// FilterCustomersForCompletion removes any source customer names that have been merged
// (so they don't appear in completion lists). The returned slice is sorted and deduplicated.
func FilterCustomersForCompletion(input []string) []string {
	if len(input) == 0 {
		return input
	}
	if mergedCustomerSet == nil {
		loadMergedCustomerMapIntoMemory()
	}
	outSet := map[string]struct{}{}
	for _, s := range input {
		if s == "" {
			continue
		}
		// skip source names that were merged away
		if IsCustomerMerged(s) {
			// but ensure canonical appears instead: include canonical if not empty
			if c := CanonicalCustomer(s); c != "" {
				outSet[c] = struct{}{}
			}
			continue
		}
		outSet[s] = struct{}{}
	}
	out := make([]string, 0, len(outSet))
	for k := range outSet {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
