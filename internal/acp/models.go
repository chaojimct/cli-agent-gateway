package acp

import "strings"

// ResolveModel maps gateway model names to cursor-agent ACP model IDs.
func ResolveModel(requested string) string {
	if requested == "" || requested == "auto" {
		return "default[]"
	}
	switch requested {
	case "composer-2.5-fast", "composer-2.5", "composer25", "composer25fast":
		return "composer-2.5[fast=true]"
	case "composer-2-fast", "composer-2", "composer2", "composer2fast":
		return "composer-2[fast=true]"
	default:
		if strings.Contains(requested, "[") {
			return requested
		}
		return requested
	}
}
