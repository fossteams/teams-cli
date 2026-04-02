# teams-cli

A Command Line Interface (or TUI) to interact with Microsoft Teams that uses the [teams-api](https://github.com/fossteams/teams-api) Go package.

## Status

Upstream `fossteams/teams-cli` has been archived and is read-only. This fork is the active maintenance branch for the codebase, and new work should target this repository.

This project is still WIP and will be updated with more features. The goal is to have a CLI / TUI replacement for the Microsoft Teams desktop client. Today the client is primarily read-only (browsing conversations and reading recent messages).

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
- Valid Teams JWT files generated with [teams-token](https://github.com/fossteams/teams-token)
- A terminal with cursor-addressing support (e.g. Terminal.app, iTerm2)

### Basic Usage

Run the app locally once you have obtained your tokens:
```bash
go run ./
```

To limit each conversation view to the most recent `N` messages:
```bash
go run ./ msg=20
```

*For more runtime flags and usage examples, see the [Usage Wiki](wiki/Usage.md).*

## Lite Overview

### Core Features
- Browse your Teams, Channels, and direct/group Chats from a TUI.
- Automatically load and read recent messages in selected conversations.
- Background refresh for both message lists and conversation trees.
- Navigate the UI entirely with keyboard shortcuts.
- Clean shutdown on `q` or `Ctrl+C`.

### Keyboard Navigation
- **Arrow Keys**: Move up/down lists, expand (`Right`) or collapse (`Left`) trees.
- **Enter**: Open the selected channel or chat.
- **Tab**: Switch focus between the conversations tree and messages pane.
- **Esc**: Go back one level or return to the conversations tree.
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
