package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manager holds live configuration with optional persistence.
type Manager struct {
	mu       sync.RWMutex
	cfg      *Config
	path     string
	onReload func(*Config)
}

func NewManager(cfg *Config, path string, onReload func(*Config)) *Manager {
	return &Manager{cfg: cfg, path: path, onReload: onReload}
}

func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Snapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return snapshotConfig(m.cfg)
}

func (m *Manager) Apply(patch map[string]interface{}) (requiresRestart bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := yaml.Marshal(m.cfg)
	if err != nil {
		return false, err
	}
	var merged map[string]interface{}
	if err := yaml.Unmarshal(data, &merged); err != nil {
		return false, err
	}
	deepMerge(merged, patch)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return false, err
	}
	newCfg := Default()
	if err := yaml.Unmarshal(out, newCfg); err != nil {
		return false, fmt.Errorf("invalid config: %w", err)
	}
	applyProfileDefaults(newCfg)

	requiresRestart = configRequiresRestart(m.cfg, newCfg)
	m.cfg = newCfg

	if m.path != "" {
		if err := os.WriteFile(m.path, out, 0600); err != nil {
			return requiresRestart, fmt.Errorf("save config: %w", err)
		}
	}

	if m.onReload != nil {
		m.onReload(newCfg)
	}
	return requiresRestart, nil
}

func configRequiresRestart(old, new *Config) bool {
	if old == nil || new == nil {
		return true
	}
	return old.Server.Host != new.Server.Host ||
		old.Server.Port != new.Server.Port ||
		old.Cursor.BinaryPath != new.Cursor.BinaryPath ||
		old.Cursor.UseDaemon != new.Cursor.UseDaemon ||
		old.Log.Format != new.Log.Format
}

func deepMerge(dst map[string]interface{}, patch map[string]interface{}) {
	for k, v := range patch {
		if vMap, ok := v.(map[string]interface{}); ok {
			if existing, ok := dst[k].(map[string]interface{}); ok {
				deepMerge(existing, vMap)
				continue
			}
		}
		dst[k] = v
	}
}

func snapshotConfig(cfg *Config) map[string]interface{} {
	data, _ := yaml.Marshal(cfg)
	var out map[string]interface{}
	_ = yaml.Unmarshal(data, &out)
	maskSecrets(out)
	return out
}

func maskSecrets(m map[string]interface{}) {
	if auth, ok := m["auth"].(map[string]interface{}); ok {
		if key, ok := auth["api_key"].(string); ok && key != "" {
			auth["api_key"] = "***"
		}
	}
}

// Schema returns metadata for the config UI.
func Schema() map[string]interface{} {
	return map[string]interface{}{
		"hot_reload": []string{
			"cursor.max_concurrent",
			"cursor.agent_profile",
			"cursor.stream_pending_mode",
			"cursor.thinking_visibility",
			"auth.enabled",
			"session.enabled",
			"agents.auto_discover",
			"agents.default",
			"agents.model_cache_ttl",
			"agents.fallback_to_default",
		},
		"requires_restart": []string{
			"server.host",
			"server.port",
			"cursor.binary_path",
			"cursor.use_daemon",
			"logging.format",
			"agents.profiles",
		},
	}
}
