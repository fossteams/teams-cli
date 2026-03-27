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
./scripts/build-release-artifacts.sh v0.2.1-test
```

To generate trusted-release metadata locally after building archives:

```bash
go install github.com/anchore/syft/cmd/syft@v1.42.3
./scripts/generate-release-sboms.sh v0.2.1-test
```

Keyless signing and GitHub attestations are performed in GitHub Actions during
the protected `main` release flow, because they rely on GitHub OIDC and
repository-scoped attestation APIs.

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
7. Wait for the governed checks to pass:
   - the single `CI and Release` workflow, which now includes quality,
     platform, security, and release gating jobs
   - required job checks such as `CodeQL`, `Dependency Review`, and `Secret Detection`
8. Expect CODEOWNERS review for workflow, security-policy, and release-path changes.

## Branch Governance

The maintained fork now treats `dev` and `main` as protected branches.

- `dev` is the integration branch and should receive normal feature pull requests
- `main` is the release branch and should receive reviewed promotions from `dev`
- direct pushes are reserved for maintainer recovery cases
- signed commits are required on protected branches
- stale reviews should be refreshed after materially changing a pull request

## Release Process

- Keep `version.go` at `dev` while iterating on `dev`
- When preparing the next release, set `version.go` to the next semantic version such as `v0.2.1`
- Update `CHANGELOG.md` for that version before merging to `main`
- Pushing the versioned commit to `main` triggers the combined CI and release workflow
- The `Publish Release` job is gated by the protected `release` environment and requires manual approval before publication
- Before publish, the workflow smoke-tests the built archives on the runner
- After approval, the workflow generates a bundled SPDX SBOM archive, signs the checksum file with a cosign keyless bundle, creates GitHub provenance attestations for release archives, creates the tag, and publishes the GitHub release automatically
- After the release branch has landed, move `dev` back to `version = "dev"` for the next development cycle if needed

## Release Rollback

- If the release job is waiting for environment approval, reject it rather than
  publishing suspect artifacts
- After publication, prefer a new patch release over mutating or replacing an
  existing signed release
- Revert or fix on `dev`, promote to `main`, bump the next patch version, and
  let the governed release flow publish the replacement
- If you must delete a release, record why in `CHANGELOG.md` or the release
  notes so the audit trail stays understandable

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
