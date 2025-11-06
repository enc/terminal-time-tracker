# Changelog

# Changelog

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
