# internal/journal

This package provides a focused, well-tested parser for the CLI journal JSONL format used by this repository.
It reconstructs materialized time `Entry` records from the event stream (JSON lines) produced by the `cmd` code.

Place
- Package path: `internal/journal`
- Use `internal` because this package is intended for use inside this repository only.

Goals
- Centralize journal parsing logic so `cmd/*` commands reuse a single, tested implementation.
- Provide a small, stable API: parsing, streaming, and typed results.
- Emit helpful parse errors (with line/path context) when requested.
- Be safe for concurrent use from CLI commands.

Key types and functions
- `type Event` — raw event shape (mirrors the JSON written to journal files).
- `type Entry` — reconstructed entry with fields: `ID`, `Start`, `End`, `Customer`, `Project`, `Activity`, `Billable`, `Notes`, `Tags`, `Source`.
- `type Parser` — parser configuration (timezone and strict mode).
- `func NewParser(timezone string) *Parser` — create a parser; empty timezone uses local.
- `func (p *Parser) ParseReader(r io.Reader) ([]Entry, error)` — parse events from an `io.Reader`.
- `func (p *Parser) ParseFile(path string) ([]Entry, error)` — parse a journal file and set `Entry.Source` to the path.
- `func (p *Parser) ParseStream(r io.Reader) (<-chan Entry, <-chan error)` — parse and stream entries via channel.
- `type ParseError` — returned on parse failures in strict mode and contains `Path`, `Line`, and `Err`.
- `var ErrInvalidRef` — error used when an `add` event `ref` has invalid format.

Behavior notes
- Non-strict (default) mode: malformed JSON lines or invalid `add` refs are skipped (parsing proceeds).
- Strict mode (`p.Strict = true`): the first parsing problem aborts and is returned as a `*ParseError`.
- Events are sorted by timestamp before reconstruction to ensure deterministic results.
- Reconstruction rules:
  - `start` begins a running entry (previous running entry is auto-stopped at new `start` ts).
  - `note` appends to the current running entry's `Notes`.
  - `stop` closes the current running entry and yields an `Entry`.
  - `add` creates an explicit `Entry` from `ref` in the form `startISO..endISO` (RFC3339).
  - Other event types (like `amend`, `pause`, `resume`) are ignored by reconstruction but preserved in the `Event` type.

Usage examples
- Quick one-off parse inside a command:
Use `NewParser("")` to get a parser and call `ParseFile` or `ParseReader`:
- Example (conceptual — adapt for your `cmd` code):
  - Create a parser:
    - `p := journal.NewParser("")`
  - Parse a file:
    - `entries, err := p.ParseFile(path)`
  - Iterate and use `entries` (each `Entry` has `Start`, `End`, `Project`, ...)

- Streaming usage:
  - `ch, errc := p.ParseStream(reader)`
  - Range over `ch` to process entries incrementally; check `errc` for parsing errors.

Testing
- This package includes unit tests in the repository (`internal/journal/journal_test.go`).
- Run tests for this package:
  - `go test ./internal/journal`
- Tests cover:
  - Reconstruction from mixed `start/note/stop/add` events.
  - Strict vs non-strict handling of malformed lines and invalid `add` refs.
  - `ParseFile` sets the `Entry.Source` field correctly.
  - `ParseStream` returns entries and exposes parsing errors.

Migration / Integration plan
- Replace duplicated parsing code in `cmd/common.go`/other commands by calling into this package:
  1. Import the package as `internal/journal`.
  2. Create a `Parser` with `journal.NewParser(timezone)` (use `""` to keep local timezone).
  3. Swap existing ad-hoc parsing to `ParseFile`, `ParseReader`, or `ParseStream`.
  4. If commands need more control in tests, inject a `Parser` (or an interface wrapper) in command constructors.
- Migrate commands one-by-one, keeping the old code until tests and usage are validated.

Diagnostics and error handling
- When you want user-facing errors with context, enable strict mode:
  - `p.Strict = true`
  - Errors will be returned as `*ParseError` with `Path` and `Line` populated (if parsing from a file).
- For tolerant parsing (best-effort), leave `Strict` false and check returned entries.

Extending the package
- Format plugins: the package currently parses the JSONL event format only. If you add alternate input formats, expose a `FormatParser` register system.
- Validation: consider adding `ValidateEntry(e Entry) error` and `Normalize(e *Entry)` helpers if downstream needs strict normalization.

Notes
- The package is intentionally small and focuses on deterministic reconstruction logic.
- Keep the public API minimal (`Entry`, `Parser`, `NewParser`, `ParseReader`, `ParseFile`, `ParseStream`, `ParseError`, `ErrInvalidRef`) to minimize churn when internals evolve.

If you'd like, I can:
- Provide a PR that refactors one `cmd` to use this package as an example migration.
- Add more examples to this README (including copy-paste-ready snippets).
