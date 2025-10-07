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
