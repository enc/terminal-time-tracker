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

## TUI (experimental)

Launch the terminal UI built with Bubble Tea:

```bash
./tt tui
```

What you'll see:
- A live clock, lipgloss-styled header/footer/sections, and auto-refresh when journal files change (watches ~/.tt/journal).
- Active session (if running) and the most recent closed entry from the last 7 days.

Keybindings:
- space: start/stop current timer (uses last entry context if no active session)
- n: enter note mode; type to edit; Enter to save; Esc to cancel
- s: open start/switch form (↑/↓ select, Enter apply, b toggle billable, Esc cancel)
- q, Esc, Ctrl-C: quit

Notes:
- The dashboard updates every second and also refreshes immediately on file changes.
- Styling uses lipgloss; for best results, use a terminal with truecolor support and appropriate background theme.

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

- Alias-aware completion: `tt` supports named aliases (presets) which are persisted in `~/.tt/config.yaml`. Completion is aware of aliases in two ways:
  - The `--alias` flag itself supports completion and will suggest defined alias names when you press TAB (e.g., `tt start --alias <TAB>`).
  - When completing the positional `customer` or `project` arguments for `start` and `switch`, if you provide an `--alias <name>` and that alias contains a `customer` and/or `project`, those alias-provided values are included among the completion candidates (and are suggested first). This makes it convenient to use an alias while still being able to tab-complete or confirm the customer/project values the alias represents.

Examples
```bash
# Define an alias (persists in ~/.tt/config.yaml)
tt alias set dev --customer Acme --project Website --activity dev

# The --alias flag will complete existing alias names:
tt start --alias <TAB>   # suggests: dev ...

# When completing the customer argument, the alias' customer is included:
tt start --alias dev <TAB>    # suggests: Acme ... (and other customers from your journal)

# When completing the project argument, the alias' project is preferred when the customer matches:
tt start Acme <TAB>           # suggests: Website ... (and other projects seen for Acme)
tt start --alias dev <TAB>    # if you omit customer, alias' customer/project are included in suggestions
```

Note: alias-aware completion works once you have installed the shell completion script for your shell (see earlier examples for Zsh/Bash/Fish/PowerShell). The completion logic is best-effort and safe: it will not remove other suggestions from the list; it only ensures alias-provided values appear among the candidates (often at the front) so they are easy to select.

## Security / Safety
- Generated completion scripts are just shell scripts. Inspect them if you are concerned before sourcing or installing.
- The `--install-zsh` helper modifies `~/.zshrc` only after asking for confirmation; it appends a clearly marked block so you can easily remove it later.

---

If you'd like, I can:
- Add a non-interactive `--yes` switch for `--install-zsh` (useful for scripted installs).
- Add automated installers for Bash/Fish/PowerShell as well (these require careful behavior around system vs user locations).
- Add caching to completion suggestions to improve performance on very large journals.
- Add a `customer-merge` helper and documentation for managing customer canonicalization (see below).

Happy to proceed with any of the above — tell me which option you prefer.

## customer-merge: non-destructive customer normalization

`tt` includes a non-destructive helper, `tt customer-merge`, which lets you consolidate multiple customer name variants into a single canonical name without rewriting journal files. Instead of editing old JSONL lines (which would require recomputing hashes and anchors), the command writes append-only `amend` events that set the canonical customer for the targeted entries. The journal parser understands `amend` events, so subsequent `tt ls`, `tt report`, and other viewers will show the canonical name.

Quick summary
- Command: `tt customer-merge [--targets ids] --to <canonical> [--note "<text>"] [--dry-run]`
- Selection:
  - `--targets a,b,c` — explicit list of entry IDs to amend.
  - `--since <time> --from "Old A,Old B"` — find entries since the given time whose customer matches any of the `--from` names and amend them.
- Safety:
  - Default: `--dry-run=true` (prints a plan and does not write). Use `--dry-run=false` to actually write amend events.
  - The command appends `amend` events (Type=`amend`, Ref=`<entry-id>`, Customer=`<canonical>`) using the same write path as interactive `amend` so the operation is append-only and auditable.
- Mapping persistence:
  - When the command discovers or is told source names (via `--from`) it will persist a mapping under `customers.map` in your `~/.tt/config.yaml`. This mapping is used by completion helpers and reporting code to hide merged source variants and prefer the canonical name.

Example usage
- Dry-run explicit targets:
  ```bash
  tt customer-merge --targets a1,b2 --to "Acme" --note "merge variants"
  ```
- Apply to explicit targets:
  ```bash
  tt customer-merge --targets a1,b2 --to "Acme" --dry-run=false
  ```
- Merge discovered sources since a date:
  ```bash
  tt customer-merge --since 2025-10-01 --from "ACME Corp,Acme, Inc" --to "Acme" --dry-run=false
  ```

Config example (`~/.tt/config.yaml`)
```yaml
customers:
  map:
    "ACME Corp": "Acme"
    "Acme, Inc": "Acme"
    "acme": "Acme"
```

How completion and reporting behave
- After mapping is persisted, the completion helpers will hide merged source names (e.g., `ACME Corp`) and surface the canonical name (e.g., `Acme`) instead. This keeps tab-completion and suggestion lists clean and focused on canonical values.
- Reports and `tt ls` will reflect the canonical name after the amend event is applied (the parser applies `amend` events when reconstructing entries).

Notes & recommendations
- The non-destructive workflow is reversible and auditable (amend events are appended and the original lines remain).
- If you need to permanently rewrite stored journal lines (for export or external tooling), we can implement a separate, careful rewrite workflow that recomputes canonical payload hashes and anchors — but that is more invasive and should be performed with backups and dry-runs.
- If you want, I can add `tt customer-merge list` and `tt customer-merge undo` helpers to manage mappings and revert mapping entries.


## Repairing journal hashes: `tt audit repair`

This repository includes an `audit` command with a new `repair` subcommand that helps you migrate and repair journal files' per-record hashes in a safe, inspectable way.

High level
- `tt audit repair` recomputes canonical hashes for every event in your journal files and proposes rewrites.
- The command defaults to a safe dry-run mode: it writes proposed repairs beside your originals (`<file>.repair` and `<file>.hash.repair`) and prints a small diff preview so you can inspect changes before applying them.
- When you're confident, you can apply changes to originals with `--dry-run=false --apply=true`. The tool makes backups before overwriting.

Why this exists
- Older versions of the writer computed the hash payload using a `map[string]any` which could marshal to JSON with unpredictable key order.
- Newer code uses a canonical struct to guarantee deterministic JSON bytes and stable hashes.
- `tt audit repair` migrates legacy-map hashed records to the canonical format and fixes `prev_hash` propagation so the chain is consistent.

Important flags
- `--dry-run` (default: true) — write proposed files (`.repair` and `.hash.repair`) and do not modify originals.
- `--apply` (must be used with `--dry-run=false`) — perform destructive rewrite: original file is renamed to `<file>.bak`, the repaired content is written to the original path, and the anchor (`.hash`) is updated (the original anchor is backed up as `.hash.bak` if present).

Recommended safe workflow
1. Verify current journals:
   - `tt audit verify`
   - Fix any obvious parse errors or unrelated issues first.
2. Run repair in dry-run mode (default):
   - `tt audit repair`
     This will create `<file>.repair` and `<file>.hash.repair` beside each file that would be changed, and print a brief inline preview of the first differences.
3. Inspect proposed changes:
   - Open the `.repair` file(s) and compare them with the originals:
     - `diff -u ~/.tt/journal/2025/10/2025-10-07.jsonl ~/.tt/journal/2025/10/2025-10-07.jsonl.repair`
     - Or use the inline preview already printed by the command.
4. Optionally run the small check helper (or the `verify` command) against the `.repair` files to ensure the rewritten chain is valid.
5. When satisfied, apply the repairs (destructive):
   - `tt audit repair --dry-run=false --apply=true`
   - Originals are moved to `<file>.bak` and anchors to `.hash.bak` (if present). The new files and anchors are written in place.
6. Re-run `tt audit verify` to confirm the repository is consistent after applying repairs.

Behavior notes / edge cases
- If a record's stored hash matches the legacy-map computation, the repair migrates it to canonical (updates `prev_hash` and `hash` accordingly).
- If a record's stored hash matches neither legacy nor canonical, the repair will propose a canonical rewrite (it logs a warning for manual inspection).
- Blank lines are preserved in repairs.
- The repair process is deterministic: once applied, future runs should show no changes for repaired files.
- The tool writes a small preview of up to the first changed lines; always inspect `.repair` files before applying to avoid surprises.

Safety & backups
- Dry-run mode is non-destructive: it only writes `.repair` sidecar files.
- Apply mode creates backups (`.bak`) of both the original journal file and the `.hash` anchor (if present). It attempts to restore from the backup if a write fails.
- Nevertheless, keep an independent backup of your `~/.tt` directory if your journal data is critical — applying repairs changes on-disk hashes and should be done deliberately.

Examples
- Dry-run (inspect what would change):
  ```bash
  tt audit repair
  ls -l ~/.tt/journal/**/2025-10-07.jsonl*
  diff -u ~/.tt/journal/2025/10/2025-10-07.jsonl ~/.tt/journal/2025/10/2025-10-07.jsonl.repair
  ```
- Apply repairs (with backup):
  ```bash
  tt audit repair --dry-run=false --apply=true
  # originals moved to .bak, anchors to .hash.bak if present
  tt audit verify
  ```

If you'd like, I can:
- Add an interactive confirmation prompt before apply,
- Add a `--backup-dir` option so all backups/repairs are gathered in one place,
- Add unit tests and CI checks for the repair/migration behavior.

Tell me which of the above you prefer and I’ll implement it.
