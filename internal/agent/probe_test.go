package agent

import (
	"testing"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
)

func TestValidateProbeRejectsCursorLoginForClaude(t *testing.T) {
	p := Profile{
		ID:                "claude",
		RejectAuthMethods: []string{"cursor_login"},
	}
	init := &acp.InitializeResult{
		AgentInfo:   acp.AgentInfo{Name: "Cursor Agent"},
		AuthMethods: []acp.AuthMethod{{ID: "cursor_login"}},
	}
	if ValidateProbe(p, init) {
		t.Fatal("expected claude probe to reject cursor_login agent")
	}
}

func TestValidateProbeAcceptsClaudeBridge(t *testing.T) {
	p := Profile{
		ID:                "claude",
		RejectAuthMethods: []string{"cursor_login"},
	}
	init := &acp.InitializeResult{
		AgentInfo:   acp.AgentInfo{Name: "Claude Code"},
		AuthMethods: []acp.AuthMethod{{ID: "api_key"}},
	}
	if !ValidateProbe(p, init) {
		t.Fatal("expected claude probe to accept non-cursor agent")
	}
}

func TestValidateProbeRequiresCursorLogin(t *testing.T) {
	p := Profile{ID: "cursor"}
	init := &acp.InitializeResult{
		AgentInfo:   acp.AgentInfo{Name: "Other Agent"},
		AuthMethods: []acp.AuthMethod{{ID: "api_key"}},
	}
	if ValidateProbe(p, init) {
		t.Fatal("expected cursor probe to reject non-cursor agent")
	}
}

func TestValidateProbeAcceptsCursorAgent(t *testing.T) {
	p := Profile{ID: "cursor"}
	init := &acp.InitializeResult{
		AgentInfo:   acp.AgentInfo{Name: "Cursor Agent"},
		AuthMethods: []acp.AuthMethod{{ID: "cursor_login"}},
	}
	if !ValidateProbe(p, init) {
		t.Fatal("expected cursor probe to accept cursor agent")
	}
}

func TestProfileCommandAndArgs(t *testing.T) {
	p := Profile{
		Binary:       "cursor-agent",
		ACPArgs:      []string{"acp"},
		SpawnCommand: "npx",
		SpawnArgs:    []string{"-y", "@agentclientprotocol/claude-agent-acp"},
	}
	if p.Command() != "npx" {
		t.Fatalf("command = %q", p.Command())
	}
	if len(p.Args()) != 2 || p.Args()[0] != "-y" {
		t.Fatalf("args = %v", p.Args())
	}
}
