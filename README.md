# teams-cli

A Command Line Interface (or TUI) to interact with Microsoft Teams
that uses the [teams-api](https://github.com/fossteams/teams-api)
Go package.

## Status

Upstream `fossteams/teams-cli` has been archived and is read-only. This fork is
the active maintenance branch for the codebase, and new work should target this
repository.

This fork now uses a `dev` to `main` flow:

- day-to-day work lands on `dev`
- release candidates are promoted to `main`
- pushing a new semantic version in `version.go` to `main` publishes the next release
- `main` and `dev` are governed branches with required checks, CODEOWNERS, and
  pull-request based review expectations
- signed commits are required on `main` and `dev`

The CLI can authenticate with `teams-token`, list your Teams, Channels, and
Chats, and read recent messages inside the TUI.

This project is still WIP and will be updated with more features. The goal is to
have a CLI / TUI replacement for the Microsoft Teams desktop client.

Today the client is primarily read-only:

- Browse Teams, Channels, and direct/group Chats
- Read recent messages in the selected conversation
- Refresh conversations automatically without restarting the app
- Shut down cleanly without leaving refresh workers or in-flight loads behind
- Navigate the UI entirely from the keyboard

## Requirements

- [Golang](https://golang.org/) 1.26.1 or newer
- Valid Teams JWT files generated with [teams-token](https://github.com/fossteams/teams-token)
- A terminal with cursor-addressing support, for example Terminal.app or iTerm2

## Usage

Follow the instructions on how to obtain tokens with
[teams-token](https://github.com/fossteams/teams-token), then run the app.
Tagged releases now publish tarballs and checksums for:

- macOS Apple Silicon: `teams-cli_<VERSION>_darwin_arm64.tar.gz`
- macOS Intel: `teams-cli_<VERSION>_darwin_amd64.tar.gz`
- Linux x86_64: `teams-cli_<VERSION>_linux_amd64.tar.gz`
- Linux arm64: `teams-cli_<VERSION>_linux_arm64.tar.gz`

Each release also includes:

- `teams-cli_<VERSION>_checksums.txt`
- `teams-cli_<VERSION>_checksums.txt.sigstore.json`
- `teams-cli_<VERSION>_sboms.tar.gz`

GitHub build attestations are still generated for release archives, but they are
kept in GitHub attestation records instead of being uploaded as extra release
files.

## Install From Release

Replace `<VERSION>` with a real tag such as `v0.2.1`, then download the archive
that matches your machine plus the checksum file from the release page.

Example for macOS Apple Silicon:

```bash
VERSION=v0.2.1
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_darwin_arm64.tar.gz"
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_checksums.txt"
grep " teams-cli_${VERSION}_darwin_arm64.tar.gz$" "teams-cli_${VERSION}_checksums.txt" | shasum -a 256 -c
tar -xzf "teams-cli_${VERSION}_darwin_arm64.tar.gz"
install -m 0755 teams-cli /usr/local/bin/teams-cli
teams-cli --version
```

To verify the signed checksum file:

```bash
cosign verify-blob "teams-cli_${VERSION}_checksums.txt" \
  --bundle "teams-cli_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity "https://github.com/vaishnavucv/teams-cli/.github/workflows/ci-release.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Then verify the archive checksum against that trusted file:

```bash
grep " teams-cli_${VERSION}_darwin_arm64.tar.gz$" "teams-cli_${VERSION}_checksums.txt" | shasum -a 256 -c
```

To verify GitHub build provenance for the same archive:

```bash
gh attestation verify "teams-cli_${VERSION}_darwin_arm64.tar.gz" \
  --repo vaishnavucv/teams-cli \
  --signer-workflow vaishnavucv/teams-cli/.github/workflows/ci-release.yml
```

Example for Linux x86_64:

```bash
VERSION=v0.2.1
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_linux_amd64.tar.gz"
curl -LO "https://github.com/vaishnavucv/teams-cli/releases/download/${VERSION}/teams-cli_${VERSION}_checksums.txt"
grep " teams-cli_${VERSION}_linux_amd64.tar.gz$" "teams-cli_${VERSION}_checksums.txt" | shasum -a 256 -c
tar -xzf "teams-cli_${VERSION}_linux_amd64.tar.gz"
install -m 0755 teams-cli /usr/local/bin/teams-cli
teams-cli --version
```

If `install` is not appropriate for your setup, extract the archive and move
`teams-cli` to any directory already on your `PATH`.

```bash
go run ./
```

To limit each conversation view to the most recent `N` messages, pass either
`msg=<n>` or `--msg=<n>`:

```bash
go run ./ msg=20
go run ./ --msg=20
```

To inspect the available runtime flags:

```bash
go run ./ --help
```

To print the current build version:

```bash
go run ./ --version
```

To build and run the local binary:

```bash
go build -o teams-cli .
TERM=xterm-256color ./teams-cli msg=20
```

The app reads your Teams JWT files from `~/.config/fossteams`. Keep those token
files outside this repository and do not commit them.

Additional runtime options:

- `--debug`: shortcut for `--log-level debug`
- `--log-level <level>`: configure log verbosity (`debug`, `info`, `warn`, `error`)
- `--token-dir <dir>`: read Teams JWT files from a custom directory instead of the default location
- `--refresh-messages <duration>`: override the selected-conversation polling interval
- `--refresh-tree <duration>`: override the conversation-tree polling interval
- `--no-live`: disable background refresh polling entirely
- `--version`: print the current CLI version and exit
- `doctor`: run diagnostics for tokens, refresh configuration, terminal support, and Microsoft endpoint reachability

Examples:

```bash
go run ./ --token-dir ~/.config/fossteams --debug --msg 50
go run ./ --refresh-messages 10s --refresh-tree 30s
go run ./ --no-live
go run ./ --version
go run ./ doctor --token-dir ~/.config/fossteams
```

## Contributing

Contributions should be opened against this fork, not the archived upstream
repository.

- Open pull requests against this repository's `dev` branch
- Expect CODEOWNERS review on repository, workflow, and release-path changes
- Run `go build ./...` and `go test ./...` before sending changes
- Keep local Teams JWT files outside the repository
- See [CONTRIBUTING.md](./CONTRIBUTING.md) for contributor setup and workflow

## Governance And Security Gates

Phase 1 governance for this maintained fork now includes:

- `CODEOWNERS` coverage for the repository, workflows, and release scripts
- protected `main` and `dev` branches with required status checks and
  pull-request review policy
- required signed commits on protected branches
- one combined `CI and Release` workflow that runs quality, security, and
  release jobs
- CodeQL analysis for Go sources
- dependency review on pull requests
- secret scanning in CI with `gitleaks`

The repository owner remains the only maintainer today, so branch protection is
configured to preserve an admin bypass path for recovery while still enforcing
checks and reviews for the normal contribution flow.

## Release Flow

Releases now come from `main`, not from pushed tags.

1. Develop on `dev` and keep `version.go` at `dev`.
2. When the next release is ready, change `version.go` to the next version such as `v0.2.1` and update [CHANGELOG.md](./CHANGELOG.md).
3. Merge or push that versioned change to `main`.
4. The combined GitHub Actions workflow runs CI and holds the release job behind the protected `release` environment for manual approval.
5. After approval, the workflow smoke-tests the built archives, signs and attests the release artifacts, then creates the GitHub release and tag automatically.
6. Release assets now include tarballs, checksums, one checksum signature
   bundle, and one bundled SBOM archive. GitHub attestations remain available
   through the repository attestation records.

## Release Verification And Rollback

Recommended verification after every published release:

- verify the archive checksum against `teams-cli_<VERSION>_checksums.txt`
- verify the signed checksum file with `cosign verify-blob`
- verify the archive against the trusted checksum file
- verify GitHub provenance with `gh attestation verify`
- review the SPDX SBOM that matches the downloaded archive

Rollback guidance:

- if a release is still waiting for approval, reject or cancel the `release`
  environment deployment instead of publishing
- if a bad release is already published, prefer shipping a superseding patch
  release instead of replacing assets in place
- fix on `dev`, promote to `main`, bump to the next patch version, and publish a
  new signed release
- only delete or edit an existing release as an emergency exception, because it
  weakens the audit trail for signed artifacts

## Support

- Use the built-in issue templates for bug reports and feature requests
- Review [SECURITY.md](./SECURITY.md) before reporting security-sensitive
  behavior
- See [CHANGELOG.md](./CHANGELOG.md) for release notes and unreleased work
- This fork currently keeps the historical Go module path
  `github.com/fossteams/teams-cli` for compatibility; release binaries on this
  fork are the supported install path

## Current Features

- Lists Teams, Channels, and Chats in one conversation tree
- Loads messages automatically when you move onto a channel or chat
- Limits the message view to the most recent `N` messages
- Shows a live loading indicator while messages are being fetched
- Refreshes the selected conversation every 5 seconds by default
- Refreshes the conversation tree every 15 seconds by default
- Allows refresh polling to be tuned or disabled from the CLI
- Keeps Teams, Channels, and Chats in a stable order while refreshing
- Cancels stale message loads when you switch conversations
- Stops cleanly on `q`, `Ctrl+C`, or `SIGTERM`
- Writes structured JSON logs to a user-local log file
- Supports `--debug` and structured runtime error logging
- Displays a keyboard help overlay directly inside the TUI
- Includes a `doctor` mode for local configuration and connectivity checks

## Runtime Hardening

The current runtime path is designed to fail in place instead of freezing or
leaving background work running.

- Startup installs a signal-aware app context so `q`, `Ctrl+C`, and `SIGTERM`
  stop the TUI cleanly
- Switching conversations cancels the previous in-flight message load before a
  new fetch starts
- Message requests use a bounded timeout and bounded retry/backoff for transient
  failures
- Fetch errors stay inline inside the messages pane so the TUI remains usable
  and the next refresh or manual selection can recover
- Early startup failures are logged and printed with a `See log:` path so the
  error is inspectable after the process exits

## Observability

The CLI now keeps structured JSON logs on disk instead of relying on transient
terminal output.

- `--debug` raises the logger to debug level without changing any other runtime
  flags
- Logs are written to a user-local file:
  - macOS: `~/Library/Logs/teams-cli/teams-cli.log`
  - Linux: `$XDG_STATE_HOME/teams-cli/teams-cli.log` or `~/.local/state/teams-cli/teams-cli.log`
- Log entries are structured JSON so startup, refresh, retry, and failure events
  are easier to inspect and parse
- Sensitive values such as JWTs, auth headers, and token-like query values are
  redacted before they are written

## Runtime Behavior

When you select a leaf conversation, the message pane starts loading
immediately and shows a TUI loading bar until the fetch completes.

The selected conversation refreshes automatically every 5 seconds, and the
conversation tree refreshes every 15 seconds so new messages, renamed chats,
and ordering changes show up without restarting the TUI.

If you switch away from a conversation while it is loading, the old request is
canceled and the new selection takes over. If a transient fetch fails, the
message pane shows the error inline and the next retry or refresh can recover
without restarting the process.

Chats and channels are sorted to make browsing more predictable:

- Teams prefer favorite and followed teams first
- Channels prefer pinned channels and `General`
- Chats prefer visible or sticky chats and then recent activity

## Token Safety

This repository should not contain your local Teams tokens.

- `teams-cli` reads tokens from `~/.config/fossteams`
- `--token-dir` can be used to point at another token directory when needed
- Runtime startup requires `token-skype.jwt` and `token-chatsvcagg.jwt`
- `doctor` also inspects `token-teams.jwt` when present and reports expiry
- Local JWT artifacts are ignored by `.gitignore`
- Do not copy token files into this repository before building or testing

## Diagnostics

Use `doctor` before debugging runtime issues or when setting up a new machine:

```bash
go run ./ doctor
go run ./ doctor --token-dir ~/.config/fossteams --no-live
```

`doctor` checks:

- terminal support via `TERM`
- log file path resolution and writability
- current CLI refresh configuration
- token directory accessibility
- required runtime tokens and their expiry/claims
- optional Teams token metadata when available
- TCP reachability to `teams.microsoft.com` and the current Teams message host

## Keyboard Navigation

The app is keyboard-first. A compact key legend is always shown at the bottom
of the screen, and `?` opens the full help view.

### Conversations Pane

- `Up` / `Down`: move between Teams, Channels, and Chats
- `Right` / `l`: expand a group, enter its first child, or open the selected conversation
- `Left` / `h`: collapse the current group or move to its parent
- `Esc`: go back one level
- `Enter`: open the selected channel or chat and move to the messages pane
- `Tab`: switch focus to the messages pane

### Messages Pane

- `Up` / `Down`: move through loaded messages
- `PgUp` / `PgDn`: page through messages
- `Home` / `End`: jump to the first or last loaded message
- `Left` / `h`: return to the conversations pane
- `Esc` / `Tab`: return to the conversations pane

### Global

- `?`: show or hide keyboard help
- `q`: quit
- `Ctrl+C`: clean shutdown

If everything goes well, you should see something like this:
![Teams CLI example](./docs/screenshots/2021-04-13.png)

## What works

- Logging in to Teams using tokens generated via `teams-token`
- Getting the list of Teams, Channels, and Chats
- Reading recent messages in channels and chats
- Limiting message views to the most recent `N` messages via `msg=<n>` or `--msg=<n>`
- Showing live loading feedback while messages are being fetched
- Refreshing the selected conversation and conversation tree automatically
- Tuning or disabling refresh intervals from the CLI
- Canceling stale loads and shutting down cleanly
- Writing structured local logs with redaction
- Stable ordering for Teams, Channels, and Chats while refreshing
- Keyboard-first navigation between conversations and messages
- Running local diagnostics with `doctor`

## What doesn't work

- Sending messages
- Editing messages
- Reactions, uploads, calls, and the rest of the Teams feature surface

## You might also be interested in

- [fossteams-frontend](https://github.com/fossteams/fossteams-frontend): a Vue based frontend for Microsoft Teams
