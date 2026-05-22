package concurrency

import (
	"sync"
	"testing"
	"time"
)

func TestControllerAcquireRelease(t *testing.T) {
	cc := NewController(2, time.Second)
	if err := cc.Acquire(); err != nil {
		t.Fatal(err)
	}
	if cc.ActiveCount() != 1 {
		t.Fatalf("active=%d want 1", cc.ActiveCount())
	}
	cc.Release(time.Millisecond)
	if cc.ActiveCount() != 0 {
		t.Fatalf("active=%d want 0", cc.ActiveCount())
	}
}

func TestControllerQueueTimeout(t *testing.T) {
	cc := NewController(1, 50*time.Millisecond)
	if err := cc.Acquire(); err != nil {
		t.Fatal(err)
	}
	err := cc.Acquire()
	if err == nil {
		t.Fatal("expected queue timeout")
	}
	cc.Release(0)
}

func TestControllerConcurrent(t *testing.T) {
	cc := NewController(4, 5*time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cc.Acquire(); err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			cc.Release(time.Millisecond)
		}()
	}
	wg.Wait()
	stats := cc.StatsSnapshot()
	if stats.Completed == 0 {
		t.Fatal("expected completed > 0")
	}
}
