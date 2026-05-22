package agent

import (
	"context"

	"github.com/user/cursor-gateway/internal/acp"
	"github.com/user/cursor-gateway/internal/acpsession"
	"github.com/user/cursor-gateway/internal/config"
	"github.com/user/cursor-gateway/internal/workspace"
)

// FetchModelsFromSession probes session/new for available models.
func FetchModelsFromSession(ctx context.Context, p *Profile, cfg *config.CursorConfig) ([]string, error) {
	if p == nil {
		return nil, nil
	}
	c, err := acp.NewClient(ctx, p.ACPConfig(cfg, nil, p.ShouldSkipAuth()))
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Bootstrap(ctx); err != nil {
		return nil, err
	}
	sess, err := acpsession.NewSession(ctx, c, workspace.Effective(cfg, ""))
	if err != nil {
		return nil, err
	}
	return sess.Catalog.PublicIDs(), nil
}

// ACPSessionProfile maps agent Profile to acpsession config hooks.
func ACPSessionProfile(p *Profile) acpsession.ProfileConfig {
	if p == nil {
		return acpsession.ProfileConfig{}
	}
	return acpsession.ProfileConfig{
		ResolveModel:          p.ResolveAgentModel,
		WantsConfigKey:        p.WantsConfigKey,
		UseCursorModeFallback: p.UseCursorModelResolver(),
	}
}
