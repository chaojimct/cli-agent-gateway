package acp

import (
	"context"
	"encoding/json"
)

// Bootstrap runs initialize and optional authenticate.
func (c *Client) Bootstrap(ctx context.Context) error {
	_, err := c.bootstrap(ctx)
	return err
}

// BootstrapInit runs initialize and returns the agent handshake result.
func (c *Client) BootstrapInit(ctx context.Context) (*InitializeResult, error) {
	return c.bootstrap(ctx)
}

func (c *Client) bootstrap(ctx context.Context) (*InitializeResult, error) {
	raw, err := c.Request(ctx, "initialize", InitializeParams{
		ProtocolVersion: 1,
		ClientInfo: ClientInfo{
			Name:    "cursor-gateway",
			Version: "2.0.0",
		},
		ClientCapabilities: ClientCapabilities{
			FS: &FSCapabilities{ReadTextFile: false, WriteTextFile: false},
		},
	})
	if err != nil {
		return nil, err
	}
	var init InitializeResult
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &init)
	}
	c.initResult = &init
	if c.skipAuthenticate {
		return &init, nil
	}
	authMethod := c.authMethod
	if authMethod == "" && len(init.AuthMethods) > 0 {
		authMethod = init.AuthMethods[0].ID
	}
	if authMethod == "" {
		return &init, nil
	}
	_, err = c.Request(ctx, "authenticate", AuthenticateParams{MethodID: authMethod})
	if err != nil {
		return &init, err
	}
	return &init, nil
}

// InitResult returns the cached initialize result after Bootstrap.
func (c *Client) InitResult() *InitializeResult {
	return c.initResult
}
