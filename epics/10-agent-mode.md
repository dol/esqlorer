# Epic 10: CLI Agent Mode

Agent-optimized CLI behavior inspired by `dash0-cli`.

## Tasks

- [x] Add `--agent-mode` flag
- [x] Add `ESQLORER_AGENT_MODE=0|1|false|true` override
- [x] Auto-detect known AI agent environments
- [x] Return `--help` as structured JSON in agent mode
- [x] Emit CLI errors as JSON on stderr in agent mode
- [x] Default `query` output to JSON in agent mode
- [x] Disable ANSI color output in agent mode
- [x] Document that confirmation skipping is reserved for future commands with prompts

## Notes

- Follow the `dash0-cli` priority order: explicit env disable, explicit flag enable, explicit env enable, then auto-detection
- Keep human-readable output unchanged outside agent mode
- Apply agent mode globally at the root command so subcommands inherit it

## Status: ✅ COMPLETED
