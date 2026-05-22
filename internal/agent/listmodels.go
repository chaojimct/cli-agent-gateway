package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

// ListModelsCLI runs agent-specific model listing when supported.
func ListModelsCLI(ctx context.Context, p *Profile, cfg *config.CursorConfig) ([]string, error) {
	if p == nil || len(p.ListModelsArgs) == 0 {
		return nil, nil
	}
	exe, prefix := resolveSpawn(p.Command())
	args := append(append([]string{}, prefix...), p.ListModelsArgs...)
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = listModelsEnv(p, cfg)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseModelList(string(out)), nil
}

func listModelsEnv(p *Profile, cfg *config.CursorConfig) []string {
	env := os.Environ()
	if p == nil {
		return env
	}
	return append(env, SpawnEnv(*p, cfg)...)
}

func parseModelList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(raw), &arr) == nil && len(arr) > 0 {
		return arr
	}
	var obj struct {
		Models []string `json:"models"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &obj) == nil {
		if len(obj.Models) > 0 {
			return obj.Models
		}
		if len(obj.Data) > 0 {
			out := make([]string, 0, len(obj.Data))
			for _, m := range obj.Data {
				if m.ID != "" {
					out = append(out, m.ID)
				}
			}
			return out
		}
	}
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "Available models") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "tip:") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func resolveSpawn(binPath string) (exe string, argsPrefix []string) {
	if binPath == "" {
		return "cursor-agent", nil
	}
	if filepath.Separator != '\\' {
		return binPath, nil
	}
	lower := strings.ToLower(binPath)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		comspec := os.Getenv("COMSPEC")
		if comspec == "" {
			comspec = `C:\Windows\System32\cmd.exe`
		}
		return comspec, []string{"/c", binPath}
	}
	return binPath, nil
}
