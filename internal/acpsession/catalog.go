package acpsession

import (
	"strings"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
)

// CatalogEntry is one selectable model from session/new.
type CatalogEntry struct {
	Display   string
	ValueID   string
	Value     string
	IsDefault bool
}

// ModelCatalog indexes models from session/new.
type ModelCatalog struct {
	Entries []CatalogEntry
}

// BuildModelCatalog extracts models from session/new result.
func BuildModelCatalog(sn *acp.SessionNewResult) *ModelCatalog {
	if sn == nil {
		return &ModelCatalog{}
	}
	seen := map[string]bool{}
	var entries []CatalogEntry

	for _, opt := range sn.ConfigOptions {
		if opt.ID != "model" {
			continue
		}
		for _, v := range opt.Options {
			display := firstNonEmpty(v.Name, v.Value, v.ValueID)
			if display == "" {
				continue
			}
			key := strings.ToLower(display)
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, CatalogEntry{
				Display:   display,
				ValueID:   v.ValueID,
				Value:     v.Value,
				IsDefault: v.IsDefault,
			})
		}
	}

	if sn.Models != nil {
		for _, m := range sn.Models.AvailableModels {
			display := firstNonEmpty(m.Name, m.ModelID)
			if display == "" {
				continue
			}
			key := strings.ToLower(display)
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, CatalogEntry{
				Display: display,
				ValueID: m.ModelID,
				Value:   m.ModelID,
			})
		}
	}

	return &ModelCatalog{Entries: entries}
}

// PublicIDs returns display ids suitable for /v1/models.
func (c *ModelCatalog) PublicIDs() []string {
	if c == nil || len(c.Entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.Entries))
	for _, e := range c.Entries {
		id := firstNonEmpty(e.Display, e.Value, e.ValueID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// Resolve maps a requested alias to valueId and legacy value fields.
func (c *ModelCatalog) Resolve(requested string) (valueID, value string, found bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.EqualFold(requested, "auto") {
		if c == nil {
			return "", "", false
		}
		for _, e := range c.Entries {
			if e.IsDefault {
				return firstNonEmpty(e.ValueID, e.Value, e.Display), firstNonEmpty(e.Value, e.ValueID, e.Display), true
			}
		}
		return "", "", false
	}
	if c == nil {
		return requested, requested, true
	}
	want := strings.ToLower(requested)
	for _, e := range c.Entries {
		for _, candidate := range []string{e.Display, e.Value, e.ValueID} {
			if candidate != "" && strings.EqualFold(candidate, want) {
				return firstNonEmpty(e.ValueID, e.Value, e.Display), firstNonEmpty(e.Value, e.ValueID, e.Display), true
			}
		}
	}
	return requested, requested, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
