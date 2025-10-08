package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if verifyDay(path) {
				fmt.Printf("OK  %s\n", path)
			} else {
				fmt.Printf("ERR %s\n", path)
				ok = false
			}
			return nil
		})
		if !ok {
			os.Exit(1)
		}
	},
}

func init() {
	auditCmd.AddCommand(auditVerifyCmd)
}

func verifyDay(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	prev := readLastHash(strings.TrimSuffix(path, ".jsonl"))
	// We recompute by scanning lines and ensuring each record's prev matches the file-prev,
	// then updating prev with the recomputed hash of the current record.
	// Because we persist the last hash alongside the file, we treat it as an anchor.
	lines := []string{}
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if len(lines) == 0 {
		return true
	}
	// Recompute hash chain fresh
	prev = ""
	for _, line := range lines {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return false
		}
		payload := map[string]any{
			"id": e.ID, "type": e.Type, "ts": e.TS.Format(time.RFC3339Nano),
			"user": e.User, "customer": e.Customer, "project": e.Project,
			"activity": e.Activity, "billable": e.Billable, "note": e.Note,
			"tags": e.Tags, "ref": e.Ref, "prev_hash": prev,
		}
		j, _ := json.Marshal(payload)
		h := sha256.Sum256(j)
		calc := hex.EncodeToString(h[:])
		if e.Hash != calc {
			return false
		}
		prev = calc
	}
	// Optional: compare prev with saved .hash (end-of-day anchor)
	anchorBytes, _ := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".hash")
	anchor := strings.TrimSpace(string(anchorBytes))
	if anchor != "" && anchor != prev {
		return false
	}
	return true
}
