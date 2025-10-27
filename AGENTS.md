# Repository Guidelines

## Project Structure & Module Organization
- `main.go` wires the Cobra CLI and delegates subcommands defined in `cmd/`.
- Business logic for journals and the TUI lives under `internal/journal` and `internal/tui`.
- Tests reside beside their implementation files (e.g. `cmd/start_test.go`), plus fixture data in `cmd/testdata/`.

## Build, Test, and Development Commands
- `go build ./...` compiles the CLI; binary defaults to `tt`.
- `go test ./...` runs the unit and integration suite.
- `go run . <command>` quickly executes the CLI without building a binary.

## Coding Style & Naming Conventions
- Go modules follow standard formatting; always run `gofmt` (or `go fmt ./...`) before opening a PR.
- Keep package-level variables lowercase unless exported, and favour descriptive command names matching Cobra usage strings.
- Flags should use kebab-case identifiers to align with existing commands (see `cmd/start.go`).

## Core Principles
	•	Readability first: clear names, small focused functions, linear control flow, meaningful boundaries.
	•	SOLID where it helps; DRY but avoid premature abstraction; KISS; YAGNI.
	•	Deterministic, testable units; pure functions where practical; limit side effects.
	•	Fail fast with explicit errors; no silent catch/ignore.
	•	Security by default: validate inputs, least privilege, no secrets in code, safe defaults.
	•	Performance: measure critical paths, avoid accidental N², allocate once in hot loops, stream/iterate for large data.
## Testing Guidelines
- Prefer table-driven tests alongside the command or helper under test.
- Integration tests (e.g. `cmd/audit_integration_test.go`) can be run with `go test ./cmd -run Integration`.
- When adding features manipulating journals, use fixtures in `cmd/testdata/` or create temporary files via `t.TempDir()`.
•	Provide unit tests for new logic; focus on edge cases and error paths; fast and deterministic.
•	Include a minimal runnable usage example in docs or tests.
•	Use property/table-driven tests where helpful.
•	Avoid mocking internals—mock boundaries only.

## Commit & Pull Request Guidelines
- Commits in history are concise and imperative (e.g. “Add split command helpers”); follow that tone and keep scopes focused.
- Document user-visible CLI changes in the PR description and include example invocations where applicable.
- Link issues when relevant and note any manual verification steps (tests run, platforms checked) before requesting review.
