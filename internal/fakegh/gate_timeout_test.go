package fakegh

import (
	"context"
	"sync"
	"testing"
	"time"
)

// A gate that never fills gives up after its timeout, however the timer and
// the waiting request happen to interleave. The timer used to fire a wakeup
// that the waiter could miss — it fired a moment before the deadline the
// waiter compared against, so the waiter went back to sleep with nothing left
// to wake it — and a lone request hung until the test binary was killed.
func TestGateGivesUpOnALoneRequestEveryTime(t *testing.T) {
	t.Parallel()
	for i := range 2000 {
		g := &gate{n: 2, timeout: time.Microsecond, cond: sync.NewCond(&sync.Mutex{})}
		done := make(chan bool, 1)
		go func() { done <- g.enter(context.Background()) }()
		select {
		case opened := <-done:
			if opened {
				t.Fatalf("attempt %d: a lone request opened a gate of 2", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("attempt %d: a lone request was still waiting long after the gate timed out", i)
		}
	}
}

// A request whose client has gone stops waiting at once, however the
// cancellation and the waiting request happen to interleave.
func TestGateStopsWaitingWhenTheClientGoes(t *testing.T) {
	t.Parallel()
	for i := range 2000 {
		g := &gate{n: 2, timeout: time.Hour, cond: sync.NewCond(&sync.Mutex{})}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan bool, 1)
		go func() { done <- g.enter(ctx) }()
		cancel()
		select {
		case opened := <-done:
			if opened {
				t.Fatalf("attempt %d: a cancelled request opened a gate of 2", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("attempt %d: a cancelled request was still waiting", i)
		}
	}
}
