package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ticketStore map[string]map[string]string
type observedProject struct {
	Customer string
	Project  string
	LastSeen time.Time
}
type ticketEntry struct {
	Customer string
	Project  string
	Ticket   string
}

var ticketsCache ticketStore

// function hooks for testability
var (
	setTicketFn        = setTicket
	deleteTicketFn     = deleteTicket
	loadTicketsFn      = loadTickets
	observedProjectsFn = loadObservedProjects
)

var (
	ticketListCustomer string
	ticketListMissing  bool
)

var ticketCmd = &cobra.Command{
	Use:   "ticket",
	Short: "Manage project ticket numbers (e.g., ABC-123)",
}

var ticketListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ticket numbers",
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		ts := loadTickets()
		observed := observedProjectsFn()
		showMissing := ticketListMissing

		var pairs []struct {
			customer string
			project  string
			ticket   string
		}
		if showMissing {
			missing := missingTicketEntries(observed, ts)
			for _, m := range missing {
				if ticketListCustomer != "" && !strings.EqualFold(ticketListCustomer, m.Customer) {
					continue
				}
				pairs = append(pairs, struct {
					customer string
					project  string
					ticket   string
				}{m.Customer, m.Project, ""})
			}
		} else {
			for cust, pm := range ts {
				if ticketListCustomer != "" && !strings.EqualFold(ticketListCustomer, cust) {
					continue
				}
				for proj, t := range pm {
					pairs = append(pairs, struct {
						customer string
						project  string
						ticket   string
					}{cust, proj, t})
				}
			}
		}
		if len(pairs) == 0 {
			if showMissing {
				fmt.Fprintln(out, "no missing tickets found")
			} else {
				fmt.Fprintln(out, "no tickets configured")
			}
			return
		}

		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		if showMissing {
			fmt.Fprintln(tw, "Customer\tProject\tTicket?")
		} else {
			fmt.Fprintln(tw, "Customer\tProject\tTicket")
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].customer != pairs[j].customer {
				return pairs[i].customer < pairs[j].customer
			}
			return pairs[i].project < pairs[j].project
		})
		for _, p := range pairs {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", p.customer, p.project, p.ticket)
		}
		_ = tw.Flush()
	},
}

var ticketSetCmd = &cobra.Command{
	Use:   "set <customer> <project> <ticket>",
	Short: "Set or update a ticket number for a project",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		customer := args[0]
		project := args[1]
		ticket := args[2]
		if err := setTicket(customer, project, ticket); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("ticket for %s / %s set to %s\n", customer, project, ticket)
	},
}

var ticketRmCmd = &cobra.Command{
	Use:   "rm <customer> <project>",
	Short: "Remove a ticket number from a project",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		customer := args[0]
		project := args[1]
		if err := deleteTicket(customer, project); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Printf("ticket for %s / %s removed\n", customer, project)
	},
}

func init() {
	rootCmd.AddCommand(ticketCmd)
	ticketCmd.AddCommand(ticketListCmd, ticketSetCmd, ticketRmCmd)

	ticketListCmd.Flags().StringVar(&ticketListCustomer, "customer", "", "filter by customer (case-insensitive)")
	ticketListCmd.Flags().BoolVar(&ticketListMissing, "missing", false, "show only projects seen in journals that have no ticket yet")
}

func loadTickets() ticketStore {
	if ticketsCache != nil {
		return cloneTickets(ticketsCache)
	}

	raw := viper.GetStringMap("tickets")
	if len(raw) == 0 {
		cf := configFilePath()
		if _, err := os.Stat(cf); err == nil {
			viper.SetConfigFile(cf)
			viper.SetConfigType("yaml")
			_ = viper.ReadInConfig()
			raw = viper.GetStringMap("tickets")
		}
	}
	store := ticketStore{}
	for cust, v := range raw {
		if pm, ok := v.(map[string]any); ok {
			for proj, tv := range pm {
				if s, ok := tv.(string); ok && strings.TrimSpace(s) != "" {
					if _, exists := store[cust]; !exists {
						store[cust] = map[string]string{}
					}
					store[cust][proj] = s
				}
			}
		}
	}

	ticketsCache = cloneTickets(store)
	return store
}

func cloneTickets(src ticketStore) ticketStore {
	out := ticketStore{}
	for c, pm := range src {
		out[c] = map[string]string{}
		for p, t := range pm {
			out[c][p] = t
		}
	}
	return out
}

func getTicket(customer, project string) (string, bool) {
	store := loadTickets()
	custNorm := strings.ToLower(strings.TrimSpace(customer))
	projNorm := strings.ToLower(strings.TrimSpace(project))
	for cust, pm := range store {
		if strings.ToLower(strings.TrimSpace(cust)) != custNorm {
			continue
		}
		for proj, t := range pm {
			if strings.ToLower(strings.TrimSpace(proj)) == projNorm {
				return t, true
			}
		}
	}
	return "", false
}

func setTicket(customer, project, ticket string) error {
	store := loadTickets()
	cust := strings.TrimSpace(customer)
	proj := strings.TrimSpace(project)
	if _, ok := store[cust]; !ok {
		store[cust] = map[string]string{}
	}
	store[cust][proj] = ticket
	ticketsCache = cloneTickets(store)
	viper.Set("tickets", store)
	return saveViperConfig()
}

func deleteTicket(customer, project string) error {
	store := loadTickets()
	custNorm := strings.ToLower(strings.TrimSpace(customer))
	projNorm := strings.ToLower(strings.TrimSpace(project))
	foundCust := ""
	for cust := range store {
		if strings.ToLower(strings.TrimSpace(cust)) == custNorm {
			foundCust = cust
			break
		}
	}
	if foundCust == "" {
		return fmt.Errorf("no tickets found for customer %q", customer)
	}
	pm := store[foundCust]
	foundProj := ""
	for proj := range pm {
		if strings.ToLower(strings.TrimSpace(proj)) == projNorm {
			foundProj = proj
			break
		}
	}
	if foundProj == "" {
		return fmt.Errorf("ticket for %s / %s not found", customer, project)
	}
	delete(pm, foundProj)
	if len(pm) == 0 {
		delete(store, foundCust)
	} else {
		store[foundCust] = pm
	}
	ticketsCache = cloneTickets(store)
	viper.Set("tickets", store)
	return saveViperConfig()
}

func loadObservedProjects() []observedProject {
	idx, err := BuildCompletionIndex("")
	if err != nil || idx == nil {
		return nil
	}
	var res []observedProject
	for canonicalCust, grp := range idx.Customers {
		displayCust := canonicalCust
		// pick most frequent raw name for display
		var bestName string
		var bestCount int
		for _, ns := range grp.Names {
			if ns.Count > bestCount {
				bestCount = ns.Count
				bestName = ns.Raw
			}
		}
		if bestName != "" {
			displayCust = bestName
		}
		projs := idx.Projects[canonicalCust]
		for _, ps := range projs {
			res = append(res, observedProject{
				Customer: displayCust,
				Project:  ps.Name,
				LastSeen: ps.LastSeen,
			})
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].LastSeen.After(res[j].LastSeen)
	})
	return res
}

func missingTicketEntries(observed []observedProject, store ticketStore) []ticketEntry {
	var out []ticketEntry
	for _, op := range observed {
		if hasTicketInStore(store, op.Customer, op.Project) {
			continue
		}
		out = append(out, ticketEntry{Customer: op.Customer, Project: op.Project})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Customer != out[j].Customer {
			return out[i].Customer < out[j].Customer
		}
		return out[i].Project < out[j].Project
	})
	return out
}

func hasTicketInStore(store ticketStore, customer, project string) bool {
	custNorm := strings.ToLower(strings.TrimSpace(customer))
	projNorm := strings.ToLower(strings.TrimSpace(project))
	for cust, pm := range store {
		if strings.ToLower(strings.TrimSpace(cust)) != custNorm {
			continue
		}
		for proj := range pm {
			if strings.ToLower(strings.TrimSpace(proj)) == projNorm {
				return true
			}
		}
	}
	return false
}
