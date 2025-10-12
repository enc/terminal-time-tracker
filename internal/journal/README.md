# tt — a fast, local, billing‑friendly CLI time tracker

tt helps you track time from the terminal with near-zero ceremony. It stores data locally as append-only JSONL and keeps a per-day hash chain you can verify and repair. Reporting is designed to be billing-friendly (grouping, rounding, weekly summaries, and export helpers).

Highlights
- Start/stop and notes in one line; switch context instantly
- Retro-add entries with flexible time parsing
- List and report with billable-ready summaries
- Weekly report with Markdown table output and (optional) Tempo export
- Local, append-only JSONL with per-day hash anchors
- Shell completion with dynamic suggestions (customers/projects) and an optional zsh auto-installer
- Optional TUI (Bubble Tea) for a live dashboard

---

## Install

Prerequisites
- Go (version declared in this repo’s go.mod; currently 1.24+)
- macOS, Linux, or Windows

From source
1) Clone this repository
2) Build and place `tt` on your PATH:
   - macOS/Linux:
     - go build -o tt .
     - mv ./tt /usr/local/bin/ (or another directory on your PATH)
   - Windows (PowerShell):
     - go build -o tt.exe .
     - Move tt.exe into a directory on your PATH

Check:
- tt --help

Upgrade: rebuild the binary from the latest source using the same steps.

---

## Quickstart

- Start a new running entry:
  - tt start acme portal -a dev -n "Bootstrap service"
- Add notes while running:
  - tt note "Drafted data model"
- Stop the current entry:
  - tt stop
- List what you did today:
  - tt ls --today
- Summarize for billing:
  - tt report --today --by customer,project,activity
- Verify the journal hash-chain:
  - tt audit verify

A short tour
- tt start [customer] [project] starts a running session; you can set activity, billable, tags, and note with flags.
- tt note <text> appends a note to the currently running entry.
- tt stop closes the running entry.
- tt add <start> <end> [customer] [project] records a finished entry in the past (handy for meetings).
- tt ls and tt report summarize entries for a period.
- tt audit verifies (and can repair) per-day hash integrity.
- tt completion installs or prints completion scripts (zsh/bash/fish/powershell).
- tt tui starts an optional interactive terminal UI (experimental).

---

## Commands (everyday use)

Start a running entry
- tt start [customer] [project]
- Flags:
  - -a, --activity string  Activity (design, workshop, docs, travel, etc.)
  - -b, --billable         Mark as billable (default true)
  - -t, --tag value        Tag(s); repeat for multiple
  - -n, --note string      Note to attach to this entry
  - --at string            Custom start time (see “Time formats”)
- Examples:
  - tt start acme portal -a dev -n "Init repository"
  - tt start acme portal --at 2025-10-07T09:00

Add a finished (retro) entry
- tt add <start> <end> [customer] [project]
- Same flags as start for activity, billable, tags, and note
- Examples:
  - tt add 09:00 10:30 acme portal -a workshop -n "Kickoff"
  - tt add 2025-10-07T13:00 2025-10-07T14:15 acme portal -t demo

Stop the current entry
- tt stop

Switch to a new entry (stop current, then start new)
- tt switch [customer] [project]
- Flags (same semantics as start):
  - -a/--activity, -b/--billable, -t/--tag, -n/--note

Add a note to the current entry
- tt note <text>

Show current status and last closed entry
- tt status

---

## Listing and reporting

List entries
- tt ls [--today | --range A..B]
- Flags:
  - --today           Today only
  - --range A..B      Custom range (see “Time formats”)
- Output includes date, time span, activity, customer/project, billable, and HHhMMm.

Summarize for billing
- tt report [--today | --week | --range A..B] [--by fields] [--detailed]
- Flags:
  - --today                 Today only
  - --week                  This week (Mon..Sun)
  - --range A..B            Custom range (see “Time formats”)
  - --by string             Comma-separated fields to group by (default: customer,project,activity)
  - --detailed              Include per-entry details/notes
- Rounding and minimum billable per entry are configured via config (see Configuration).

Weekly report (table/markdown/json + optional Tempo export)
- tt report week
- Useful when you need a week-by-week breakdown with merged notes.
- Flags:
  - --week string           ISO week, e.g. 2025-W41 (default = current ISO week)
  - --from YYYY-MM-DD       Start date (overrides --week if used with --to)
  - --to YYYY-MM-DD         End date (overrides --week if used with --from)
  - --format string         table | json | markdown (default: table)
  - --round int             Divisions per hour for rounding (e.g., 4 => 15-min quantum)
  - --customer string       Filter by exact customer (case-insensitive)
  - --tag value             Filter by tag (repeatable; AND logic)
  - --include-open          Include running entries (treat end = now)
  - --notes-wrap int        Wrap merged notes to N columns (0 = no wrap) (default: 80)
  - --locale string         de | en (default: de)
  - --export-tempo path     Write Tempo JSON export to a file
  - --tempo-rounded         Use rounded seconds in Tempo export

---

## Time formats

Accepted inputs (for tt add and start --at)
- RFC3339 with timezone:
  - 2025-10-07T09:00:00+02:00
- Local date-time (no timezone):
  - 2025-10-07T09:00, 2025-10-07 09:00, with or without seconds
- Time-only (assumes “today” in your configured timezone):
  - 09:00, 09:00:30

Ranges:
- Use A..B with any accepted forms (e.g., 2025-10-07T09:00..2025-10-07T17:00 or 09:00..17:00).

Timezone:
- The CLI uses your configured timezone (see Configuration). If none provided, it falls back to your system local timezone.

---

## Configuration

Config file (auto-created if missing):
- ~/.tt/config.yaml

Defaults
- timezone: Europe/Berlin (if not set, system local is used)
- rounding.quantum_min: 15
- rounding.minimum_billable_min: 0
- rounding.strategy: up (effective default; can be down or nearest)

Suggested config
timezone: Europe/Berlin
rounding:
  # Rounding strategy applied by tt report
  # up | down | nearest
  strategy: up
  # Minute quantum for rounding (e.g. 6 => 10 min, 4 => 15 min)
  quantum_min: 15
  # Minimum billable minutes per entry after rounding
  minimum_billable_min: 0

Notes
- tt report uses rounding settings from the config.
- The weekly subcommand (tt report week) also accepts a per-run rounding quantum via --round.

---

## Data storage and integrity

Where your data lives
- Journal root: ~/.tt/journal
- Per-day JSONL files:
  - ~/.tt/journal/YYYY/MM/YYYY-MM-DD.jsonl
- Per-day anchor (last hash):
  - ~/.tt/journal/YYYY/MM/YYYY-MM-DD.hash

Format
- Each line in the .jsonl file is a single immutable event (start, stop, add, note, etc.).
- Events include a deterministic hash and a prev_hash, forming a per-day hash chain.

Verify integrity
- tt audit verify
  - Walks your journal and confirms each event’s stored hash matches the canonical payload hash or a legacy-compatible hash.
  - Compares chain end with the per-day .hash anchor when present.

Repair and migrate hashes (safe, inspectable workflow)
- tt audit repair
  - Dry-run by default: proposes repaired files next to originals:
    - 2025-10-07.jsonl.repair and 2025-10-07.hash.repair
  - Prints a small inline diff preview of the first changes.
  - Apply changes (destructive, with backups):
    - tt audit repair --dry-run=false --apply=true
    - Backs up originals to .bak (both .jsonl and .hash when present), then writes repaired files.

Why repair?
- Older files might contain hashes computed from map-encoded JSON (unstable key order).
- Current code uses a canonical struct to ensure stable hashes.
- The repair command updates stored hash/prev_hash to canonical and keeps the chain consistent.

Tips
- After a repair dry-run, inspect differences:
  - diff -u ~/.tt/journal/2025/10/2025-10-07.jsonl ~/.tt/journal/2025/10/2025-10-07.jsonl.repair
- Always re-run:
  - tt audit verify

---

## Shell completion

Generate a completion script for your shell
- tt completion zsh
- tt completion bash
- tt completion fish
- tt completion powershell

Zsh (recommended)
- One-off (current shell):
  - source <(tt completion zsh)
- Persistent (manual):
  - mkdir -p ~/.zfunc
  - tt completion zsh > ~/.zfunc/_tt
  - Ensure in ~/.zshrc:
    - fpath=(~/.zfunc $fpath)
    - autoload -Uz compinit && compinit
  - Restart zsh or run exec zsh
- Automated installer:
  - tt completion --install-zsh
  - Writes ~/.zfunc/_tt and appends a clearly marked block to ~/.zshrc when needed (asks for confirmation first)

Dynamic suggestions
- Customer/project suggestions come from your own journals. If your journal is very large, completions may take slightly longer the first time they run in a shell session.

---

## Optional TUI (experimental)

- tt tui
- Launches a minimal Bubble Tea dashboard that shows a live timer, last entry, and will evolve toward a timeline and command palette.
- Best used in a truecolor-capable terminal.

---

## Examples

A typical day
- tt start acme portal -a dev -n "Standup + sprint planning"
- tt note "Reviewed PR #42"
- tt switch acme portal -a review -n "API contract"
- tt stop

List and report
- tt ls --today
- tt report --today --by customer,project,activity
- tt report week --week 2025-W41 --format markdown

Retro meeting
- tt add 09:00 09:45 acme portal -a workshop -t kickoff -n "Stakeholder alignment"

Verification and repair
- tt audit verify
- tt audit repair         # dry-run (writes *.repair)
- tt audit repair --dry-run=false --apply=true

---

## Troubleshooting

- No completion? Ensure your shell’s config sources the generated script (or use the zsh installer).
- “Cannot parse time”: Check the “Time formats” section; for time-only inputs, the date defaults to today in your timezone.
- Empty listings: Confirm you specified the right range, and that you’ve started/stopped at least one entry.
- Integrity failures on verify: Inspect the lines reported, then run a dry-run repair and review the proposed changes.
- Backups after repair: Look for .bak files next to your original journals/anchors.

---

Happy tracking! If you want enhancements (CSV/XLSX export, additional rounding strategies, richer TUI, etc.), open an issue or propose a change.
