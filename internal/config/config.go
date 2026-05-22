package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Cursor  CursorConfig  `yaml:"cursor"`
	Agents  *AgentsConfig `yaml:"agents"`
	Session SessionConfig `yaml:"session"`
	Auth    AuthConfig    `yaml:"auth"`
	Log     LogConfig     `yaml:"logging"`
	WebUI   WebUIConfig   `yaml:"webui"`
	Admin   AdminConfig   `yaml:"admin"`
}

type AgentsConfig struct {
	AutoDiscover       bool                          `yaml:"auto_discover"`
	Default            string                        `yaml:"default"`
	FallbackToDefault  bool                          `yaml:"fallback_to_default"`
	ModelCacheTTL      time.Duration                 `yaml:"model_cache_ttl"`
	Profiles           map[string]AgentProfileConfig `yaml:"profiles"`
}

func (c *AgentsConfig) AllowFallbackToDefault() bool {
	return c != nil && c.FallbackToDefault
}

type AgentProfileConfig struct {
	Enabled      *bool              `yaml:"enabled"`
	Name         string             `yaml:"name"`
	BinaryPath   string             `yaml:"binary_path"`
	Prefix       string             `yaml:"prefix"`
	AuthMethod   string             `yaml:"auth_method"`
	ACPArgs      []string           `yaml:"acp_args"`
	StaticModels []string           `yaml:"static_models"`
	Spawn        *AgentSpawnConfig  `yaml:"spawn"`
	Env          map[string]string  `yaml:"env"`
	Probe        *AgentProbeConfig  `yaml:"probe"`
	Models       *AgentModelsConfig `yaml:"models"`
	Extensions   *bool              `yaml:"cursor_extensions"`
}

type AgentSpawnConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type AgentProbeConfig struct {
	ExpectedAuthMethods []string `yaml:"expected_auth_methods"`
	RejectAuthMethods   []string `yaml:"reject_auth_methods"`
	ExpectedAgentNames  []string `yaml:"expected_agent_names"`
}

type AgentModelsConfig struct {
	Source string `yaml:"source"` // cli | session_new | static
}

type ServerConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	MaxRequestBody  int64         `yaml:"max_request_body"`
	AllowedOrigins  []string      `yaml:"allowed_origins"`
}

type CursorConfig struct {
	BinaryPath           string        `yaml:"binary_path"`
	DefaultModel         string        `yaml:"default_model"`
	RequestTimeout       time.Duration `yaml:"request_timeout"`
	MaxConcurrent        int           `yaml:"max_concurrent"`
	Workspace            string        `yaml:"workspace"`
	ApproveMCPs          bool          `yaml:"approve_mcps"`
	Force                bool          `yaml:"force"`
	Proxy                string        `yaml:"proxy"`
	UseDaemon            bool          `yaml:"use_daemon"`
	ThinkingVisibility   string        `yaml:"thinking_visibility"`
	AgentProfile         string        `yaml:"agent_profile"` // model | agent
	AgentMode            string        `yaml:"agent_mode"`    // plan | ask
	ModelWorkspace       string        `yaml:"model_workspace"`
	OnToolCall           string        `yaml:"on_tool_call"` // abort | warn
	ModelSystemPreamble  bool          `yaml:"model_system_preamble"`
	ModelRetryOnTool     bool          `yaml:"model_retry_on_tool"`
	StreamPendingMode    string        `yaml:"stream_pending_mode"` // optimistic | buffer
	ClientToolsMode      string        `yaml:"client_tools_mode"`        // auto | off
	ClientToolsAgentMode string        `yaml:"client_tools_agent_mode"`  // plan | ask — OpenCode 请求用 plan，避免 ask 拒绝执行
}

type SessionConfig struct {
	Enabled     bool          `yaml:"enabled"`
	StoragePath string        `yaml:"storage_path"`
	LockTimeout time.Duration `yaml:"lock_timeout"`
}

type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type WebUIConfig struct {
	StoreRequestBody           bool `yaml:"store_request_body"`
	MaxDeltaBytes              int  `yaml:"max_delta_bytes"`
	MaxTraces                  int  `yaml:"max_traces"`
	AllowUnauthenticatedConfig bool `yaml:"allow_unauthenticated_config"`
}

type AdminConfig struct {
	RestartGrace time.Duration `yaml:"restart_grace"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    300 * time.Second,
			ShutdownTimeout: 10 * time.Second,
			MaxRequestBody:  8 * 1024 * 1024,
		},
		Cursor: CursorConfig{
			BinaryPath:          "cursor-agent",
			DefaultModel:        "cursor/composer-2.5-fast",
			RequestTimeout:      300 * time.Second,
			MaxConcurrent:       8,
			ApproveMCPs:         false,
			Force:               false,
			UseDaemon:           false,
			ThinkingVisibility:  "reasoning_content",
			AgentProfile:        "model",
			AgentMode:           "ask",
			OnToolCall:          "abort",
			ModelSystemPreamble: true,
			ModelRetryOnTool:    true,
			StreamPendingMode:   "optimistic",
			ClientToolsMode:      "auto",
			ClientToolsAgentMode: "ask",
		},
		Session: SessionConfig{
			Enabled:     true,
			StoragePath: "sessions.json",
			LockTimeout: 5 * time.Second,
		},
		Auth: AuthConfig{
			Enabled: false,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		WebUI: WebUIConfig{
			StoreRequestBody:           true,
			MaxDeltaBytes:              131072,
			MaxTraces:                  2000,
			AllowUnauthenticatedConfig: true,
		},
		Admin: AdminConfig{
			RestartGrace: 30 * time.Second,
		},
		Agents: &AgentsConfig{
			AutoDiscover: true,
			Default:      "cursor",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}

		localPath := filepath.Join(filepath.Dir(path), "config.local.yaml")
		if localData, err := os.ReadFile(localPath); err == nil {
			if err := yaml.Unmarshal(localData, cfg); err != nil {
				return nil, fmt.Errorf("parse config.local.yaml: %w", err)
			}
		}
	}

	applyEnvOverrides(cfg)
	applyProfileDefaults(cfg)
	return cfg, nil
}

// IsModelProfile returns true when running as a generic model API.
func (c *CursorConfig) IsModelProfile() bool {
	return c.AgentProfile != "agent"
}

// UseClientToolsPreamble returns whether to inject client-tool instructions.
func (c *CursorConfig) UseClientToolsPreamble() bool {
	return c.ClientToolsMode == "auto" || c.ClientToolsMode == "always"
}

// ClientToolsEnabled reports whether client tools should be active for this request.
func (c *CursorConfig) ClientToolsEnabled(hasToolsInRequest bool) bool {
	switch c.ClientToolsMode {
	case "off", "false", "0":
		return false
	case "always", "true", "1":
		return true
	default: // auto
		return hasToolsInRequest
	}
}

func applyProfileDefaults(cfg *Config) {
	if cfg.Cursor.AgentProfile == "" {
		cfg.Cursor.AgentProfile = "model"
	}
	if cfg.Cursor.AgentMode == "" {
		if cfg.Cursor.IsModelProfile() {
			cfg.Cursor.AgentMode = "ask"
		} else {
			cfg.Cursor.AgentMode = "ask"
		}
	}
	if cfg.Cursor.OnToolCall == "" {
		cfg.Cursor.OnToolCall = "abort"
	}
	if cfg.Cursor.StreamPendingMode == "" {
		cfg.Cursor.StreamPendingMode = "optimistic"
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CG_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("CG_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("CG_CURSOR_BINARY_PATH"); v != "" {
		cfg.Cursor.BinaryPath = v
	}
	if v := os.Getenv("CG_CURSOR_DEFAULT_MODEL"); v != "" {
		cfg.Cursor.DefaultModel = v
	}
	if v := os.Getenv("CG_CURSOR_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Cursor.MaxConcurrent = n
		}
	}
	if v := os.Getenv("CG_CURSOR_PROXY"); v != "" {
		cfg.Cursor.Proxy = v
	}
	if v := os.Getenv("CG_SESSION_ENABLED"); v != "" {
		cfg.Session.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("CG_SESSION_STORAGE_PATH"); v != "" {
		cfg.Session.StoragePath = v
	}
	if v := os.Getenv("CG_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("CG_AUTH_API_KEY"); v != "" {
		cfg.Auth.APIKey = v
	}
	if v := os.Getenv("CG_LOGGING_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("CG_CURSOR_THINKING_VISIBILITY"); v != "" {
		cfg.Cursor.ThinkingVisibility = v
	}
	if v := os.Getenv("CG_CURSOR_AGENT_PROFILE"); v != "" {
		cfg.Cursor.AgentProfile = v
	}
	if v := os.Getenv("CG_CURSOR_STREAM_PENDING_MODE"); v != "" {
		cfg.Cursor.StreamPendingMode = v
	}
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
