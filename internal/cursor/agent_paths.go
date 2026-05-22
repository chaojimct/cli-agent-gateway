package cursor

import (
	"os"
	"path/filepath"
	"strings"
)

func isWindows() bool {
	return os.PathSeparator == '\\'
}

func findLatestVersion(basePath string) string {
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

// ResolveBinary picks cursor-agent executable, including Windows .cmd wrapper.
func ResolveBinary(binPath string) (exe string, argsPrefix []string) {
	if binPath == "" {
		binPath = "cursor-agent"
	}
	if !isWindows() {
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
