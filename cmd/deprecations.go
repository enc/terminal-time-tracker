package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type deprecationStore struct {
	customers       map[string]string            // canonical customer -> display
	projects        map[string]map[string]string // canonical customer -> normalized project -> display
	customerDisplay map[string]string            // canonical customer -> display (for project-only entries)
}

type deprecatedFile struct {
	Customers []string            `yaml:"customers"`
	Projects  map[string][]string `yaml:"projects"`
}

var deprecationsCache *deprecationStore

func newDeprecationStore() deprecationStore {
	return deprecationStore{
		customers:       map[string]string{},
		projects:        map[string]map[string]string{},
		customerDisplay: map[string]string{},
	}
}

func cloneDeprecationStore(in deprecationStore) deprecationStore {
	out := newDeprecationStore()
	for k, v := range in.customers {
		out.customers[k] = v
	}
	for k, v := range in.customerDisplay {
		out.customerDisplay[k] = v
	}
	for cust, projects := range in.projects {
		if out.projects[cust] == nil {
			out.projects[cust] = map[string]string{}
		}
		for pk, pv := range projects {
			out.projects[cust][pk] = pv
		}
	}
	return out
}

func deprecatedFilePath() string {
	base := configFilePath()
	dir := filepath.Dir(base)
	return filepath.Join(dir, "deprecated.yaml")
}

func loadDeprecations() (deprecationStore, error) {
	if deprecationsCache != nil {
		return cloneDeprecationStore(*deprecationsCache), nil
	}

	store := newDeprecationStore()
	path := deprecatedFilePath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			deprecationsCache = &store
			return cloneDeprecationStore(store), nil
		}
		return store, err
	}

	var raw deprecatedFile
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return store, err
	}

	for _, customer := range raw.Customers {
		store.setCustomer(customer)
	}
	for customer, projects := range raw.Projects {
		for _, project := range projects {
			store.setProject(customer, project)
		}
	}

	deprecationsCache = &store
	return cloneDeprecationStore(store), nil
}

func saveDeprecations(store deprecationStore) error {
	raw := deprecatedFile{
		Customers: sortedMapValues(store.customers),
		Projects:  map[string][]string{},
	}
	for custKey, projects := range store.projects {
		if len(projects) == 0 {
			continue
		}
		display := store.displayCustomerName(custKey)
		if display == "" {
			display = custKey
		}
		raw.Projects[display] = sortedMapValues(projects)
	}

	b, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	path := deprecatedFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	deprecationsCache = &store
	return nil
}

func (d *deprecationStore) setCustomer(name string) {
	key := canonicalCustomerKey(name)
	if key == "" {
		return
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = key
	}
	d.customers[key] = display
	if _, ok := d.customerDisplay[key]; !ok {
		d.customerDisplay[key] = display
	}
}

func (d *deprecationStore) removeCustomer(name string) {
	key := canonicalCustomerKey(name)
	if key == "" {
		return
	}
	delete(d.customers, key)
}

func (d *deprecationStore) setProject(customer, project string) {
	custKey := canonicalCustomerKey(customer)
	if custKey == "" {
		return
	}
	displayCustomer := strings.TrimSpace(customer)
	if displayCustomer == "" {
		displayCustomer = custKey
	}
	if _, ok := d.customerDisplay[custKey]; !ok {
		d.customerDisplay[custKey] = displayCustomer
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return
	}
	if d.projects[custKey] == nil {
		d.projects[custKey] = map[string]string{}
	}
	d.projects[custKey][normalizeProjectKey(project)] = project
}

func (d *deprecationStore) removeProject(customer, project string) {
	custKey := canonicalCustomerKey(customer)
	if custKey == "" {
		return
	}
	if d.projects[custKey] == nil {
		return
	}
	delete(d.projects[custKey], normalizeProjectKey(project))
	if len(d.projects[custKey]) == 0 {
		delete(d.projects, custKey)
	}
}

func (d deprecationStore) isCustomerDeprecated(name string) bool {
	key := canonicalCustomerKey(name)
	if key == "" {
		return false
	}
	_, ok := d.customers[key]
	return ok
}

func (d deprecationStore) isProjectDeprecated(customer, project string) bool {
	custKey := canonicalCustomerKey(customer)
	if custKey == "" {
		return false
	}
	if d.isCustomerDeprecated(custKey) {
		return true
	}
	projectKey := normalizeProjectKey(project)
	if projectKey == "" {
		return false
	}
	if set := d.projects[custKey]; set != nil {
		if _, ok := set[projectKey]; ok {
			return true
		}
	}
	return false
}

func (d deprecationStore) displayCustomerName(custKey string) string {
	if v, ok := d.customers[custKey]; ok {
		return v
	}
	if v, ok := d.customerDisplay[custKey]; ok {
		return v
	}
	return custKey
}

func canonicalCustomerKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return normalizeCustomerKey(CanonicalCustomer(name))
}

func normalizeProjectKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}

func sortedMapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	seen := map[string]struct{}{}
	for _, v := range m {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
