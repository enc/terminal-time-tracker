package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var installZsh bool

// completionCmd writes shell completion scripts for supported shells and
// registers dynamic positional completion for customer/project arguments.
// It also supports an automated zsh installation via --install-zsh which will
// write the completion file into ~/.zfunc/_tt and optionally update ~/.zshrc
// after an explicit user confirmation.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If user requested automated zsh installation, perform it (ignores positional arg).
		if installZsh {
			fmt.Println("This command will install zsh completion for `tt` into your home directory:")
			fmt.Println(" - Completion file: ~/.zfunc/_tt")
			fmt.Println(" - It will also attempt to update ~/.zshrc to add ~/.zfunc to your fpath and ensure compinit is run.")
			fmt.Print("Proceed with automatic installation? (yes/no): ")
			reader := bufio.NewReader(os.Stdin)
			resp, _ := reader.ReadString('\n')
			resp = strings.TrimSpace(strings.ToLower(resp))
			if resp != "yes" && resp != "y" {
				fmt.Println("Aborted by user.")
				return nil
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			zfunc := filepath.Join(home, ".zfunc")
			if err := os.MkdirAll(zfunc, 0o755); err != nil {
				return fmt.Errorf("cannot create %s: %w", zfunc, err)
			}
			dest := filepath.Join(zfunc, "_tt")
			f, err := os.Create(dest)
			if err != nil {
				return fmt.Errorf("cannot create completion file %s: %w", dest, err)
			}
			if err := rootCmd.GenZshCompletion(f); err != nil {
				f.Close()
				return fmt.Errorf("failed to generate zsh completion: %w", err)
			}
			f.Close()
			fmt.Printf("Wrote zsh completion to %s\n", dest)

			// Update ~/.zshrc if needed
			zshrc := filepath.Join(home, ".zshrc")
			content, _ := os.ReadFile(zshrc)
			s := string(content)

			// Determine if we need to add fpath line and/or compinit call.
			needFpath := !strings.Contains(s, ".zfunc")
			needCompinit := !strings.Contains(s, "compinit")

			appendLines := ""
			if needFpath {
				appendLines += "\n# tt: ensure completion functions directory is in fpath\nfpath=(~/.zfunc $fpath)\n"
			}
			if needCompinit {
				appendLines += "\n# tt: ensure compinit is initialized for completion\nautoload -Uz compinit && compinit\n"
			}

			if appendLines != "" {
				fz, err := os.OpenFile(zshrc, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if err != nil {
					return fmt.Errorf("failed to open %s for writing: %w", zshrc, err)
				}
				if _, err := fz.WriteString("\n# --- added by `tt completion --install-zsh` ---\n" + appendLines + "# --- end tt changes ---\n"); err != nil {
					fz.Close()
					return fmt.Errorf("failed to update %s: %w", zshrc, err)
				}
				fz.Close()
				fmt.Printf("Updated %s\n", zshrc)
			} else {
				fmt.Printf("%s already looks configured; no changes made.\n", zshrc)
			}
			fmt.Println("Installation complete. Restart zsh or run `exec zsh` to enable completion.")
			return nil
		}

		// If the first positional matches a subcommand (e.g. "review"), delegate to it.
		if len(args) > 0 {
			name := strings.ToLower(strings.TrimSpace(args[0]))
			if sub := cmd.Commands(); len(sub) > 0 {
				for _, child := range sub {
					if child.Name() == name || child.HasAlias(name) {
						child.SetArgs(args[1:])
						child.SetContext(cmd.Context())
						return child.Execute()
					}
				}
			}
		}

		// Normal behavior: require a shell argument
		if len(args) == 0 {
			return fmt.Errorf("missing shell argument; expected one of: bash, zsh, fish, powershell (or use --install-zsh)")
		}
		shell := args[0]
		switch shell {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			// With descriptions if supported by cobra version
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", shell)
		}
	},
}

func init() {
	// register the --install-zsh flag (explicit user agreement required)
	completionCmd.Flags().BoolVar(&installZsh, "install-zsh", false, "Install zsh completion into ~/.zfunc/_tt and update ~/.zshrc (requires confirmation)")

	rootCmd.AddCommand(completionCmd)

	// Register dynamic completion for commands that accept customer/project as
	// positional arguments.
	startCmd.ValidArgsFunction = customerProjectValidArgs
	switchCmd.ValidArgsFunction = customerProjectValidArgs
	addCmd.ValidArgsFunction = addCmdValidArgs

	// Register flag completion for --alias to suggest existing alias names.
	// We ignore the returned error as registration should succeed in normal cobra usage;
	// if the flag does not exist, registering will be a no-op.
	_ = startCmd.RegisterFlagCompletionFunc("alias", aliasFlagCompletion)
	_ = switchCmd.RegisterFlagCompletionFunc("alias", aliasFlagCompletion)
}

// customerProjectValidArgs completes first arg as customer and second arg as project.
// When an --alias flag is present, suggestions will include the alias' customer/project
// where appropriate so users can rely on alias-injected values for completion.
func customerProjectValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	decisions := loadCompletionDecisions()

	var aliasName string
	if cmd != nil {
		if f := cmd.Flags(); f != nil {
			if v, err := f.GetString("alias"); err == nil {
				aliasName = strings.TrimSpace(v)
			}
		}
	}

	var aliasCustomer, aliasProject string
	if aliasName != "" {
		if a, ok := getAlias(aliasName); ok {
			aliasCustomer = strings.TrimSpace(a.Customer)
			aliasProject = strings.TrimSpace(a.Project)
		}
	}

	aliasCanonical := canonicalForCompletion(aliasCustomer)

	if len(args) == 0 {
		custs := customerCompletionList(decisions, aliasCanonical, toComplete)
		return custs, cobra.ShellCompDirectiveNoFileComp
	}

	if len(args) == 1 {
		inputCustomer := strings.TrimSpace(args[0])
		canonicalCustomer := canonicalForCompletion(inputCustomer)
		if canonicalCustomer == "" && inputCustomer == "" {
			canonicalCustomer = aliasCanonical
		}
		projs := projectCompletionList(decisions, canonicalCustomer, aliasProject, toComplete)
		return projs, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}

// addCmdValidArgs handles completion for `add` which has two required args before
// optional customer/project. We provide completion when args length is 2 or 3:
// args == 2 => completing customer, args == 3 => completing project.
func addCmdValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	decisions := loadCompletionDecisions()

	if len(args) <= 2 {
		custs := customerCompletionList(decisions, "", toComplete)
		return custs, cobra.ShellCompDirectiveNoFileComp
	}

	if len(args) == 3 {
		cust := canonicalForCompletion(strings.TrimSpace(args[2]))
		projs := projectCompletionList(decisions, cust, "", toComplete)
		return projs, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}

func canonicalForCompletion(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return CanonicalCustomer(name)
}

func customerCompletionList(decisions completionDecisions, aliasCustomer, prefix string) []string {
	entries := map[string]string{}
	for _, name := range decisions.allowedCustomers() {
		canonical := canonicalForCompletion(name)
		if canonical == "" {
			continue
		}
		key := strings.ToLower(canonical)
		if _, ok := entries[key]; ok {
			continue
		}
		entries[key] = canonical
	}

	base := make([]string, 0, len(entries))
	for _, v := range entries {
		base = append(base, v)
	}

	filtered := filterPrefixAndSort(base, prefix)
	return insertAliasCustomer(filtered, aliasCustomer, prefix)
}

func projectCompletionList(decisions completionDecisions, customer, aliasProject, prefix string) []string {
	seen := map[string]struct{}{}
	base := []string{}

	appendProjects := func(list []string) {
		for _, name := range list {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			base = append(base, trimmed)
		}
	}

	appendProjects(decisions.allowedProjects(customer))
	appendProjects(decisions.allowedProjects(""))

	filtered := filterPrefixAndSort(base, prefix)
	return insertAliasProject(filtered, aliasProject, prefix)
}

func insertAliasCustomer(list []string, aliasCustomer, prefix string) []string {
	canonical := canonicalForCompletion(aliasCustomer)
	if canonical == "" {
		return list
	}
	if prefix != "" && !strings.HasPrefix(strings.ToLower(canonical), strings.ToLower(prefix)) {
		return list
	}
	out := []string{canonical}
	for _, item := range list {
		if strings.EqualFold(item, canonical) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func insertAliasProject(list []string, aliasProject, prefix string) []string {
	alias := strings.TrimSpace(aliasProject)
	if alias == "" {
		return list
	}
	if prefix != "" && !strings.HasPrefix(strings.ToLower(alias), strings.ToLower(prefix)) {
		return list
	}
	out := []string{alias}
	for _, item := range list {
		if strings.EqualFold(item, alias) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// filterPrefixAndSort filters the provided list by the prefix (case-insensitive)
// and returns a sorted result.
func filterPrefixAndSort(list []string, prefix string) []string {
	if prefix == "" {
		sort.Strings(list)
		return list
	}
	lower := strings.ToLower(prefix)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if strings.HasPrefix(strings.ToLower(item), lower) {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

// aliasFlagCompletion provides shell completion suggestions for the --alias flag.
// It returns the list of defined alias names filtered by the current prefix.
func aliasFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// gather names
	m := loadAliases()
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return filterPrefixAndSort(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}
