# Contributing to teams-cli

This repository is the maintained fork of the archived upstream
`fossteams/teams-cli` project. New changes should be proposed against this
repository.

## Development Setup

Requirements:

- Go 1.18 or newer
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

## Token Handling

Keep JWT files out of the repository.

- The app reads tokens from `~/.config/fossteams`
- Do not copy token files into this repository
- Do not commit `.jwt` files or local auth artifacts

## Contribution Workflow

1. Create a branch from `master`.
2. Keep changes scoped and explain the user-visible behavior.
3. Run `go build ./...` and `go test ./...`.
4. Update `README.md` when behavior, controls, or runtime options change.
5. Open the pull request against this repository's `master` branch.

## Change Guidelines

- Preserve keyboard-first navigation behavior
- Avoid committing local binaries, tokens, or machine-specific artifacts
- Add or update tests for navigation, ordering, loading, or option parsing when
  behavior changes
- Keep documentation aligned with the actual runtime behavior
