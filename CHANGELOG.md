# Changelog

All notable changes to this maintained fork are documented in this file.

The format is based on Keep a Changelog, but kept intentionally lightweight for
this repository.

## [Unreleased]

### Added

- Security policy and support templates for bugs, feature requests, and pull
  requests
- Failure-mode tests for transport retries, cancellation, token edge cases,
  malformed payloads, UI focus transitions, and option parsing

### Changed

- Refactored the TUI/runtime implementation into smaller files by concern
- Raised the Go baseline to `1.26.1`
- Fixed the CI workflow syntax and aligned CI with the current Go baseline
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
