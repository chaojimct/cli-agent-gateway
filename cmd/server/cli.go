package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

func runInit() {
	dir, err := config.EnsureUserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config directory ready:\n  %s\n", dir)
	fmt.Printf("  %s\n", filepath.Join(dir, config.DefaultConfigFileName))
	fmt.Printf("  %s\n", filepath.Join(dir, "config.local.yaml"))
	fmt.Println("Edit config.local.yaml (cursor.binary_path, workspace), then start the gateway.")
}

func runDoctor() {
	fmt.Printf("cli-agent-gateway %s\n", version)

	if custom := os.Getenv("CG_BINARY_PATH"); custom != "" {
		fmt.Printf("binary (CG_BINARY_PATH): %s\n", custom)
	} else {
		self, err := os.Executable()
		if err == nil {
			fmt.Printf("binary: %s\n", self)
		}
	}

	dir, err := config.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "user config dir: %v\n", err)
	} else {
		fmt.Printf("user config dir: %s\n", dir)
		if _, err := config.ResolveConfigPath(config.DefaultConfigFileName); err != nil {
			fmt.Fprintf(os.Stderr, "resolve config: %v\n", err)
		}
	}

	cwd, _ := os.Getwd()
	cwdCfg := filepath.Join(cwd, config.DefaultConfigFileName)
	if _, err := os.Stat(cwdCfg); err == nil {
		fmt.Printf("cwd config: %s\n", cwdCfg)
	}

	agent := "cursor-agent"
	if path, err := exec.LookPath(agent); err == nil {
		fmt.Printf("cursor-agent: %s\n", path)
	} else {
		fmt.Printf("cursor-agent: not found in PATH (install Cursor CLI or set cursor.binary_path)\n")
	}
}
