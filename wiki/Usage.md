# Usage

Follow the instructions on how to obtain tokens with [teams-token](https://github.com/fossteams/teams-token), then run the app. The app reads your Teams JWT files from `~/.config/fossteams`. Keep those token files outside this repository and do not commit them.

## Runtime Options

To limit each conversation view to the most recent `N` messages, pass either `msg=<n>` or `--msg=<n>`:

```bash
go run ./ msg=20
go run ./ --msg=20
```

Additional runtime options:

- `--debug`: shortcut for `--log-level debug`
- `--log-level <level>`: configure log verbosity (`debug`, `info`, `warn`, `error`)
- `--token-dir <dir>`: read Teams JWT files from a custom directory instead of the default location
- `--refresh-messages <duration>`: override the selected-conversation polling interval
- `--refresh-tree <duration>`: override the conversation-tree polling interval
- `--no-live`: disable background refresh polling entirely
- `--version`: print the current CLI version and exit
- `doctor`: run diagnostics for tokens, refresh configuration, terminal support, and Microsoft endpoint reachability

### Examples:

```bash
go run ./ --token-dir ~/.config/fossteams --debug --msg 50
go run ./ --refresh-messages 10s --refresh-tree 30s
go run ./ --no-live
go run ./ --version
go run ./ doctor --token-dir ~/.config/fossteams
```

## Keyboard Navigation

The app is keyboard-first. A compact key legend is always shown at the bottom of the screen, and `?` opens the full help view.

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
