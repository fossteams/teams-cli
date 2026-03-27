# Changelog

All notable changes to this maintained fork are documented in this file.

The format is based on Keep a Changelog, but kept intentionally lightweight for
this repository.

## [Unreleased]

### Added

- `CODEOWNERS` coverage for repository-critical paths
- CodeQL, dependency-review, and secret-detection workflows for the maintained
  fork
- Trusted release metadata generation for SBOMs, GitHub provenance bundles, and
  cosign keyless signatures on release artifacts

### Changed

- Source governance now expects protected `dev` and `main` branches with
  required checks and review policy
- Security gates now run inside the single `CI and Release` workflow instead of
  separate workflow files

## [v0.2.1] - 2026-03-27

### Changed

- The repository now uses a `dev` integration branch and a `main` release branch
- CI and release automation now run from a single combined workflow
- Releases are now created only from pushes to `main` when `version.go` is bumped to the next semantic version
- Future release runs now publish through the GitHub CLI in workflow steps
  instead of the deprecated Node 20 release action
- The release workflow artifact upload step now uses the current
  `actions/upload-artifact` major

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
