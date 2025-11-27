package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ticketStore map[string]map[string]string

var ticketsCache ticketStore

var (
	ticketListCustomer string
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
		if len(ts) == 0 {
			fmt.Fprintln(out, "no tickets configured")
			return
		}
		var pairs []struct {
			customer string
			project  string
			ticket   string
		}
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
		if len(pairs) == 0 {
			fmt.Fprintln(out, "no tickets configured")
			return
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].customer != pairs[j].customer {
				return pairs[i].customer < pairs[j].customer
			}
			return pairs[i].project < pairs[j].project
		})
		for _, p := range pairs {
			fmt.Fprintf(out, "%s / %s: %s\n", p.customer, p.project, p.ticket)
		}
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
