package webui

import "testing"

func TestCompareTraces(t *testing.T) {
	s := NewStore(10, nil)
	s.StartTrace("a", "/v1/chat/completions", "auto", "model", nil, "")
	s.CompleteTrace("a", nil, 100, "hello")
	s.StartTrace("b", "/v1/chat/completions", "auto", "model", nil, "")
	s.CompleteTrace("b", nil, 200, "hello world")

	cmp, err := s.Compare("a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if cmp.TraceA.ID != "a" || cmp.TraceB.ID != "b" {
		t.Fatalf("ids=%s %s", cmp.TraceA.ID, cmp.TraceB.ID)
	}
}
