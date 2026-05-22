package session

import (
	"testing"
	"time"
)

func TestPoolAgentMismatchDeletesStale(t *testing.T) {
	p := NewPool(time.Minute, 8)
	p.Put("conv-1", "cursor", "sess-c", "auto", "ask", "/ws", "hash", 2)

	if _, ok := p.Get("conv-1", "claude"); ok {
		t.Fatal("expected stale entry removed on agent mismatch")
	}
	if _, ok := p.Get("conv-1", "cursor"); ok {
		t.Fatal("entry should be deleted after mismatch")
	}
}

func TestPoolGetByAgent(t *testing.T) {
	p := NewPool(time.Minute, 8)
	p.Put("conv-1", "cursor", "sess-c", "auto", "ask", "/ws", "hash", 2)

	e, ok := p.Get("conv-1", "cursor")
	if !ok || e.SessionID != "sess-c" || e.AgentID != "cursor" {
		t.Fatalf("unexpected entry: ok=%v entry=%+v", ok, e)
	}
}

func TestPoolDelete(t *testing.T) {
	p := NewPool(time.Minute, 8)
	p.Put("conv-1", "cursor", "sess-c", "auto", "ask", "/ws", "hash", 1)
	p.Delete("conv-1")
	if p.Active() != 0 {
		t.Fatalf("active=%d want 0", p.Active())
	}
}

func TestPoolInvalidateAll(t *testing.T) {
	p := NewPool(time.Minute, 8)
	p.Put("conv-1", "cursor", "sess-c", "auto", "ask", "/ws", "hash", 1)
	p.Put("conv-2", "claude", "sess-a", "sonnet", "ask", "/ws2", "hash2", 1)
	p.InvalidateAll()
	if p.Active() != 0 {
		t.Fatalf("active=%d want 0", p.Active())
	}
}
