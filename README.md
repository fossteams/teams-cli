# teams-cli

A Command Line Interface (or TUI) to interact with Microsoft Teams that uses the [teams-api](https://github.com/fossteams/teams-api) Go package.

## Status

Upstream `fossteams/teams-cli` has been archived and is read-only. This fork is the active maintenance branch for the codebase, and new work should target this repository.

This project is still WIP and will be updated with more features. The goal is to have a CLI / TUI replacement for the Microsoft Teams desktop client. Today the client browses conversations, reads recent messages, and sends plain-text replies.

## Documentation

**We have moved our comprehensive documentation to the Wiki.** 

Please browse the [GitHub Wiki](https://github.com/vaishnavucv/teams-cli/wiki) for detailed guides:
- [Installation Guide](https://github.com/vaishnavucv/teams-cli/wiki/Installation)
- [Usage & Keyboard Navigation](https://github.com/vaishnavucv/teams-cli/wiki/Usage)
- [Features List](https://github.com/vaishnavucv/teams-cli/wiki/Features)
- [Development, CI/CD, & Governance](https://github.com/vaishnavucv/teams-cli/wiki/CI-CD)

## Quick Start

### Requirements
- [Golang](https://golang.org/) 1.26.1 or newer
- A Chrome-family browser (Google Chrome, Chromium, Edge, Brave, or a Playwright Chromium) for sign-in
- A terminal with cursor-addressing support (e.g. Terminal.app, iTerm2)

### Sign in

```bash
go build -o teams-cli . && ./teams-cli refresh-token
```

A browser window opens on the Microsoft sign-in page the first time. Sign in as usual (SSO, MFA). The tokens are saved to `~/.config/fossteams/` and the browser profile is kept, so later refreshes finish silently in the background. The TUI refreshes tokens on its own when they are about to expire or a request is rejected; press `R` inside it if a sign-in is needed again. Use `./teams-cli refresh-token --logout` to forget the browser session.

### Basic Usage

```bash
./teams-cli
```

To limit each conversation view to the most recent `N` messages:
```bash
go run ./ msg=20
```

*For more runtime flags and usage examples, see the [Usage Wiki](wiki/Usage.md).*

## Lite Overview

### Core Features
- Browse your Teams, Channels, direct/group Chats, and Meeting chats from a TUI, newest activity first.
- Pin chats locally with `p`; pins are stored in `teams-cli/pins.json` under your user config directory.
- Automatically load and read recent messages in selected conversations.
- Send plain-text replies from the messages pane.
- Background refresh for both message lists and conversation trees.
- Navigate the UI entirely with keyboard shortcuts.
- Clean shutdown on `q` or `Ctrl+C`.

### Keyboard Navigation
- **Arrow Keys** or **j/k/h/l**: Move up/down lists, expand (`Right`/`l`) or collapse (`Left`/`h`) trees.
- **Enter**: Open the selected channel or chat.
- **Tab**: Switch focus between the conversations tree and messages pane.
- **Messages pane**: `j`/`k` or arrows scroll, `PgUp`/`PgDn` page, `g`/`G` jump to top/bottom. Long messages wrap.
- **Enter** or **i** in the messages pane: open the reply line. `Enter` sends, `Esc` cancels. Replies are plain text and work in chats, meeting chats, and channels (a channel reply starts a new post).
- **Esc**: Go back one level or return to the conversations tree.
- **p**: Pin or unpin the selected chat so it stays at the top of Chats.
- **?**: Show/hide the full keyboard help inside the app.
- **q**: Quit the app.

### Diagnostics
If you have issues logging in or starting the app, run the built-in doctor to test token validity and network reachability:
```bash
go run ./ doctor
```

## Support

- Found an issue or have a feature request? Please use our **built-in issue templates**.
- Found a security vulnerability? Please review our **[SECURITY.md](./SECURITY.md)** guidelines privately before opening a public issue.
- View our **[CHANGELOG.md](./CHANGELOG.md)** for detailed release notes and updates.
- This fork maintains the `github.com/fossteams/teams-cli` module path for compatibility, but the releases published here are the officially supported install path.
