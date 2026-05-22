package handler

import (
	"testing"

	"github.com/chaojimct/cli-agent-gateway/internal/translator"
)

func TestDeriveConversationIDStable(t *testing.T) {
	msgs := []translator.OpenAIChatMessage{
		{Role: "system", Content: "You are opencode"},
		{Role: "user", Content: "现在几点"},
	}
	a := deriveConversationID(msgs)
	b := deriveConversationID(append(msgs,
		translator.OpenAIChatMessage{Role: "assistant", Content: "4"},
		translator.OpenAIChatMessage{Role: "user", Content: "谢谢"},
	))
	if a == "" {
		t.Fatal("expected non-empty id")
	}
	if a != b {
		t.Fatalf("thread key should be stable: %q vs %q", a, b)
	}
	if len(a) < 10 || a[:5] != "auto:" {
		t.Fatalf("unexpected format: %q", a)
	}
}

func TestDeriveConversationIDDifferentThreads(t *testing.T) {
	a := deriveConversationID([]translator.OpenAIChatMessage{
		{Role: "system", Content: "You are opencode"},
		{Role: "user", Content: "查时间"},
	})
	b := deriveConversationID([]translator.OpenAIChatMessage{
		{Role: "system", Content: "You are a title generator"},
		{Role: "user", Content: "查时间"},
	})
	if a == b {
		t.Fatal("different system prompts should not share thread key")
	}
}

func TestResolveConversationIDExplicitWins(t *testing.T) {
	req := &translator.OpenAIChatRequest{
		Messages: []translator.OpenAIChatMessage{{Role: "user", Content: "hi"}},
		Metadata: map[string]interface{}{"conversation_id": "thread-1"},
	}
	got := resolveConversationID(nil, req)
	if got.ID != "thread-1" || got.Source != "explicit" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveConversationIDAuto(t *testing.T) {
	req := &translator.OpenAIChatRequest{
		Messages: []translator.OpenAIChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
		},
	}
	got := resolveConversationID(nil, req)
	if got.ID == "" || got.Source != "auto" {
		t.Fatalf("got %+v", got)
	}
}
