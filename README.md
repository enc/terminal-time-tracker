# tt - CLI time tracker

## Purpose

tt is a fast, local, billing-friendly time tracker. It stores append-only JSONL events on disk, supports flexible time parsing, produces reports with rounding, and can export to calendars. An optional TUI provides a live dashboard.

## Quick guide

```bash
# start / stop
./tt start acme portal -a dev -n "Kickoff"
./tt note "Reviewed PR #42"
./tt pause
./tt continue
./tt stop

# switch to another task
./tt switch acme portal -a review -n "API contract"

# retro-add a meeting
./tt add 09:00 10:30 acme portal -a meeting -n "Weekly sync"

# inspect and report
./tt status
./tt ls --today
./tt report --week --by customer,project,activity

# verify journal integrity
./tt audit verify
```

## Command guide (important -> supportive)

Core tracking
- `tt start [customer] [project]` start a running entry
- `tt stop` stop the current entry
- `tt pause` pause the current entry
- `tt continue` continue the most recently paused entry
- `tt switch [customer] [project]` stop current and start a new entry at the same timestamp
- `tt note <text>` attach a note to the running entry
- `tt add <start> <end> [customer] [project]` add a finished entry in the past

Inspect and report
- `tt status` show active entry and last closed entry
- `tt ls [--today|--range A..B]` list entries
- `tt report [--today|--week|--range A..B] [--by fields] [--detailed] [--with-budget]` grouped summaries
- `tt report week [--week|--from/--to] [--format table|json|markdown] ...` weekly breakdown

Corrections and maintenance
- `tt amend [id] [--last|--select] [--start|--end|--note|--customer|--project|--activity|--billable|--tag]` append-only corrections
- `tt split [id] [--last] --at <time>` split an entry into two
- `tt merge --targets <id,id,...> [--since <time>] [--customer|--project|--activity|--billable|--into]` merge entries
- `tt audit verify` verify per-day hash chain
- `tt audit repair [--dry-run|--apply]` propose or apply canonical hash rewrites

Exports and integrations
- `tt export ics [--today|--week|--range A..B] [--out dir]` write per-day calendar files

Organization helpers
- `tt budget set|ls|show|edit|rm` budgets and enforcement
- `tt ticket set|list|rm|tui` ticket numbers per customer/project
- `tt alias set|list|rm|show` named alias for customer/project/activity
- `tt deprecate set <customer> [project] [--active]` hide from completions
- `tt deprecate list` list deprecated customers/projects
- `tt customer-merge ...` merge customer spellings for completion

Supportive
- `tt completion [bash|zsh|fish|powershell]` generate completion scripts
- `tt completion review` review completion allow/ignore lists
- `tt tui` live dashboard

## Time formats

Accepted inputs for `--at` and `tt add`:
- RFC3339: `2025-10-07T09:00:00+02:00`
- Local datetime: `2025-10-07 09:00` or `2025-10-07T09:00`
- Time only (today in configured timezone): `09:00`
- Relative: `now-30m`, `2h-now`, `+15m`, `15m`
- Ranges: `A..B` with any accepted form

## Configuration and files

- Config: `~/.tt/config.yaml`
- Deprecated list: `~/.tt/deprecated.yaml` (separate from config)
- Journal: `~/.tt/journal/YYYY/MM/YYYY-MM-DD.jsonl`
- Hash anchor: `~/.tt/journal/YYYY/MM/YYYY-MM-DD.hash`

Example config:
```yaml
timezone: Europe/Berlin
rounding:
  strategy: up
  quantum_min: 15
  minimum_billable_min: 0
```

Deprecations format (example):
```yaml
customers:
  - Acme
projects:
  Acme:
    - Legacy Portal
```

## Data integrity

Each journal line includes a hash and prev_hash to form a per-day chain. Use:
- `tt audit verify` to validate
- `tt audit repair` to propose canonical rewrites or apply fixes

## Shell completion

```bash
./tt completion zsh
./tt completion bash
./tt completion fish
./tt completion powershell
```

Zsh installer:
```bash
./tt completion --install-zsh
```

## TUI

```bash
./tt tui
```

Shows a live clock, current entry, and recent activity. Use `q` or `esc` to quit.

## Troubleshooting

- No completion: ensure your shell sources the generated script.
- Time parse errors: review the Time formats section.
- Integrity failures: run `tt audit repair` (dry-run first) and re-verify.
