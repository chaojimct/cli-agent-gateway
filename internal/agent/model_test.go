package agent

import "testing"

func TestParseModel(t *testing.T) {
	tests := []struct {
		in, def, wantAgent, wantModel string
	}{
		{"cursor/composer-2.5-fast", "cursor", "cursor", "composer-2.5-fast"},
		{"claude/sonnet", "cursor", "claude", "sonnet"},
		{"composer-2.5-fast", "cursor", "cursor", "composer-2.5-fast"},
		{"", "cursor", "cursor", "auto"},
	}
	for _, tc := range tests {
		agent, model := ParseModel(tc.in, tc.def)
		if agent != tc.wantAgent || model != tc.wantModel {
			t.Fatalf("ParseModel(%q): got %q/%q want %q/%q", tc.in, agent, model, tc.wantAgent, tc.wantModel)
		}
	}
}

func TestFormatModel(t *testing.T) {
	if got := FormatModel("cursor", "auto"); got != "cursor/auto" {
		t.Fatalf("FormatModel: %q", got)
	}
}
