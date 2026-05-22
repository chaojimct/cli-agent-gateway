package runner

import (
	"errors"
	"strings"
)

// ErrKind classifies cursor-agent failures.
type ErrKind string

const (
	ErrUnauthenticated ErrKind = "unauthenticated"
	ErrRateLimit       ErrKind = "rate_limit"
	ErrModelNotFound   ErrKind = "model_not_found"
	ErrNetwork         ErrKind = "network"
	ErrUnknown         ErrKind = "unknown"
)

// ClassifiedErr wraps an error with HTTP mapping hints.
type ClassifiedErr struct {
	Kind    ErrKind
	Message string
	Cause   error
}

func (e *ClassifiedErr) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func (e *ClassifiedErr) Unwrap() error { return e.Cause }

func Classify(stderr string, cause error) error {
	lower := stderr
	if cause != nil {
		lower += " " + cause.Error()
	}
	lower = strings.ToLower(lower)
	switch {
	case containsAny(lower, "not logged in", "unauthorized", "authentication", "login"):
		return &ClassifiedErr{Kind: ErrUnauthenticated, Message: "cursor-agent not authenticated", Cause: cause}
	case containsAny(lower, "rate limit", "quota", "too many requests"):
		return &ClassifiedErr{Kind: ErrRateLimit, Message: "cursor rate limit", Cause: cause}
	case containsAny(lower, "model not found", "unknown model"):
		return &ClassifiedErr{Kind: ErrModelNotFound, Message: "model not found", Cause: cause}
	case containsAny(lower, "network", "connection refused", "timeout", "dial"):
		return &ClassifiedErr{Kind: ErrNetwork, Message: "cursor network error", Cause: cause}
	default:
		if cause != nil {
			return cause
		}
		return errors.New("cursor-agent error")
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// HTTPStatus maps classified errors to status codes.
func HTTPStatus(err error) int {
	var ce *ClassifiedErr
	if errors.As(err, &ce) {
		switch ce.Kind {
		case ErrUnauthenticated:
			return 401
		case ErrRateLimit:
			return 429
		case ErrModelNotFound:
			return 404
		case ErrNetwork:
			return 502
		}
	}
	return 500
}
