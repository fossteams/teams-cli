# Changelog

All notable changes to this maintained fork are documented in this file.

The format is based on Keep a Changelog, but kept intentionally lightweight for
this repository.

## [Unreleased]

## [v0.2.0] - 2026-03-27

### Added

- Expanded CLI configuration with `--help`, `--version`, `--log-level`,
  `--token-dir`, `--refresh-messages`, `--refresh-tree`, `--no-live`, and
  `doctor`
- Structured JSON logging with redaction and user-local log files
- Security policy, issue templates, pull request template, and a lightweight
  changelog process for this maintained fork
- Failure-mode tests for transport retries, cancellation, token edge cases,
  malformed payloads, UI focus transitions, and option parsing

### Changed

- Hardened runtime shutdown, timeout handling, retry behavior, and inline error
  recovery
- Refactored the TUI/runtime implementation into smaller files by concern
- Raised the Go baseline to `1.26.1`
- Fixed the CI workflow syntax, aligned CI with the current Go baseline,
  upgraded GitHub Actions to current majors, and opted workflows into the Node
  24 JavaScript action runtime
- Clarified maintainer workflow, support paths, and module-path expectations

## [v0.1.0] - 2026-03-27

### Added

- Release automation with darwin/linux tarballs for amd64 and arm64 plus
  checksums
- Conversation browsing for Teams, Channels, and Chats with recent-message
  loading
- Keyboard navigation help, loading indicators, and live refresh behavior
- Runtime hardening, structured logging, and `doctor` diagnostics
- CI validation with build, test, race, coverage, `staticcheck`, and
  `govulncheck`

### Changed

- Positioned this repository as the maintained fork of the archived upstream
- Updated contributor and runtime documentation to match the current behavior
