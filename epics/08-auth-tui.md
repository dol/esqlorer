# Epic 8: Interactive Auth TUI

Interactive TUI for adding server configurations with Bubble Tea.

## Tasks

- [x] Create cmd/auth/tui/model.go
- [x] Multi-step form (Name, URL, Auth Method, Credentials)
- [x] Password masking with EchoPassword
- [x] Fallback to CLI prompts when no TTY

## Notes

- Uses bubbles/textinput with EchoPassword for masking
- Auto-detects TTY for CLI vs TUI mode
- In TTY: runs Bubble Tea TUI
- In non-TTY: falls back to CLI prompts

## Status: ✅ COMPLETED