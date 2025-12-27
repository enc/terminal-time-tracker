# tt Specification (implementation guide)

This document captures the essential behaviours needed to implement `tt`. It focuses on storage format and the core CLI commands: `start`, `stop`, `switch`, and `report`.

## Storage
- **Location:** `$HOME/.tt/journal/YYYY/MM/YYYY-MM-DD.jsonl`
- **Format:** Append-only JSON Lines. One event per line; blank lines ignored.
- **Event fields:**
  - `id` (string, required): unique per event.
  - `type` (string, required): `start` | `stop` | `add` | `note` (future correction events may exist; ignore unknown types safely).
  - `ts` (RFC3339 string, required): event timestamp.
  - `customer`, `project`, `activity` (strings, optional).
  - `billable` (bool, optional; default true when omitted).
  - `note` (string, optional).
  - `tags` (array of strings, optional).
  - `ref` (string, optional; for `add`, it is `startISO..endISO`).
  - `prev_hash`, `hash` (strings, optional): hash chain fields; when present, preserve but an implementation may ignore for parsing.
- **Event ordering:** Write events in the order they occur. Use the event timestamp (`ts`) to choose the target file; the timestamp may differ from wall-clock “now”.
- **Parsing to entries:**
  - A `start` opens an entry (with its own `id`).
  - The next `stop` closes the most recent open entry (FIFO) and sets the end time to that stop’s timestamp.
  - An `add` creates a closed entry immediately using `ref`’s start/end timestamps and the event’s metadata.
  - `note` attaches to the most recent open entry if one exists; otherwise attach to the most recent closed entry for the same day.
  - Entries spanning midnight belong to both days for reporting; when reading a range, include the previous/next day to capture cross-midnight spans.

## Time & config
- Default timezone: `Europe/Berlin` (configurable; fall back to local).
- Time parsing for CLI flags accepts RFC3339 and `YYYY-MM-DD HH:MM` (or `THH:MM`).
- Rounding config (used by `report`):
  - `rounding.strategy`: `up` | `down` | `nearest` (default `up`).
  - `rounding.quantum_min`: minutes quantum (default 15).
  - `rounding.minimum_billable_min`: minimum minutes per entry (default 0).

## Commands

### start
- Syntax: `tt start [customer] [project] [-a activity] [-b billable] [-n note] [-t tag ...] [--at <time>]`
- Behaviour:
  - Resolve timestamp from `--at` or now (in configured timezone).
  - Write a `start` event with provided metadata; default `billable=true` if omitted.
  - If an entry is already running, either reject with an error or stop it first (current implementation stops via `switch`; `start` should warn/error).

### stop
- Syntax: `tt stop [--at <time>]`
- Behaviour:
  - Find the currently running entry (most recent `start` without `stop`).
  - Use `--at` or now as stop time; must be after start.
  - Write a `stop` event; entry becomes closed.
  - If no running entry, print a friendly message and exit non-zero.

### switch
- Syntax: `tt switch [customer] [project] [-a activity] [-b billable] [-n note] [-t tag ...] [--at <time>]`
- Behaviour:
  - Equivalent to `stop` (at the provided/derived timestamp) then `start` a new entry at the same timestamp with given metadata.
  - Reuse the `--at` time for both stop and subsequent start so they line up exactly.

### report
- Syntax: `tt report [--today|--week|--range A..B] [--by fields] [--detailed]`
- Behaviour:
  - Load entries intersecting the range (include +/- one day for cross-midnight).
  - Skip running entries or zero/negative durations.
  - Apply per-entry rounding: `roundMinutes(duration, config)`; then aggregate by requested fields (default `customer,project,activity`).
  - Output per-group raw and rounded totals, plus overall totals. In `--detailed` mode, list each entry with notes.
  - Time ranges:
    - `--today`: current day in configured TZ.
    - `--week`: ISO week (Mon–Sun) containing today.
    - `--range A..B`: inclusive range; accept dates or datetimes.

## Tickets in ICS/report (context)
- When generating ICS summaries, use `<ticket?> <customer> / <project>` (ticket prefix optional).
- Notes belong in the ICS description (deduplicated, one per line).
