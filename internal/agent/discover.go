package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/cursor-gateway/internal/acp"
	"github.com/user/cursor-gateway/internal/config"
)

const (
	probeTimeout       = 4 * time.Second
	probeTimeoutBridge = 120 * time.Second
)

// Discover finds installed ACP agents and verifies ACP stdio support.
func Discover(ctx context.Context, cfg *config.Config, logger *slog.Logger) []*Profile {
	if cfg == nil {
		return nil
	}
	auto := true
	defaultAgent := "cursor"
	overrides := map[string]config.AgentProfileConfig{}
	if cfg.Agents != nil {
		auto = cfg.Agents.AutoDiscover
		if cfg.Agents.Default != "" {
			defaultAgent = cfg.Agents.Default
		}
		overrides = cfg.Agents.Profiles
	}
	_ = defaultAgent

	seen := map[string]bool{}
	var candidates []Profile
	for _, def := range knownDefs {
		ov, hasOverride := overrides[def.ID]
		if hasOverride && ov.Enabled != nil && !*ov.Enabled {
			continue
		}
		if !auto && !hasOverride {
			continue
		}
		profile := buildKnownProfile(def, ov, cfg)
		if profile.Command() == "" {
			continue
		}
		candidates = append(candidates, profile)
		seen[def.ID] = true
	}

	for id, ov := range overrides {
		if seen[id] {
			continue
		}
		if ov.Enabled != nil && !*ov.Enabled {
			continue
		}
		if !auto && ov.Spawn == nil && ov.BinaryPath == "" {
			continue
		}
		profile := buildCustomProfile(id, ov, cfg)
		if profile.Command() == "" {
			continue
		}
		candidates = append(candidates, profile)
		seen[id] = true
	}

	var mu sync.Mutex
	var out []*Profile
	var wg sync.WaitGroup
	for _, profile := range candidates {
		wg.Add(1)
		go func(p Profile) {
			defer wg.Done()
			if !probeACP(ctx, p, cfg, logger) {
				if logger != nil {
					logger.Debug("acp agent probe failed", "agent", p.ID, "command", p.Command())
				}
				IncProbeFailure()
				return
			}
			mu.Lock()
			out = append(out, &p)
			mu.Unlock()
			if logger != nil {
				logger.Info("discovered acp agent", "agent", p.ID, "command", p.Command())
			}
		}(profile)
	}
	wg.Wait()
	return out
}

func buildKnownProfile(def agentDef, ov config.AgentProfileConfig, cfg *config.Config) Profile {
	binary := resolveBinary(def.ID, def.Commands, def.WindowsPaths, ov, cfg)
	spawnCmd := def.SpawnCommand
	spawnArgs := append([]string{}, def.SpawnArgs...)
	if ov.Spawn != nil {
		if ov.Spawn.Command != "" {
			spawnCmd = ov.Spawn.Command
			binary = ov.Spawn.Command
		}
		if len(ov.Spawn.Args) > 0 {
			spawnArgs = append([]string{}, ov.Spawn.Args...)
		}
	}
	if spawnCmd == "" {
		spawnCmd = binary
	}

	profile := Profile{
		ID:                  def.ID,
		Name:                def.Name,
		Transport:           "acp",
		Binary:              binary,
		ACPArgs:             append([]string{}, def.ACPArgs...),
		SpawnCommand:        spawnCmd,
		SpawnArgs:           spawnArgs,
		AuthMethod:          def.AuthMethod,
		SkipAuthEnv:         append([]string{}, def.SkipAuthEnv...),
		ListModelsArgs:      append([]string{}, def.ListModelsArgs...),
		StaticModels:        append([]string{}, def.StaticModels...),
		ModelsSource:        def.ModelsSource,
		ExpectedAuthMethods: append([]string{}, def.ExpectedAuthMethods...),
		RejectAuthMethods:   append([]string{}, def.RejectAuthMethods...),
		ConfigKeys:          append([]string{}, def.ConfigKeys...),
		CursorExtensions:    def.CursorExtensions,
		ArgsComplete:        def.ArgsComplete,
	}
	if def.ID == "claude" {
		resolveClaudeSpawn(&profile, ov)
	}
	applyProfileOverride(&profile, ov)
	return profile
}

func buildCustomProfile(id string, ov config.AgentProfileConfig, cfg *config.Config) Profile {
	name := ov.Name
	if name == "" {
		name = id
	}
	profile := Profile{
		ID:        id,
		Name:      name,
		Transport: "acp",
		ModelsSource: "session_new",
		ConfigKeys: []string{"model", "mode"},
	}
	if ov.BinaryPath != "" {
		profile.Binary = ov.BinaryPath
		profile.SpawnCommand = ov.BinaryPath
	}
	if ov.Spawn != nil {
		if ov.Spawn.Command != "" {
			profile.SpawnCommand = ov.Spawn.Command
			profile.Binary = ov.Spawn.Command
		}
		if len(ov.Spawn.Args) > 0 {
			profile.SpawnArgs = append([]string{}, ov.Spawn.Args...)
			profile.ACPArgs = append([]string{}, ov.Spawn.Args...)
		}
	}
	if len(ov.ACPArgs) > 0 {
		profile.ACPArgs = append([]string{}, ov.ACPArgs...)
		if len(profile.SpawnArgs) == 0 {
			profile.SpawnArgs = append([]string{}, ov.ACPArgs...)
		}
	}
	if profile.Command() == "" {
		profile.SpawnCommand = resolveBinary(id, []string{id}, nil, ov, cfg)
		profile.Binary = profile.SpawnCommand
	}
	applyProfileOverride(&profile, ov)
	return profile
}

func resolveClaudeSpawn(profile *Profile, ov config.AgentProfileConfig) {
	if ov.Spawn != nil && ov.Spawn.Command != "" {
		return
	}
	if ov.BinaryPath != "" {
		return
	}
	for _, cmd := range []string{"claude-agent-acp", "claude-agent-acp.cmd"} {
		if p, err := exec.LookPath(cmd); err == nil {
			profile.SpawnCommand = p
			profile.Binary = p
			profile.SpawnArgs = nil
			profile.ACPArgs = nil
			profile.ArgsComplete = true
			return
		}
	}
}

func applyProfileOverride(profile *Profile, ov config.AgentProfileConfig) {
	if ov.Prefix != "" {
		profile.ID = ov.Prefix
	}
	if ov.Name != "" {
		profile.Name = ov.Name
	}
	if ov.AuthMethod != "" {
		profile.AuthMethod = ov.AuthMethod
	}
	if len(ov.StaticModels) > 0 {
		profile.StaticModels = append([]string{}, ov.StaticModels...)
	}
	if ov.Models != nil && ov.Models.Source != "" {
		profile.ModelsSource = ov.Models.Source
	}
	if ov.Probe != nil {
		if len(ov.Probe.ExpectedAuthMethods) > 0 {
			profile.ExpectedAuthMethods = append([]string{}, ov.Probe.ExpectedAuthMethods...)
		}
		if len(ov.Probe.RejectAuthMethods) > 0 {
			profile.RejectAuthMethods = append([]string{}, ov.Probe.RejectAuthMethods...)
		}
		if len(ov.Probe.ExpectedAgentNames) > 0 {
			profile.ExpectedAgentNames = append([]string{}, ov.Probe.ExpectedAgentNames...)
		}
	}
	if ov.Spawn != nil && len(ov.Spawn.Args) > 0 {
		profile.ArgsComplete = true
	}
	if ov.Extensions != nil {
		profile.CursorExtensions = *ov.Extensions
	}
	if len(ov.Env) > 0 {
		if profile.SpawnEnv == nil {
			profile.SpawnEnv = make(map[string]string, len(ov.Env))
		}
		for k, v := range ov.Env {
			profile.SpawnEnv[k] = resolveEnvTemplate(v)
		}
	}
}

func resolveEnvTemplate(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return os.Getenv(v[2 : len(v)-1])
	}
	return v
}

func resolveBinary(id string, commands, windowsPaths []string, ov config.AgentProfileConfig, cfg *config.Config) string {
	if ov.BinaryPath != "" {
		if st, err := os.Stat(ov.BinaryPath); err == nil && !st.IsDir() {
			return ov.BinaryPath
		}
	}
	if ov.Spawn != nil && ov.Spawn.Command != "" {
		if p, err := exec.LookPath(ov.Spawn.Command); err == nil {
			return p
		}
		return ov.Spawn.Command
	}
	if id == "cursor" && cfg != nil && cfg.Cursor.BinaryPath != "" {
		if st, err := os.Stat(cfg.Cursor.BinaryPath); err == nil && !st.IsDir() {
			return cfg.Cursor.BinaryPath
		}
	}
	for _, cmd := range commands {
		if p, err := exec.LookPath(cmd); err == nil {
			return p
		}
	}
	if filepath.Separator == '\\' {
		for _, p := range windowsPaths {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
		if id == "cursor" {
			if local := os.Getenv("LOCALAPPDATA"); local != "" {
				p := filepath.Join(local, "cursor-agent", "cursor-agent.cmd")
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
	}
	return ""
}

func probeTimeoutFor(p Profile) time.Duration {
	cmd := strings.ToLower(p.Command())
	for _, a := range p.Args() {
		lower := strings.ToLower(a)
		if strings.Contains(lower, "claude-agent-acp") || strings.Contains(lower, "@agentclientprotocol/") {
			return probeTimeoutBridge
		}
	}
	if cmd == "npx" || strings.HasSuffix(cmd, "npx.cmd") {
		return probeTimeoutBridge
	}
	return probeTimeout
}

func probeACP(ctx context.Context, p Profile, cfg *config.Config, logger *slog.Logger) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeoutFor(p))
	defer cancel()

	c, err := acp.NewClient(ctx, p.ACPConfig(&cfg.Cursor, logger, p.ShouldSkipAuth()))
	if err != nil {
		return false
	}
	defer c.Close()

	init, err := c.BootstrapInit(ctx)
	if err != nil {
		return false
	}
	if !ValidateProbe(p, init) {
		return false
	}
	return true
}

// SpawnEnv builds env for discover probe (re-exported from proxyenv.go).
func spawnEnv(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return ProxyEnv(&cfg.Cursor)
}

func cursorEnv() []string {
	var env []string
	if k := strings.TrimSpace(os.Getenv("CURSOR_API_KEY")); k != "" {
		env = append(env, "CURSOR_API_KEY="+k, "CURSOR_AUTH_TOKEN="+k)
	}
	return env
}
