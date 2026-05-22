package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFileName = "config.yaml"
	localConfigFileName   = "config.local.yaml"
	userConfigDirName     = "cli-agent-gateway"
)

// UserConfigDir returns the per-user configuration directory.
// Override with CG_CONFIG_HOME (full path to the config root for this app).
func UserConfigDir() (string, error) {
	if v := os.Getenv("CG_CONFIG_HOME"); v != "" {
		return filepath.Clean(v), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, userConfigDirName), nil
}

// ResolveConfigPath picks the config file to load.
// When explicit is empty or "config.yaml", use ./config.yaml in the working directory if it exists;
// otherwise ensure the user config directory exists (with defaults) and use config.yaml there.
// Any other explicit path is returned unchanged.
func ResolveConfigPath(explicit string) (string, error) {
	if explicit != "" && explicit != DefaultConfigFileName {
		return explicit, nil
	}

	if cwd, err := os.Getwd(); err == nil {
		cwdConfig := filepath.Join(cwd, DefaultConfigFileName)
		if fileExists(cwdConfig) {
			return cwdConfig, nil
		}
	}

	dir, err := EnsureUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigFileName), nil
}

// EnsureUserConfigDir creates the user config directory and seeds config.yaml and
// config.local.yaml when missing.
func EnsureUserConfigDir() (string, error) {
	dir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	mainPath := filepath.Join(dir, DefaultConfigFileName)
	if !fileExists(mainPath) {
		if err := writeDefaultUserConfig(mainPath, dir); err != nil {
			return "", err
		}
	}

	localPath := filepath.Join(dir, localConfigFileName)
	if !fileExists(localPath) {
		if err := os.WriteFile(localPath, []byte(defaultLocalConfigTemplate), 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", localPath, err)
		}
	}

	return dir, nil
}

func writeDefaultUserConfig(path, configDir string) error {
	cfg := Default()
	cfg.Session.StoragePath = filepath.Join(configDir, "sessions.json")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const defaultLocalConfigTemplate = `# Local overrides for cli-agent-gateway (user config dir).
# Copy keys from config.yaml; this file is merged after config.yaml.

cursor:
  binary_path: cursor-agent
  default_model: cursor/composer-2.5-fast
  client_tools_agent_mode: plan
  # proxy: http://127.0.0.1:7890
  # workspace: /path/to/project

# logging:
#   level: debug
#   format: text

# server:
#   port: 8080

# auth:
#   enabled: true
#   api_key: "your-secret"
`
