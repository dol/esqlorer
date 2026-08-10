package agentmode

import "testing"

func TestEnvBool(t *testing.T) {
	tests := []struct {
		input   string
		value   bool
		matched bool
	}{
		{input: "true", value: true, matched: true},
		{input: "1", value: true, matched: true},
		{input: "false", value: false, matched: true},
		{input: "0", value: false, matched: true},
		{input: "", value: false, matched: false},
	}

	for _, tt := range tests {
		value, matched := envBool(tt.input)
		if value != tt.value || matched != tt.matched {
			t.Fatalf("envBool(%q) = (%v, %v), want (%v, %v)", tt.input, value, matched, tt.value, tt.matched)
		}
	}
}

func TestHasAgentModeFlag(t *testing.T) {
	if !hasAgentModeFlag([]string{"query", "--agent-mode"}) {
		t.Fatal("expected --agent-mode to enable agent mode")
	}
	if hasAgentModeFlag([]string{"query", "--agent-mode=false"}) {
		t.Fatal("expected --agent-mode=false to not enable agent mode")
	}
}

func TestDetectedAgentEnvironment(t *testing.T) {
	getenv := func(key string) string {
		if key == "CODEX" {
			return "1"
		}
		return ""
	}

	if !detectedAgentEnvironment(getenv) {
		t.Fatal("expected CODEX to trigger agent mode detection")
	}
}
