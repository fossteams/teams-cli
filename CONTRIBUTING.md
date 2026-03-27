# Contributing to teams-cli

This repository is the maintained fork of the archived upstream
`fossteams/teams-cli` project. New changes should be proposed against this
repository.

## Development Setup

Requirements:

- Go 1.26.1 or newer
- A terminal with cursor-addressing support
- Teams JWT files generated with `teams-token`

Install dependencies and verify the local build:

```bash
go build ./...
go test ./...
```

To run the TUI locally:

```bash
TERM=xterm-256color go run ./
```

To limit the number of loaded messages per conversation:

```bash
TERM=xterm-256color go run ./ msg=20
```

To inspect the runtime configuration surface:

```bash
go run ./ --help
```

To enable debug logging while running locally:

```bash
TERM=xterm-256color go run ./ --debug
```

To run local diagnostics:

```bash
go run ./ doctor
```

To build release archives locally:

```bash
./scripts/build-release-artifacts.sh v0.1.0-test
```

## Token Handling

Keep JWT files out of the repository.

- The app reads tokens from `~/.config/fossteams`
- `--token-dir` can be used when token files live elsewhere
- Runtime logs are written to a user-local log file, not into this repository
- Do not copy token files into this repository
- Do not commit `.jwt` files or local auth artifacts

## Contribution Workflow

1. Create a branch from `dev`.
2. Keep changes scoped and explain the user-visible behavior.
3. Run `go build ./...` and `go test ./...`.
4. Run `go run ./ doctor` when changing token loading, refresh behavior, startup configuration, or logging behavior.
5. Update `README.md` and `CHANGELOG.md` when behavior, controls, or runtime options change.
6. Open the pull request against this repository's `dev` branch.

## Release Process

- Keep `version.go` at `dev` while iterating on `dev`
- When preparing the next release, set `version.go` to the next semantic version such as `v0.2.1`
- Update `CHANGELOG.md` for that version before merging to `main`
- Pushing the versioned commit to `main` triggers the combined CI and release workflow
- The workflow builds tarballs for darwin/linux on amd64/arm64, publishes checksums, creates the tag, and creates the GitHub release automatically
- After the release branch has landed, move `dev` back to `version = "dev"` for the next development cycle if needed

## Change Guidelines

- Preserve keyboard-first navigation behavior
- Avoid committing local binaries, tokens, or machine-specific artifacts
- Add or update tests for navigation, ordering, loading, or option parsing when
  behavior changes
- Add or update tests for logging, redaction, or startup diagnostics when
  observability behavior changes
- Keep documentation aligned with the actual runtime behavior

## Security Reporting

- Review [SECURITY.md](./SECURITY.md) before opening a public issue for any
  security-sensitive problem
- Never include JWTs, auth headers, cookies, or private Teams message content
  in issues, pull requests, or screenshots
- Prefer sanitized logs when reporting startup, refresh, or auth failures

## Module Path

This fork currently keeps the historical Go module path
`github.com/fossteams/teams-cli` for compatibility with existing scripts and
imports.

- Release binaries from this fork are the supported install path
- Any future module path change should be treated as a breaking release and
  documented in `CHANGELOG.md`
