package cmd

import (
	"bufio"
	"encoding/json"
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
}

// customerProjectValidArgs completes first arg as customer and second arg as project.
func customerProjectValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// No args -> complete customer
	if len(args) == 0 {
		custs := uniqueStringsFromJournal("customer")
		return filterPrefixAndSort(custs, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	// One arg -> complete project (prefer projects for given customer)
	if len(args) == 1 {
		cust := strings.TrimSpace(args[0])
		var projs []string
		if cust != "" {
			projs = projectsForCustomer(cust)
		}
		if len(projs) == 0 {
			projs = uniqueStringsFromJournal("project")
		}
		return filterPrefixAndSort(projs, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	// Otherwise, no completion
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// addCmdValidArgs handles completion for `add` which has two required args before
// optional customer/project. We provide completion when args length is 2 or 3:
// args == 2 => completing customer, args == 3 => completing project.
func addCmdValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Before the optional customer (positions: 0=start,1=end), args length 2 means user is completing the customer
	if len(args) <= 2 {
		custs := uniqueStringsFromJournal("customer")
		return filterPrefixAndSort(custs, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	// If user already provided customer (args[2]), complete project
	if len(args) == 3 {
		cust := strings.TrimSpace(args[2])
		var projs []string
		if cust != "" {
			projs = projectsForCustomer(cust)
		}
		if len(projs) == 0 {
			projs = uniqueStringsFromJournal("project")
		}
		return filterPrefixAndSort(projs, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// uniqueStringsFromJournal scans the journal directory and returns unique values
// for the requested field: "customer" or "project".
func uniqueStringsFromJournal(field string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	jroot := filepath.Join(home, ".tt", "journal")
	seen := map[string]struct{}{}

	// Use filepath.Walk and ignore errors so completion remains best-effort.
	_ = filepath.Walk(jroot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if field == "customer" {
				if v := strings.TrimSpace(ev.Customer); v != "" {
					seen[v] = struct{}{}
				}
			} else {
				if v := strings.TrimSpace(ev.Project); v != "" {
					seen[v] = struct{}{}
				}
			}
		}
		return nil
	})

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// projectsForCustomer returns projects that appeared with the given customer.
func projectsForCustomer(customer string) []string {
	if customer == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	jroot := filepath.Join(home, ".tt", "journal")
	seen := map[string]struct{}{}

	_ = filepath.Walk(jroot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if strings.TrimSpace(ev.Customer) == customer {
				if v := strings.TrimSpace(ev.Project); v != "" {
					seen[v] = struct{}{}
				}
			}
		}
		return nil
	})

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
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
