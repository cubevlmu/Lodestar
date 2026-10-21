package api

import (
	"sync"
	"testing"
	"time"
)

func TestNotifierWakesWaiter(t *testing.T) {
	n := newJobNotifier()
	ch := n.subscribe(7)

	go func() {
		time.Sleep(10 * time.Millisecond)
		n.notify(7)
	}()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("waiter was not woken by notify")
	}
}

func TestNotifierIgnoresOtherTargets(t *testing.T) {
	n := newJobNotifier()
	mine := n.subscribe(1)
	theirs := n.subscribe(2)

	n.notify(2)

	select {
	case <-theirs:
	case <-time.After(time.Second):
		t.Fatal("notify(2) should wake the waiter for target 2")
	}
	select {
	case <-mine:
		t.Fatal("notify(2) must not wake the waiter for target 1")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNotifierAfterUnsubscribe(t *testing.T) {
	n := newJobNotifier()
	ch := n.subscribe(3)
	n.unsubscribe(3, ch)

	// Notifying a target nobody waits on must be a no-op, not a panic.
	n.notify(3)

	select {
	case <-ch:
		t.Fatal("an unsubscribed channel must not be closed")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNotifierWakesEveryWaiter(t *testing.T) {
	n := newJobNotifier()
	const waiters = 8
	chans := make([]chan struct{}, 0, waiters)
	for i := 0; i < waiters; i++ {
		chans = append(chans, n.subscribe(9))
	}

	n.notify(9)

	for i, ch := range chans {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("waiter %d was not woken", i)
		}
	}
}

// The poll loop re-subscribes after every wake-up, so a second job must wake it
// again. This mirrors what AgentNextJob does.
func TestNotifierResubscribe(t *testing.T) {
	n := newJobNotifier()
	ch := n.subscribe(5)
	n.notify(5)
	<-ch

	ch = n.subscribe(5)
	defer n.unsubscribe(5, ch)
	n.notify(5)

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("re-subscribed waiter was not woken")
	}
}

func TestNotifierConcurrentUse(t *testing.T) {
	n := newJobNotifier()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(target uint) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				ch := n.subscribe(target)
				n.notify(target)
				select {
				case <-ch:
				case <-time.After(time.Second):
					t.Errorf("target %d was not woken", target)
					return
				}
				n.unsubscribe(target, ch)
			}
		}(uint(i%3 + 1))
	}
	wg.Wait()
}
