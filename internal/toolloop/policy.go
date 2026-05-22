package toolloop

import "github.com/user/cursor-gateway/internal/acp"

// Profile determines permission and streaming behavior.
type Profile string

const (
	ProfileAgentic     Profile = "agentic"
	ProfileClientTools Profile = "client-tools"
	ProfileAsk         Profile = "ask"
	ProfilePlan        Profile = "plan"
)

// PermissionDecision for session/request_permission.
type PermissionDecision struct {
	Response acp.PermissionResponse
	EmitToClient bool // translate to OpenAI tool_calls for client-tools
}

// DecidePermission maps gateway profile to ACP permission outcome.
func DecidePermission(profile Profile) PermissionDecision {
	switch profile {
	case ProfileAgentic:
		return PermissionDecision{
			Response: acp.PermissionResponse{
				Outcome: acp.PermissionOutcome{Outcome: "selected", OptionID: "allow-once"},
			},
		}
	case ProfileClientTools:
		return PermissionDecision{
			Response: acp.PermissionResponse{
				Outcome: acp.PermissionOutcome{Outcome: "selected", OptionID: "reject-once"},
			},
			EmitToClient: true,
		}
	default:
		return PermissionDecision{
			Response: acp.PermissionResponse{
				Outcome: acp.PermissionOutcome{Outcome: "selected", OptionID: "reject-once"},
			},
		}
	}
}

// ProfileFromConfig derives profile from agent settings.
func ProfileFromConfig(agentProfile string, clientTools bool) Profile {
	if clientTools {
		return ProfileClientTools
	}
	if agentProfile == "agent" {
		return ProfileAgentic
	}
	if agentProfile == "plan" {
		return ProfilePlan
	}
	return ProfileAsk
}
