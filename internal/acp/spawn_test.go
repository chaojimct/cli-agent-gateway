package acp

import "testing"

func TestShouldUseCursorNodeLayoutBareNpx(t *testing.T) {
	if shouldUseCursorNodeLayout("npx") {
		t.Fatal("bare npx must not use cursor-agent node layout")
	}
}

func TestShouldUseCursorNodeLayoutCursorAgent(t *testing.T) {
	if !shouldUseCursorNodeLayout("cursor-agent") {
		t.Fatal("cursor-agent should use node layout on Windows")
	}
	if !shouldUseCursorNodeLayout("") {
		t.Fatal("empty path should default to cursor-agent layout")
	}
}

func TestShouldUseCursorNodeLayoutClaudeBridge(t *testing.T) {
	if shouldUseCursorNodeLayout("claude-agent-acp.cmd") {
		t.Fatal("claude bridge cmd must not use cursor node layout")
	}
	if shouldUseCursorNodeLayout("npx.cmd") {
		t.Fatal("npx.cmd must not use cursor node layout")
	}
}

func TestResolveSpawnDoesNotRewriteBareNpx(t *testing.T) {
	exe, args := ResolveSpawn("npx", []string{"-y", "claude-agent-acp@latest"}, true)
	if exe != "npx" {
		t.Fatalf("exe=%q want npx", exe)
	}
	if len(args) < 2 || args[0] != "-y" {
		t.Fatalf("unexpected args: %v", args)
	}
}
