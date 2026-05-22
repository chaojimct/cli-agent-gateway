package acp

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveSpawn picks the process executable and args for cursor-agent acp.
// On Windows, bypass .cmd/.ps1 wrappers and spawn node.exe + index.js directly
// so JSON-RPC stdio stays stable for long-lived sessions.
func ResolveSpawn(binPath string, args []string, noAppendACP bool) (exe string, fullArgs []string) {
	if len(args) == 0 && !noAppendACP {
		args = []string{"acp"}
	} else if len(args) > 0 && !noAppendACP && !containsACP(args) {
		args = append(append([]string{}, args...), "acp")
	}

	if node, idx, ok := resolveWindowsNode(binPath); ok {
		return node, append([]string{idx}, args...)
	}

	exe, prefix := resolveCommand(binPath)
	return exe, append(append([]string{}, prefix...), args...)
}

func resolveWindowsNode(binPath string) (nodeExe, indexJS string, ok bool) {
	if filepath.Separator != '\\' {
		return "", "", false
	}
	if binPath == "" {
		binPath = "cursor-agent"
	}
	if !shouldUseCursorNodeLayout(binPath) {
		return "", "", false
	}
	base := filepath.Dir(binPath)
	if base == "." || strings.EqualFold(binPath, "cursor-agent") {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidate := filepath.Join(local, "cursor-agent", "cursor-agent.cmd")
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				base = filepath.Dir(candidate)
			}
		}
	}
	ver := findLatestVersionDir(base)
	if ver == "" {
		return "", "", false
	}
	nodeExe = filepath.Join(ver, "node.exe")
	indexJS = filepath.Join(ver, "index.js")
	if st, err := os.Stat(nodeExe); err != nil || st.IsDir() {
		return "", "", false
	}
	if st, err := os.Stat(indexJS); err != nil || st.IsDir() {
		return "", "", false
	}
	return nodeExe, indexJS, true
}

func shouldUseCursorNodeLayout(binPath string) bool {
	lower := strings.ToLower(strings.TrimSpace(binPath))
	if lower == "" || lower == "cursor-agent" {
		return true
	}
	if strings.Contains(lower, "cursor-agent") {
		return true
	}
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".ps1") {
		return strings.Contains(lower, "cursor-agent")
	}
	return false
}

func findLatestVersionDir(basePath string) string {
	versionsDir := filepath.Join(basePath, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return ""
	}
	var latestName string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 10 || name[4] != '.' || name[7] != '.' || name[10] != '-' {
			continue
		}
		if name > latestName {
			latestName = name
		}
	}
	if latestName == "" {
		return ""
	}
	return filepath.Join(versionsDir, latestName)
}
