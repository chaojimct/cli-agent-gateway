package agent

import "github.com/chaojimct/cli-agent-gateway/internal/config"

// ProxyEnv returns proxy-related environment variables for child processes.
func ProxyEnv(cfg *config.CursorConfig) []string {
	if cfg == nil || cfg.Proxy == "" {
		return nil
	}
	p := cfg.Proxy
	return []string{
		"HTTP_PROXY=" + p,
		"HTTPS_PROXY=" + p,
		"ALL_PROXY=" + p,
		"http_proxy=" + p,
		"https_proxy=" + p,
		"npm_config_proxy=" + p,
		"npm_config_https_proxy=" + p,
		"NPX_YES=1",
	}
}

// SpawnEnv builds extra env vars for an ACP child process.
func SpawnEnv(profile Profile, cfg *config.CursorConfig) []string {
	var env []string
	if profile.ID == "cursor" {
		env = append(env, cursorEnv()...)
	}
	for k, v := range profile.SpawnEnv {
		if k != "" && v != "" {
			env = append(env, k+"="+v)
		}
	}
	env = append(env, ProxyEnv(cfg)...)
	return env
}
