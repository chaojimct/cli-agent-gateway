package acpsession

import (
	"testing"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
)

func TestBuildModelCatalogFromConfigOptions(t *testing.T) {
	sn := &acp.SessionNewResult{
		ConfigOptions: []acp.ConfigOption{
			{
				ID: "model",
				Options: []acp.ConfigValue{
					{Value: "sonnet", ValueID: "claude-sonnet-4-6", IsDefault: true},
					{Value: "opus", ValueID: "claude-opus-4-6"},
				},
			},
		},
	}
	c := BuildModelCatalog(sn)
	valueID, value, ok := c.Resolve("sonnet")
	if !ok || valueID != "claude-sonnet-4-6" || value != "sonnet" {
		t.Fatalf("resolve sonnet: valueID=%q value=%q ok=%v", valueID, value, ok)
	}
	ids := c.PublicIDs()
	if len(ids) != 2 {
		t.Fatalf("public ids = %v", ids)
	}
}

func TestCatalogResolveAutoUsesDefault(t *testing.T) {
	sn := &acp.SessionNewResult{
		ConfigOptions: []acp.ConfigOption{
			{
				ID: "model",
				Options: []acp.ConfigValue{
					{Value: "haiku", ValueID: "claude-haiku-4-5"},
					{Value: "sonnet", ValueID: "claude-sonnet-4-6", IsDefault: true},
				},
			},
		},
	}
	c := BuildModelCatalog(sn)
	valueID, _, ok := c.Resolve("auto")
	if !ok || valueID != "claude-sonnet-4-6" {
		t.Fatalf("auto resolve = %q ok=%v", valueID, ok)
	}
}

func TestCatalogResolveByValueID(t *testing.T) {
	sn := &acp.SessionNewResult{
		Models: &acp.ModelList{
			AvailableModels: []acp.ModelDescriptor{
				{ModelID: "gpt-5", Name: "GPT 5"},
			},
		},
	}
	c := BuildModelCatalog(sn)
	valueID, _, ok := c.Resolve("gpt-5")
	if !ok || valueID != "gpt-5" {
		t.Fatalf("resolve gpt-5 = %q ok=%v", valueID, ok)
	}
}
