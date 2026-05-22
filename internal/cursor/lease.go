package cursor

import (
	"sync"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/concurrency"
)

// slotLease releases a concurrency slot exactly once.
type slotLease struct {
	cc   *concurrency.Controller
	start sync.Once
}

func newSlotLease(cc *concurrency.Controller) *slotLease {
	return &slotLease{cc: cc}
}

func (l *slotLease) Release(latency ...time.Duration) {
	if l == nil || l.cc == nil {
		return
	}
	l.start.Do(func() {
		var d time.Duration
		if len(latency) > 0 {
			d = latency[0]
		}
		l.cc.Release(d)
	})
}
