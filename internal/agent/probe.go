package agent

import (
	"strings"

	"github.com/user/cursor-gateway/internal/acp"
)

// ValidateProbe checks initialize result against profile probe rules.
func ValidateProbe(p Profile, init *acp.InitializeResult) bool {
	if init == nil {
		return false
	}

	for _, reject := range p.RejectAuthMethods {
		for _, am := range init.AuthMethods {
			if am.ID == reject {
				return false
			}
		}
	}

	if len(p.ExpectedAuthMethods) > 0 {
		matched := false
		for _, exp := range p.ExpectedAuthMethods {
			for _, am := range init.AuthMethods {
				if am.ID == exp {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	nameLower := strings.ToLower(init.AgentInfo.Name)
	if p.ID == "claude" && strings.Contains(nameLower, "cursor") {
		return false
	}

	if len(p.ExpectedAgentNames) > 0 {
		matched := false
		for _, exp := range p.ExpectedAgentNames {
			if strings.Contains(nameLower, strings.ToLower(exp)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if p.ID == "cursor" {
		hasCursorLogin := false
		for _, am := range init.AuthMethods {
			if am.ID == "cursor_login" {
				hasCursorLogin = true
				break
			}
		}
		if !hasCursorLogin && !strings.Contains(nameLower, "cursor") {
			return false
		}
	}

	return true
}
