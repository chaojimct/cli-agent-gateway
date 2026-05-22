package toolloop

import "github.com/user/cursor-gateway/internal/acp"

func StopReasonFromString(s string) acp.StopReason {
	switch s {
	case "max_tokens":
		return acp.StopMaxTokens
	case "max_turn_requests":
		return acp.StopMaxTurnRequests
	case "refusal":
		return acp.StopRefusal
	case "cancelled":
		return acp.StopCancelled
	default:
		return acp.StopEndTurn
	}
}
