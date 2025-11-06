package cmd

import (
	"sort"
	"strings"

	"github.com/spf13/viper"
)

type completionDecisions struct {
	allowCustomers  map[string]string
	ignoreCustomers map[string]string

	allowProjects  map[string]map[string]struct{}
	ignoreProjects map[string]map[string]struct{}
}

func loadCompletionDecisions() completionDecisions {
	cd := completionDecisions{
		allowCustomers:  toCustomerSet(viper.GetStringSlice("completion.allow.customers")),
		ignoreCustomers: toCustomerSet(viper.GetStringSlice("completion.ignore.customers")),
		allowProjects:   toNestedSet(viper.GetStringMap("completion.allow.projects")),
		ignoreProjects:  toNestedSet(viper.GetStringMap("completion.ignore.projects")),
	}
	return cd
}

func (c completionDecisions) isCustomerAllowed(name string) bool {
	_, ok := c.allowCustomers[normalizeCustomerKey(name)]
	return ok
}

func (c completionDecisions) isCustomerIgnored(name string) bool {
	_, ok := c.ignoreCustomers[normalizeCustomerKey(name)]
	return ok
}

func (c completionDecisions) allowCustomer(name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	key := normalizeCustomerKey(trimmed)
	delete(c.ignoreCustomers, key)
	c.allowCustomers[key] = trimmed
}

func (c completionDecisions) ignoreCustomer(name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	key := normalizeCustomerKey(trimmed)
	delete(c.allowCustomers, key)
	c.ignoreCustomers[key] = trimmed
}

func (c completionDecisions) allowProject(customer, project string) {
	if project == "" {
		return
	}
	key := canonicalProjectKey(customer)
	ensureSet(c.allowProjects, key)[project] = struct{}{}
	delete(ensureSet(c.ignoreProjects, key), project)
}

func (c completionDecisions) ignoreProject(customer, project string) {
	if project == "" {
		return
	}
	key := canonicalProjectKey(customer)
	ensureSet(c.ignoreProjects, key)[project] = struct{}{}
	delete(ensureSet(c.allowProjects, key), project)
}

func (c completionDecisions) allowedCustomers() []string {
	return sortedValues(c.allowCustomers)
}

func (c completionDecisions) ignoredCustomers() []string {
	return sortedValues(c.ignoreCustomers)
}

func (c completionDecisions) allowedProjects(customer string) []string {
	key := canonicalProjectKey(customer)
	set := c.allowProjects[key]
	if set == nil {
		return nil
	}
	return sortedKeys(set)
}

func (c completionDecisions) ignoredProjects(customer string) []string {
	key := canonicalProjectKey(customer)
	set := c.ignoreProjects[key]
	if set == nil {
		return nil
	}
	return sortedKeys(set)
}

func (c completionDecisions) isProjectAllowed(customer, project string) bool {
	project = strings.TrimSpace(project)
	if project == "" {
		return false
	}
	key := canonicalProjectKey(customer)
	if set := c.allowProjects[key]; set != nil {
		if _, ok := set[project]; ok {
			return ok
		}
	}
	return false
}

func (c completionDecisions) isProjectIgnored(customer, project string) bool {
	project = strings.TrimSpace(project)
	if project == "" {
		return false
	}
	key := canonicalProjectKey(customer)
	if set := c.ignoreProjects[key]; set != nil {
		if _, ok := set[project]; ok {
			return ok
		}
	}
	return false
}

func (c completionDecisions) save() error {
	viper.Set("completion.allow.customers", c.allowedCustomers())
	viper.Set("completion.ignore.customers", c.ignoredCustomers())
	viper.Set("completion.allow.projects", c.projectsForPersistence(c.allowProjects))
	viper.Set("completion.ignore.projects", c.projectsForPersistence(c.ignoreProjects))
	return saveViperConfig()
}

func (c completionDecisions) projectsForPersistence(in map[string]map[string]struct{}) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := map[string][]string{}
	for key, set := range in {
		if len(set) == 0 {
			continue
		}
		display := c.displayNameForKey(key)
		out[display] = sortedKeys(set)
	}
	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedValues(m map[string]string) []string {
	dedup := map[string]struct{}{}
	out := make([]string, 0, len(m))
	for _, v := range m {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := dedup[v]; ok {
			continue
		}
		dedup[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func ensureSet(m map[string]map[string]struct{}, key string) map[string]struct{} {
	if key == "" {
		key = "_uncategorized"
	}
	if m[key] == nil {
		m[key] = map[string]struct{}{}
	}
	return m[key]
}

func canonicalProjectKey(customer string) string {
	if strings.TrimSpace(customer) == "" {
		return "_uncategorized"
	}
	return normalizeCustomerKey(customer)
}

func toNestedSet(raw map[string]any) map[string]map[string]struct{} {
	if raw == nil {
		return map[string]map[string]struct{}{}
	}
	out := map[string]map[string]struct{}{}
	for key, val := range raw {
		values := []string{}
		switch vv := val.(type) {
		case []string:
			values = append(values, vv...)
		case []interface{}:
			for _, item := range vv {
				if s, ok := item.(string); ok && s != "" {
					values = append(values, s)
				}
			}
		case map[string]any:
			// treat nested map[string]any as keys with truthy values
			for k := range vv {
				if strings.TrimSpace(k) != "" {
					values = append(values, k)
				}
			}
		default:
			if s, ok := vv.(string); ok && s != "" {
				values = append(values, s)
			}
		}
		set := map[string]struct{}{}
		for _, item := range values {
			cleaned := strings.TrimSpace(item)
			if cleaned == "" {
				continue
			}
			set[cleaned] = struct{}{}
		}
		if len(set) > 0 {
			out[canonicalProjectKey(key)] = set
		}
	}
	return out
}

func toCustomerSet(items []string) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := normalizeCustomerKey(trimmed)
		out[key] = trimmed
	}
	return out
}

func (c completionDecisions) displayNameForKey(key string) string {
	if key == "_uncategorized" {
		return ""
	}
	if v, ok := c.allowCustomers[key]; ok {
		return v
	}
	if v, ok := c.ignoreCustomers[key]; ok {
		return v
	}
	return key
}

func normalizeCustomerKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}
