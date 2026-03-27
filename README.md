# teams-cli

A Command Line Interface (or TUI) to interact with Microsoft Teams
that uses the [teams-api](https://github.com/fossteams/teams-api)
Go package.

## Status

The CLI can log in with `teams-token`, list your Teams, Channels, and Chats,
and read recent messages inside the TUI.

This project is still WIP and will be updated with more features. The goal is to
have a CLI / TUI replacement for the Microsoft Teams desktop client.

## Requirements

- [Golang](https://golang.org/)

## Usage

Follow the instructions on how to obtain a token with [teams-token](https://github.com/fossteams/teams-token),
then simply run the following to start the app. Binary releases will appear on this repository as soon as
we have a product with more features.

```bash
go run ./
```

To limit each conversation view to the most recent `N` messages:

```bash
go run ./ msg=20
```

To build and run the local binary:

```bash
go build -o teams-cli .
TERM=xterm-256color ./teams-cli msg=20
```

The app reads your Teams JWT files from `~/.config/fossteams`. Keep those
token files outside this repository and do not commit them.

The selected conversation refreshes automatically every 5 seconds, and the
conversation tree refreshes every 15 seconds so new messages, renamed chats,
and ordering changes show up without restarting the TUI.

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
- `Ctrl+C`: force quit

If everything goes well, you should see something like this:
![Teams CLI example](./docs/screenshots/2021-04-13.png)

## What works

- Logging in into Teams using the token generated via `teams-token`
- Getting the list of Teams, Channels, and Chats
- Reading recent messages in channels and chats
- Limiting message views to the most recent `N` messages via `msg=<n>`
- Showing live loading feedback while messages are being fetched
- Refreshing the selected conversation and conversation tree automatically
- Keyboard-first navigation between conversations and messages

## What doesn't work

- Everything else

## You might also be interested in

- [fossteams-frontend](https://github.com/fossteams/fossteams-frontend): a Vue based frontend for Microsoft Teams
