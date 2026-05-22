package cursor

import "github.com/user/cursor-gateway/internal/concurrency"

// ConcurrencyStats is kept for backward-compatible JSON/API types.
type ConcurrencyStats = concurrency.Stats

// ConcurrencyController limits in-flight agent runs.
type ConcurrencyController = concurrency.Controller

// NewConcurrencyController creates a concurrency controller.
var NewConcurrencyController = concurrency.NewController
