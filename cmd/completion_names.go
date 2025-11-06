package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NameStats tracks occurrences for a specific raw name and when it was seen.
type NameStats struct {
	Raw       string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// CustomerGroup aggregates raw names under a canonical customer identifier.
type CustomerGroup struct {
	Canonical string
	Names     map[string]*NameStats
	Total     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// ProjectStats tracks occurrences for a project name tied to a canonical customer.
type ProjectStats struct {
	Name      string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// CompletionIndex aggregates customer and project observations from the journal.
type CompletionIndex struct {
	Customers map[string]*CustomerGroup           // canonical customer -> group
	Projects  map[string]map[string]*ProjectStats // canonical customer -> project -> stats
}

// BuildCompletionIndex scans the journal directory and aggregates customer/project
// observations. If root is empty, the default $HOME/.tt/journal path is used.
func BuildCompletionIndex(root string) (*CompletionIndex, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".tt", "journal")
	}

	idx := &CompletionIndex{
		Customers: map[string]*CustomerGroup{},
		Projects:  map[string]map[string]*ProjectStats{},
	}

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return idx, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return idx, nil
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable directories/files but continue walking.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		return idx.ingestJournal(path)
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return idx, nil
}

func (idx *CompletionIndex) ingestJournal(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // best-effort: skip unreadable files silently
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Allow long JSON lines (notes can be sizable) by increasing buffer.
	buf := make([]byte, 0, 1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		ts := ev.TS

		rawCustomer := strings.TrimSpace(ev.Customer)
		canonicalCustomer := ""
		if rawCustomer != "" {
			canonicalCustomer = CanonicalCustomer(rawCustomer)
			idx.addCustomerObservation(canonicalCustomer, rawCustomer, ts)
		}

		rawProject := strings.TrimSpace(ev.Project)
		if rawProject != "" {
			idx.addProjectObservation(canonicalCustomer, rawProject, ts)
		}
	}

	return nil
}

func (idx *CompletionIndex) addCustomerObservation(canonical, raw string, ts time.Time) {
	grp, ok := idx.Customers[canonical]
	if !ok {
		grp = &CustomerGroup{
			Canonical: canonical,
			Names:     map[string]*NameStats{},
		}
		idx.Customers[canonical] = grp
	}

	stats, ok := grp.Names[raw]
	if !ok {
		stats = &NameStats{Raw: raw, FirstSeen: ts, LastSeen: ts}
		grp.Names[raw] = stats
	}
	stats.Count++
	if ts.Before(stats.FirstSeen) {
		stats.FirstSeen = ts
	}
	if ts.After(stats.LastSeen) {
		stats.LastSeen = ts
	}

	grp.Total++
	if grp.FirstSeen.IsZero() || ts.Before(grp.FirstSeen) {
		grp.FirstSeen = ts
	}
	if ts.After(grp.LastSeen) {
		grp.LastSeen = ts
	}
}

func (idx *CompletionIndex) addProjectObservation(canonicalCustomer, project string, ts time.Time) {
	customerKey := canonicalCustomer
	if customerKey == "" {
		customerKey = "_uncategorized"
	}
	projMap, ok := idx.Projects[customerKey]
	if !ok {
		projMap = map[string]*ProjectStats{}
		idx.Projects[customerKey] = projMap
	}

	stats, ok := projMap[project]
	if !ok {
		stats = &ProjectStats{Name: project, FirstSeen: ts, LastSeen: ts}
		projMap[project] = stats
	}
	stats.Count++
	if ts.Before(stats.FirstSeen) {
		stats.FirstSeen = ts
	}
	if ts.After(stats.LastSeen) {
		stats.LastSeen = ts
	}
}

// SortedCustomerCanonicals returns canonical customer names ordered lexicographically.
func (idx *CompletionIndex) SortedCustomerCanonicals() []string {
	out := make([]string, 0, len(idx.Customers))
	for k := range idx.Customers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SortedProjects returns sorted project names for the provided canonical customer key.
// If canonicalCustomer is empty, uncategorized projects are returned.
func (idx *CompletionIndex) SortedProjects(canonicalCustomer string) []string {
	key := canonicalCustomer
	if key == "" {
		key = "_uncategorized"
	}
	projMap, ok := idx.Projects[key]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(projMap))
	for name := range projMap {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
