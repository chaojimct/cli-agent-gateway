package concurrency

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks real-time concurrency metrics.
type Stats struct {
	Active         int32 `json:"active"`
	Queued         int32 `json:"queued"`
	Completed      int64 `json:"completed"`
	TotalLatencyMs int64 `json:"total_latency_ms"`
	RequestCount   int64 `json:"request_count"`
	MaxConcurrent  int   `json:"max_concurrent"`
	QueueTimeout   int   `json:"queue_timeout_s"`

	mu            sync.Mutex
	latencySum    time.Duration
	latencyCount  int64
	latencyWindow []time.Duration
	windowSize    int
}

// Controller limits in-flight agent runs.
type Controller struct {
	sem     chan struct{}
	stats   *Stats
	timeout time.Duration
}

// NewController creates a controller with the given limits.
func NewController(maxConcurrent int, queueTimeout time.Duration) *Controller {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	if queueTimeout <= 0 {
		queueTimeout = 30 * time.Second
	}
	sem := make(chan struct{}, maxConcurrent)
	for i := 0; i < maxConcurrent; i++ {
		sem <- struct{}{}
	}
	return &Controller{
		sem: sem,
		stats: &Stats{
			MaxConcurrent: maxConcurrent,
			QueueTimeout:  int(queueTimeout.Seconds()),
			windowSize:    100,
		},
		timeout: queueTimeout,
	}
}

// Acquire blocks until a permit is available or the queue times out.
func (cc *Controller) Acquire() error {
	atomic.AddInt32(&cc.stats.Queued, 1)
	timer := time.NewTimer(cc.timeout)
	defer timer.Stop()

	select {
	case <-cc.sem:
		atomic.AddInt32(&cc.stats.Queued, -1)
		atomic.AddInt32(&cc.stats.Active, 1)
		return nil
	case <-timer.C:
		atomic.AddInt32(&cc.stats.Queued, -1)
		return fmt.Errorf("queue timeout: all %d worker slots busy, waited %v", cc.stats.MaxConcurrent, cc.timeout)
	}
}

// Release returns a permit after a successful Acquire.
func (cc *Controller) Release(latency time.Duration) {
	newActive := atomic.AddInt32(&cc.stats.Active, -1)
	if newActive < 0 {
		atomic.AddInt32(&cc.stats.Active, 1)
		return
	}

	select {
	case cc.sem <- struct{}{}:
	default:
		atomic.AddInt32(&cc.stats.Active, 1)
		return
	}

	atomic.AddInt64(&cc.stats.Completed, 1)

	cc.stats.mu.Lock()
	cc.stats.latencySum += latency
	cc.stats.latencyCount++
	cc.stats.TotalLatencyMs = cc.stats.latencySum.Milliseconds()
	cc.stats.RequestCount = cc.stats.latencyCount
	cc.stats.latencyWindow = append(cc.stats.latencyWindow, latency)
	if len(cc.stats.latencyWindow) > cc.stats.windowSize {
		cc.stats.latencyWindow = cc.stats.latencyWindow[1:]
	}
	cc.stats.mu.Unlock()
}

// StatsSnapshot returns a snapshot of current metrics.
func (cc *Controller) StatsSnapshot() Stats {
	active := atomic.LoadInt32(&cc.stats.Active)
	if active < 0 {
		active = 0
	}
	stats := Stats{
		Active:         active,
		Queued:         atomic.LoadInt32(&cc.stats.Queued),
		Completed:      atomic.LoadInt64(&cc.stats.Completed),
		TotalLatencyMs: atomic.LoadInt64(&cc.stats.TotalLatencyMs),
		RequestCount:   atomic.LoadInt64(&cc.stats.RequestCount),
		MaxConcurrent:  cc.stats.MaxConcurrent,
		QueueTimeout:   cc.stats.QueueTimeout,
	}

	cc.stats.mu.Lock()
	if len(cc.stats.latencyWindow) > 0 {
		var sum time.Duration
		for _, d := range cc.stats.latencyWindow {
			sum += d
		}
		stats.TotalLatencyMs = sum.Milliseconds()
		stats.RequestCount = int64(len(cc.stats.latencyWindow))
	}
	cc.stats.mu.Unlock()

	return stats
}

// ActiveCount returns the number of currently active requests.
func (cc *Controller) ActiveCount() int32 {
	n := atomic.LoadInt32(&cc.stats.Active)
	if n < 0 {
		return 0
	}
	return n
}

// Utilization returns current utilization as a percentage (0-100).
func (cc *Controller) Utilization() float64 {
	return (float64(cc.ActiveCount()) / float64(cc.stats.MaxConcurrent)) * 100
}
