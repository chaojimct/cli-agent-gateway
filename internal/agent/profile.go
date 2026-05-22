package agent

import (
	"os"
	"strings"

	"github.com/user/cursor-gateway/internal/acp"
)

// Profile describes how to spawn and talk to one ACP agent.
type Profile struct {
	ID       string
	Name     string
	Transport string // acp (MVP)

	Binary  string
	ACPArgs []string

	SpawnCommand string
	SpawnArgs    []string
	SpawnEnv     map[string]string

	AuthMethod  string
	SkipAuthEnv []string

	ListModelsArgs []string
	StaticModels   []string
	ModelsSource   string // cli | session_new | static

	ExpectedAuthMethods []string
	RejectAuthMethods   []string
	ExpectedAgentNames  []string

	ConfigKeys       []string // e.g. model, mode
	CursorExtensions bool
	ArgsComplete     bool // spawn args are final (npx bridge); do not append "acp"
}

// Command returns the process to spawn.
func (p Profile) Command() string {
	if p.SpawnCommand != "" {
		return p.SpawnCommand
	}
	return p.Binary
}

// Args returns argv for the ACP child.
func (p Profile) Args() []string {
	if len(p.SpawnArgs) > 0 {
		return append([]string{}, p.SpawnArgs...)
	}
	return append([]string{}, p.ACPArgs...)
}

// ShouldSkipAuth returns true when env indicates auth can be skipped.
func (p Profile) ShouldSkipAuth() bool {
	for _, key := range p.SkipAuthEnv {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// WantsConfigKey reports whether a session config key should be applied.
func (p Profile) WantsConfigKey(key string) bool {
	if len(p.ConfigKeys) == 0 {
		return key == "model" || key == "mode"
	}
	for _, k := range p.ConfigKeys {
		if k == key {
			return true
		}
	}
	return false
}

// ResolveAgentModel maps a gateway model name to the agent-native model id.
func (p Profile) ResolveAgentModel(requested string) string {
	if requested == "" || requested == "auto" {
		if p.ID == "cursor" || p.UseCursorModelResolver() {
			return "default[]"
		}
		return "auto"
	}
	if p.UseCursorModelResolver() {
		return acp.ResolveModel(requested)
	}
	return requested
}

func (p Profile) UseCursorModelResolver() bool {
	return p.ID == "cursor" || p.CursorExtensions
}

// agentDef is a built-in agent template.
type agentDef struct {
	ID                  string
	Name                string
	Commands            []string
	WindowsPaths        []string
	ACPArgs             []string
	SpawnCommand        string
	SpawnArgs           []string
	AuthMethod          string
	SkipAuthEnv         []string
	ListModelsArgs      []string
	StaticModels        []string
	ModelsSource        string
	ExpectedAuthMethods []string
	RejectAuthMethods   []string
	ConfigKeys          []string
	CursorExtensions    bool
	ArgsComplete        bool
}

// knownDefs are built-in ACP agent definitions scanned on the host.
var knownDefs = []agentDef{
	{
		ID:               "cursor",
		Name:             "Cursor Agent",
		Commands:         []string{"cursor-agent", "cursor-agent.cmd", "cursor-agent.ps1"},
		ACPArgs:          []string{"acp"},
		AuthMethod:       "cursor_login",
		SkipAuthEnv:      []string{"CURSOR_API_KEY", "CURSOR_AUTH_TOKEN"},
		ListModelsArgs:   []string{"--list-models"},
		ModelsSource:     "cli",
		ConfigKeys:       []string{"model", "mode"},
		CursorExtensions: true,
	},
	{
		ID:                "claude",
		Name:              "Claude Code",
		Commands:          []string{"npx", "npx.cmd"},
		SpawnCommand:      "npx",
		SpawnArgs:         []string{"-y", "@agentclientprotocol/claude-agent-acp"},
		SkipAuthEnv:       []string{"ANTHROPIC_API_KEY"},
		ModelsSource:      "session_new",
		RejectAuthMethods: []string{"cursor_login"},
		ConfigKeys:        []string{"model"},
		ArgsComplete:      true,
		StaticModels:      []string{"sonnet", "opus", "haiku"},
	},
}
