package config

import "testing"

func TestDefaultModelProfile(t *testing.T) {
	cfg := Default()
	if cfg.Cursor.AgentProfile != "model" {
		t.Fatalf("agent_profile=%q", cfg.Cursor.AgentProfile)
	}
	if cfg.Cursor.Force {
		t.Fatal("force should be false by default")
	}
	if cfg.Cursor.UseDaemon {
		t.Fatal("use_daemon is deprecated in ACP v2 and should default false")
	}
	if !cfg.Session.Enabled {
		t.Fatal("session should be enabled by default")
	}
}

func TestIsModelProfile(t *testing.T) {
	cfg := Default()
	if !cfg.Cursor.IsModelProfile() {
		t.Fatal("expected model profile")
	}
	cfg.Cursor.AgentProfile = "agent"
	if cfg.Cursor.IsModelProfile() {
		t.Fatal("expected agent profile")
	}
}
