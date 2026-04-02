# Features

The CLI can authenticate with `teams-token`, list your Teams, Channels, and Chats, and read recent messages inside the TUI. The goal is to have a CLI / TUI replacement for the Microsoft Teams desktop client.

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

## What Works

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

## What Doesn't Work (Yet)

- Sending messages
- Editing messages
- Reactions, uploads, calls, and the rest of the Teams feature surface

## You might also be interested in
- [fossteams-frontend](https://github.com/fossteams/fossteams-frontend): a Vue based frontend for Microsoft Teams
