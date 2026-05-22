package cursor

import "testing"

func TestPhaseTrackerPendingAndOutput(t *testing.T) {
	pt := NewPhaseTracker()
	if got := pt.OnAssistantDelta("x"); got != 0 {
		t.Fatalf("pending got %d", got)
	}
	pt.OnToolCall()
	if got := pt.OnAssistantDelta("y"); got != 2 {
		t.Fatalf("output got %d", got)
	}
}

func TestPhaseTrackerResolveNoTools(t *testing.T) {
	pt := NewPhaseTracker()
	_ = pt.OnAssistantDelta("only")
	if pt.HasTools() {
		t.Fatal("unexpected tools")
	}
	if wasThinking := pt.ResolvePending(); wasThinking {
		t.Fatal("expected output path")
	}
	if got := pt.OnAssistantDelta("more"); got != 2 {
		t.Fatalf("expected output phase, got %d", got)
	}
}
