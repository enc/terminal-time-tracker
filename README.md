# tt — CLI Time Tracker (skeleton)

A minimal, ready-to-run Go skeleton implementing the core commands:

- `tt start [customer] [project]` (with flags: `-a/--activity`, `-b/--billable`, `-t/--tag`, `-n/--note`)
- `tt stop`
- `tt switch [customer] [project]` (stops current, starts new)
- `tt note <text>` (adds a note to the current running entry)
- `tt add <start> <end> [customer] [project]` (retro-add, ISO8601 or 'YYYY-MM-DDTHH:MM')
- `tt ls [--today|--range A..B]`
- `tt report [--today|--week|--range A..B] [--by fields]`
- `tt audit verify`
- `tt tempo day` (consolidated day view helpers)
- `tt completion` (generate shell completion; see below)

Data is stored locally in JSONL journals under `~/.tt/journal/YYYY/MM/YYYY-MM-DD.jsonl`.
Config in `~/.tt/config.yaml`.

> This is a starter; extend as needed (export to XLSX, rate cards per activity, etc.).

## Build & Run
```bash
cd tt-cli
go build -o tt .
./tt --help
```

## Quick demo
```bash
./tt start acme mobilize:foundation -a design -n "Subnet layout"
sleep 2
./tt note "Chose /20 per AZ"
./tt stop
./tt ls --today
./tt report --today --by customer,project,activity
./tt audit verify
```

### Consolidated day view

Quickly review your day and get a one-liner to book:

```bash
./tt tempo day --today
# or a specific date:
./tt tempo day --date 2025-10-07 --group-by activity --round rounded --issue ACME-123
```

This prints a compact table (per activity/project/customer) with raw vs rounded totals, examples, and the exact `tt tempo book` command to execute.

---

## Shell completion

`tt` provides a `completion` command to generate shell completion scripts for several shells. Completion scripts are generated using Cobra's built-in generators. The command also exposes a convenience installer for Zsh.

General usage:
```bash
# Generate completion script for a specific shell and print to stdout
./tt completion zsh
./tt completion bash
./tt completion fish
./tt completion powershell
```

Below are recommended ways to use and install completions for each shell.

### Zsh (recommended for interactive users)
- Quick trial (current shell):
  ```bash
  source <(./tt completion zsh)
  ```
- Persistent install (manual):
  1. Create a functions directory (if you don't already use one):
     ```bash
     mkdir -p ~/.zfunc
     ```
  2. Write completion file:
     ```bash
     ./tt completion zsh > ~/.zfunc/_tt
     ```
  3. Ensure `~/.zfunc` is in your `fpath` and `compinit` is initialized (add to `~/.zshrc` if needed):
     ```sh
     fpath=(~/.zfunc $fpath)
     autoload -Uz compinit && compinit
     ```
  4. Restart zsh or run `exec zsh`.

- Automated install (convenience):
  `tt` supports an automated interactive installer:
  ```bash
  ./tt completion --install-zsh
  ```
  What it does:
  - Generates the zsh completion and writes it to `~/.zfunc/_tt`.
  - Appends lines to `~/.zshrc` to add `~/.zfunc` to `fpath` and to run `compinit` if those are not already present.
  - Requires explicit confirmation (`yes`/`y`) before making changes.
  - The installer adds a clearly marked block to `~/.zshrc`, so you can remove it later if desired.
  - Note: This is interactive by design to avoid accidental changes to your shell config.

### Bash
- One-off in current session:
  ```bash
  source <(./tt completion bash)
  ```
- Persistent install (user-level):
  ```bash
  ./tt completion bash > ~/.tt_completion.sh
  # then add to ~/.bashrc:
  echo 'source ~/.tt_completion.sh' >> ~/.bashrc
  ```
- System-wide (requires admin privileges) you can write to:
  - `/etc/bash_completion.d/tt` (Debian/Ubuntu) or equivalent on other distros.
  - After installing system-wide, open a new shell session or run `source /etc/bash_completion` as appropriate.

### Fish
- One-off:
  ```bash
  ./tt completion fish | source
  ```
- Persistent install:
  ```bash
  ./tt completion fish > ~/.config/fish/completions/tt.fish
  ```
  Fish will automatically load `~/.config/fish/completions/tt.fish` for interactive completion.

### PowerShell
- Generate and add to your profile (PowerShell Core / Windows PowerShell):
  ```powershell
  ./tt completion powershell | Out-File -Encoding utf8 $PROFILE\tt-completion.ps1
  # Then add to your profile (or dot-source the file):
  . $PROFILE\tt-completion.ps1
  ```
- Alternatively, write the script to a file and import it from your PowerShell profile so completion is available in new sessions.

## Troubleshooting & notes

- Restart your shell after installing completions so the new completion scripts are discovered.
- For Zsh, ensure `fpath` contains the directory where `_tt` was written and that `compinit` has run; otherwise completions won't be picked up.
- If completions do not appear:
  - Confirm `./tt completion zsh` (or the appropriate shell) prints a script to stdout.
  - Check permissions on the installed file (should be readable).
  - Make sure you have not disabled completion initialization in your shell config.
- The Zsh automated installer adds lines to `~/.zshrc` only when it detects relevant lines are missing; it always asks for explicit confirmation before making changes.
- The dynamic positional completion (customer/project suggestions) scans your journal files under `~/.tt/journal`. It's best-effort and tolerant of missing or unreadable files. If your journal is large and you notice latency in completion, consider manual installation of the completion script (the scanning only happens inside the shell completion runtime).

## Security / Safety
- Generated completion scripts are just shell scripts. Inspect them if you are concerned before sourcing or installing.
- The `--install-zsh` helper modifies `~/.zshrc` only after asking for confirmation; it appends a clearly marked block so you can easily remove it later.

---

If you'd like, I can:
- Add a non-interactive `--yes` switch for `--install-zsh` (useful for scripted installs).
- Add automated installers for Bash/Fish/PowerShell as well (these require careful behavior around system vs user locations).
- Add caching to completion suggestions to improve performance on very large journals.

Happy to proceed with any of the above — tell me which option you prefer.
