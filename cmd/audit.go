package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit and verify the hash-chain of journals",
}

var auditVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify per-day journal hash chain",
	Run: func(cmd *cobra.Command, args []string) {
		home, _ := os.UserHomeDir()
		base := filepath.Join(home, ".tt", "journal")
		ok := true
		_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Walk-level error: print a helpful message and continue walking
				fmt.Printf("WARN Walk error for %s: %v\n", path, err)
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			// Use stdout as the writer so messages are visible to the user
			if verifyDay(path, os.Stdout) {
				fmt.Printf("OK  %s\n", path)
			} else {
				fmt.Printf("ERR %s\n", path)
				ok = false
			}
			return nil
		})
		if !ok {
			// Non-zero exit code indicates at least one file failed verification
			os.Exit(1)
		}
	},
}

var auditRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Create proposed repairs for journal files (writes .repair files in dry-run)",
	Long: `Repair will recompute canonical hashes for every event in the journal files.
By default this command runs in dry-run mode and writes proposed files with the suffix
'.repair' next to the original journal. It also prints a small inline diff of the first
changes so you can inspect before applying. To actually apply changes use --apply (dangerous).`,
	Run: func(cmd *cobra.Command, args []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		apply, _ := cmd.Flags().GetBool("apply")

		home, _ := os.UserHomeDir()
		base := filepath.Join(home, ".tt", "journal")
		changedFiles := 0
		writtenRepairs := 0
		errFiles := 0

		_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("WARN Walk error for %s: %v\n", path, err)
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			ok, wrote, err := repairDay(path, dryRun, apply, os.Stdout)
			if err != nil {
				fmt.Printf("ERROR repairing %s: %v\n", path, err)
				errFiles++
				return nil
			}
			if ok {
				changedFiles++
			}
			if wrote {
				writtenRepairs++
			}
			return nil
		})

		fmt.Printf("\nSummary: changed files: %d, .repair files written: %d, errors: %d\n", changedFiles, writtenRepairs, errFiles)
		if errFiles > 0 {
			os.Exit(2)
		}
	},
}

func init() {
	auditCmd.AddCommand(auditVerifyCmd)
	auditCmd.AddCommand(auditRepairCmd)

	// repair flags
	auditRepairCmd.Flags().Bool("dry-run", true, "When true (default) write proposed changes to .repair files and do not modify originals")
	auditRepairCmd.Flags().Bool("apply", false, "When true, overwrite original files with repaired content and update .hash anchors (irreversible without backup).")
}

// repairDay reads a journal file, recomputes canonical hashes for every event (sequentially),
// and prepares a proposed rewritten file. If dryRun is true, the proposed content is written
// to path + \".repair\" and the function prints a small diff preview. If apply is true (and dryRun=false),
// the original file and its .hash are overwritten (backups are made with .bak suffix).
//
// Returns (changed bool, wroteRepair bool, err error).
func repairDay(path string, dryRun bool, apply bool, w io.Writer) (bool, bool, error) {
	// Read original file
	f, err := os.Open(path)
	if err != nil {
		return false, false, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	origLines := []string{}
	for scanner.Scan() {
		origLines = append(origLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return false, false, fmt.Errorf("scan: %w", err)
	}

	// Read anchor
	anchorPath := strings.TrimSuffix(path, ".jsonl") + ".hash"
	anchorBytes, _ := os.ReadFile(anchorPath)
	anchor := strings.TrimSpace(string(anchorBytes))
	prev := ""
	if anchor != "" {
		prev = anchor
		fmt.Fprintf(w, "INFO: %s has anchor %s — using as starting prev\n", path, anchor)
	} else {
		fmt.Fprintf(w, "INFO: %s has no anchor — starting prev empty\n", path)
	}

	// If file is empty, nothing to do
	if len(origLines) == 0 {
		fmt.Fprintf(w, "INFO: %s is empty, skipping\n", path)
		return false, false, nil
	}

	newLines := make([]string, 0, len(origLines))
	changed := false

	for i, raw := range origLines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// preserve blank lines
			newLines = append(newLines, raw)
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(trimmed), &e); err != nil {
			// Cannot repair if parsing fails
			fmt.Fprintf(w, "ERROR: JSON parse failed at %s:%d: %v\n", path, lineNum, err)
			return false, false, fmt.Errorf("json parse at %s:%d: %w", path, lineNum, err)
		}

		// Recompute canonical payload hash using canonicalPayload struct
		cp := canonicalPayload{
			ID:       e.ID,
			Type:     e.Type,
			TS:       e.TS.Format(time.RFC3339Nano),
			User:     e.User,
			Customer: e.Customer,
			Project:  e.Project,
			Activity: e.Activity,
			Billable: e.Billable,
			Note:     e.Note,
			Tags:     e.Tags,
			Ref:      e.Ref,
			PrevHash: prev,
		}
		jStruct, _ := json.Marshal(cp)
		hStruct := sha256.Sum256(jStruct)
		calcStruct := hex.EncodeToString(hStruct[:])

		// Also compute legacy-map hash (for historical compatibility)
		legacyPayload := map[string]any{
			"id":        e.ID,
			"type":      e.Type,
			"ts":        e.TS.Format(time.RFC3339Nano),
			"user":      e.User,
			"customer":  e.Customer,
			"project":   e.Project,
			"activity":  e.Activity,
			"billable":  e.Billable,
			"note":      e.Note,
			"tags":      e.Tags,
			"ref":       e.Ref,
			"prev_hash": prev,
		}
		jLegacy, _ := json.Marshal(legacyPayload)
		hLegacy := sha256.Sum256(jLegacy)
		calcLegacy := hex.EncodeToString(hLegacy[:])

		// Decide accepted hash: we want canonical moving forward.
		acceptedHash := calcStruct

		// If the stored hash already matches canonical, nothing to change for this record
		if e.Hash == calcStruct {
			// ensure PrevHash field in event matches prev (if not, we'll update it)
			if e.PrevHash != prev {
				e.PrevHash = prev
				changed = true
			}
		} else {
			// If stored equals legacy, we will migrate to canonical (i.e. replace stored hash)
			if e.Hash == calcLegacy {
				// migrating from legacy: update PrevHash and Hash to canonical values
				e.PrevHash = prev
				e.Hash = calcStruct
				changed = true
			} else {
				// neither matched: the row is inconsistent (possibly edited). We'll still propose canonical rewrite.
				fmt.Fprintf(w, "WARN: line %d in %s had stored hash %q that matches neither legacy nor canonical; proposing canonical rewrite\n", lineNum, path, e.Hash)
				e.PrevHash = prev
				e.Hash = calcStruct
				changed = true
			}
		}

		// Marshal event back to JSON (this preserves the same Event struct layout)
		newLineBytes, _ := json.Marshal(e)
		newLine := string(newLineBytes)
		newLines = append(newLines, newLine)

		// Advance prev using the canonical value we set for this event
		prev = acceptedHash
	}

	// Compose new content
	var buf bytes.Buffer
	for _, l := range newLines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	newContent := buf.String()

	// If nothing changed, report and exit
	if !changed {
		fmt.Fprintf(w, "OK: %s would not change (already canonical)\n", path)
		return false, false, nil
	}

	// Prepare repair path and write it (dry-run behavior)
	repairPath := path + ".repair"
	if dryRun {
		if err := os.WriteFile(repairPath, []byte(newContent), 0o644); err != nil {
			return false, false, fmt.Errorf("write repair: %w", err)
		}
		// also write proposed anchor to .hash.repair for inspection
		repairAnchorPath := anchorPath + ".repair"
		if err := os.WriteFile(repairAnchorPath, []byte(prev+"\n"), 0o644); err != nil {
			return false, false, fmt.Errorf("write repair anchor: %w", err)
		}
		fmt.Fprintf(w, "WROTE: %s (proposed rewrite)\n", repairPath)
		fmt.Fprintf(w, "WROTE: %s (proposed anchor)\n", repairAnchorPath)
		// show a small inline diff preview
		showInlineDiffPreview(path, origLines, newLines, w)
		return true, true, nil
	}

	// If apply is requested (and dryRun=false), perform destructive rewrite with backup
	if apply {
		backupPath := path + ".bak"
		if err := os.Rename(path, backupPath); err != nil {
			return false, false, fmt.Errorf("backup original: %w", err)
		}
		if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
			// attempt to restore backup
			_ = os.Rename(backupPath, path)
			return false, false, fmt.Errorf("write applied file: %w", err)
		}
		// update anchor (make backup)
		if _, err := os.Stat(anchorPath); err == nil {
			_ = os.Rename(anchorPath, anchorPath+".bak")
		}
		if err := os.WriteFile(anchorPath, []byte(prev+"\n"), 0o644); err != nil {
			return false, false, fmt.Errorf("write anchor: %w", err)
		}
		fmt.Fprintf(w, "APPLIED: %s (backup at %s)\n", path, backupPath)
		return true, false, nil
	}

	// If we reach here, no operation done
	return true, false, nil
}

// showInlineDiffPreview prints a small, human-readable preview of differences between old and new lines.
// It prints up to the first N differing lines with context and marks removals with '-' and additions with '+'.
func showInlineDiffPreview(path string, oldLines, newLines []string, w io.Writer) {
	fmt.Fprintf(w, "DIFF preview for %s (first changes shown):\n", path)
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	shown := 0
	for i := 0; i < max && shown < 10; i++ {
		var oldL, newL string
		if i < len(oldLines) {
			oldL = oldLines[i]
		}
		if i < len(newLines) {
			newL = newLines[i]
		}
		if oldL == newL {
			// print context lines sparsely if near a change
			continue
		}
		// print a few context lines before change if available
		start := i - 2
		if start < 0 {
			start = 0
		}
		for j := start; j < i && shown < 10; j++ {
			if j < len(oldLines) && j < len(newLines) && oldLines[j] == newLines[j] {
				fmt.Fprintf(w, "  %4d  %s\n", j+1, trimForPreview(oldLines[j]))
				shown++
			}
		}
		// removed line
		if oldL != "" {
			fmt.Fprintf(w, "- %4d  %s\n", i+1, trimForPreview(oldL))
			shown++
		}
		// added line
		if newL != "" {
			fmt.Fprintf(w, "+ %4d  %s\n", i+1, trimForPreview(newL))
			shown++
		}
		// show a separator
		fmt.Fprintln(w, "  ...")
	}
	if shown == 0 {
		fmt.Fprintln(w, "  (no line-level differences detected in preview)")
	}
}

// trimForPreview shortens very long lines for readable diffs.
func trimForPreview(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
