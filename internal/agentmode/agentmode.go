package agentmode

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const EnvVar = "ESQLORER_AGENT_MODE"

var detectedEnvVars = []string{
	"AIDER",
	"CLAUDE_CODE",
	"CLAUDECODE",
	"CLINE",
	"CLINE_TASK_ID",
	"CODEX",
	"CURSOR_AGENT",
	"CURSOR_SESSION_ID",
	"GITHUB_COPILOT",
	"MCP_SESSION_ID",
	"OPENAI_CODEX",
	"WINDSURF_AGENT",
	"WINDSURF_SESSION_ID",
}

func EnabledForCommand(cmd *cobra.Command) bool {
	if value, ok := envBool(os.Getenv(EnvVar)); ok {
		return value
	}

	if cmd != nil {
		if flag := cmd.Flags().Lookup("agent-mode"); flag != nil {
			if enabled, err := cmd.Flags().GetBool("agent-mode"); err == nil && enabled {
				return true
			}
		}
	}

	if detectedAgentEnvironment(os.Getenv) {
		return true
	}

	return false
}

func EnabledForArgs(args []string) bool {
	if value, ok := envBool(os.Getenv(EnvVar)); ok {
		return value
	}

	if hasAgentModeFlag(args) {
		return true
	}

	if detectedAgentEnvironment(os.Getenv) {
		return true
	}

	return false
}

func envBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	default:
		return false, false
	}
}

func hasAgentModeFlag(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--agent-mode", "--agent-mode=true", "--agent-mode=1":
			return true
		case "--agent-mode=false", "--agent-mode=0":
			return false
		}
	}
	return false
}

func detectedAgentEnvironment(getenv func(string) string) bool {
	for _, key := range detectedEnvVars {
		if strings.TrimSpace(getenv(key)) != "" {
			return true
		}
	}
	return false
}
