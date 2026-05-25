package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/chaojimct/cli-agent-gateway/internal/agent"
	"github.com/chaojimct/cli-agent-gateway/internal/config"
)

const boxWidth = 52 // inner width between ║ chars

func printBanner(w io.Writer, version string, cfg *config.Config, registry *agent.Registry) {
	addr := cfg.Addr()
	apiURL := "http://" + addr + "/v1/chat/completions"
	webURL := "http://" + addr
	healthURL := "http://" + addr + "/healthz"

	fmt.Fprintf(w, "  ╔════════════════════════════════════════════════════╗\n")
	fmt.Fprintf(w, "  ║        CLI Agent Gateway  %-24s║\n", "v"+version)
	fmt.Fprintf(w, "  ╠════════════════════════════════════════════════════╣\n")
	fmt.Fprintf(w, "  ║  API     %s%s║\n", apiURL, pad(apiURL, boxWidth-8))
	fmt.Fprintf(w, "  ║  Web UI  %s%s║\n", webURL, pad(webURL, boxWidth-8))
	fmt.Fprintf(w, "  ║  Health  %s%s║\n", healthURL, pad(healthURL, boxWidth-8))
	fmt.Fprintf(w, "  ╠════════════════════════════════════════════════════╣\n")

	if registry != nil {
		profiles := registry.Profiles()
		defaultID := registry.DefaultID()

		fmt.Fprintf(w, "  ║  Agents% s║\n", pad("", boxWidth-8))
		if len(profiles) == 0 {
			fmt.Fprintf(w, "  ║    (none discovered)%s║\n", pad("", boxWidth-21))
		} else {
			for _, p := range profiles {
				marker := " "
				if p.ID == defaultID {
					marker = "*"
				}
				name := p.Name
				if name == "" {
					name = p.ID
				}
				line := fmt.Sprintf("    %s %-8s  %s", marker, p.ID, name)
				fmt.Fprintf(w, "  ║  %s%s║\n", line, pad(line, boxWidth-2))
			}
		}

		profile := cfg.Cursor.AgentProfile
		if profile == "" {
			profile = "model"
		}
		concurrency := cfg.Cursor.MaxConcurrent
		if concurrency <= 0 {
			concurrency = 8
		}
		authStr := "on"
		if !cfg.Auth.Enabled {
			authStr = "off"
		}
		sessionStr := "on"
		if !cfg.Session.Enabled {
			sessionStr = "off"
		}

		line1 := fmt.Sprintf("  Default: %-8s  Profile: %s", defaultID, profile)
		fmt.Fprintf(w, "  ║%s%s║\n", line1, pad(line1, boxWidth))
		line2 := fmt.Sprintf("  Concurrency: %-2d  |  Auth: %-4s  |  Session: %s", concurrency, authStr, sessionStr)
		fmt.Fprintf(w, "  ║%s%s║\n", line2, pad(line2, boxWidth))
	}

	fmt.Fprintf(w, "  ╚════════════════════════════════════════════════════╝\n")
}

func pad(s string, width int) string {
	n := width - len(s)
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
