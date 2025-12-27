package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var deprecateActive bool

var deprecateCmd = &cobra.Command{
	Use:   "deprecate",
	Short: "Manage deprecated customers and projects",
}

var deprecateSetCmd = &cobra.Command{
	Use:   "set <customer> [project]",
	Short: "Deprecate a customer (and all projects) or a specific project",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		customer := strings.TrimSpace(args[0])
		if customer == "" {
			return fmt.Errorf("customer is required")
		}
		project := ""
		if len(args) > 1 {
			project = strings.TrimSpace(args[1])
		}

		store, err := loadDeprecations()
		if err != nil {
			return err
		}

		if project == "" {
			if deprecateActive {
				store.removeCustomer(customer)
			} else {
				store.setCustomer(customer)
			}
		} else {
			if deprecateActive {
				store.removeProject(customer, project)
			} else {
				store.setProject(customer, project)
			}
		}

		if err := saveDeprecations(store); err != nil {
			return err
		}

		switch {
		case project == "" && deprecateActive:
			fmt.Printf("customer %q marked active\n", customer)
		case project == "" && !deprecateActive:
			fmt.Printf("customer %q deprecated (all projects)\n", customer)
		case project != "" && deprecateActive:
			fmt.Printf("project %q for customer %q marked active\n", project, customer)
		default:
			fmt.Printf("project %q for customer %q deprecated\n", project, customer)
		}
		return nil
	},
}

var deprecateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deprecated customers and projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadDeprecations()
		if err != nil {
			return err
		}

		customers := sortedMapValues(store.customers)
		type projRow struct {
			Customer string
			Project  string
		}
		var projects []projRow
		for custKey, projMap := range store.projects {
			customer := store.displayCustomerName(custKey)
			for _, project := range sortedMapValues(projMap) {
				projects = append(projects, projRow{Customer: customer, Project: project})
			}
		}
		sort.Slice(projects, func(i, j int) bool {
			if projects[i].Customer == projects[j].Customer {
				return projects[i].Project < projects[j].Project
			}
			return projects[i].Customer < projects[j].Customer
		})

		if len(customers) == 0 && len(projects) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no deprecated customers or projects")
			return nil
		}

		out := cmd.OutOrStdout()
		if len(customers) > 0 {
			fmt.Fprintln(out, "Deprecated customers:")
			for _, c := range customers {
				fmt.Fprintf(out, "  - %s\n", c)
			}
		}
		if len(projects) > 0 {
			if len(customers) > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, "Deprecated projects:")
			for _, p := range projects {
				fmt.Fprintf(out, "  - %s / %s\n", p.Customer, p.Project)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deprecateCmd)
	deprecateCmd.AddCommand(deprecateSetCmd, deprecateListCmd)

	deprecateSetCmd.Flags().BoolVar(&deprecateActive, "active", false, "mark customer/project active (remove from deprecated list)")
	deprecateSetCmd.ValidArgsFunction = customerProjectValidArgs
}
