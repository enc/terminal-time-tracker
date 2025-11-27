# Changelog

## 0.4.2 - 2025-11-27

### Added
- Ticket management via `tt ticket set/list/rm` to associate IDs (e.g., ABC-123) with customer/project pairs.
- Ticket numbers appear in report labels and prefix ICS event titles alongside notes.
- Case-insensitive ticket lookups for friendlier matching.

## 0.4.3 - 2025-11-27

### Added
- Shell completion for `tt ticket set` now suggests customers and projects.

## 0.4.4 - 2025-11-27

### Added
- `tt ticket tui` Bubble Tea interface to add/edit/remove ticket numbers with keyboard-driven navigation.
- Ticket list output now uses a readable table layout.

## 0.4.5 - 2025-11-27

### Added
- `tt ticket list --missing` to show journal-observed projects lacking a ticket.
- Ticket TUI shows recent projects when adding a ticket and supports a missing-only filter toggle.

## 0.4.7 - 2025-11-27

### Changed
- Removed the ticket TUI (will be redesigned); CLI ticket commands remain.

## 0.4.8 - 2025-11-27

### Added
- Reintroduced `tt ticket tui` with a streamlined flow: list recent projects with tickets, filter missing/assigned/all, edit tickets inline with enter, and immediate writes to config.

## 0.4.9 - 2025-11-27

### Changed
- Styled the ticket TUI with Lip Gloss: colored headers, table rows, status/error accents, and clearer edit hints.

## 0.4.1 - 2025-11-27

### Changed
- ICS export events now place notes directly into SUMMARY and DESCRIPTION for clearer calendar entries.
- Bumped VERSION to 0.4.1.

## 0.4.0 - 2025-11-26

### Added
- `tt export ics` command to write per-day iCalendar files with customer/project/activity, notes, billable flag, and rounded durations.
- Rounding-aware allocation across cross-midnight entries so per-day segments sum to the configured rounded total.
- Documentation for ICS export and examples for weekly and range-based calendar generation.

### Changed
- Bumped VERSION to 0.4.0.

## 0.3.0 - 2025-10-27

### Added
- Budget management commands (`tt budget set/ls/show/edit/rm`) with warn/block enforcement during start/switch/add and reporting via `--with-budget`.
- Budget-aware warnings for stop/report flows with consistent colored messaging.

### Changed
- Entry reconstruction now spans multiple journal files to handle cross-midnight sessions and cross-day amendments.
- README updated with budget usage examples and demos.

## 0.2.0 - 2025-10-27

### Added
- `tt completion review`: interactive TUI to approve/ignore customers and projects for completion suggestions.

### Changed
- Completion now pulls from curated allow/ignore lists backed by config, hiding typos by default.
- Documentation updated to describe the new review flow and curated completion behaviour.

## 0.1.4 - 2025-10-27

### Changed
- Allow GoReleaser to tolerate the workflow-generated `VERSION` file.

## 0.1.3 - 2025-10-27

### Fixed
- Updated the release workflow to drop the deprecated `--rm-dist` flag.

## 0.1.2 - 2025-10-27

### Changed
- Incremented patch version after publishing 0.1.1.

## 0.1.1 - 2025-10-27

### Added
- Allow `tt amend` to default to the most recent entry when no id is provided.
- Provide `tt amend --select` to launch a simple TUI picker for choosing entries.
- Cover the amend selection helpers with unit tests.

## 0.1.0 - 2025-10-20

### Added
- Initial release.
