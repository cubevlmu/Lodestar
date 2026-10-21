package api

import "sync"

// jobNotifier wakes long-polling agents the moment a job is queued.
//
// Without it every waiting agent has to poll the database on a timer (it used
// to be once per second), which turns an idle fleet into a steady query load
// and shows up as constant traffic in the access log. The notifier is a plain
// in-process broadcast: cheap, no goroutines of its own, and safe for several
// agents waiting on the same host.
//
// Jobs created by another process (a second replica, or an older build) are
// still picked up by the slow re-check in AgentNextJob.
type jobNotifier struct {
	mu      sync.Mutex
	waiters map[uint]map[chan struct{}]struct{}
}

func newJobNotifier() *jobNotifier {
	return &jobNotifier{waiters: make(map[uint]map[chan struct{}]struct{})}
}

// subscribe registers interest in jobs for targetID. The returned channel is
// closed by notify, so a receive unblocks immediately.
func (n *jobNotifier) subscribe(targetID uint) chan struct{} {
	ch := make(chan struct{})
	n.mu.Lock()
	if n.waiters[targetID] == nil {
		n.waiters[targetID] = make(map[chan struct{}]struct{})
	}
	n.waiters[targetID][ch] = struct{}{}
	n.mu.Unlock()
	return ch
}

// unsubscribe drops a waiter. It is safe to call for a channel that notify has
// already closed and forgotten.
func (n *jobNotifier) unsubscribe(targetID uint, ch chan struct{}) {
	n.mu.Lock()
	if set, ok := n.waiters[targetID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(n.waiters, targetID)
		}
	}
	n.mu.Unlock()
}

// notify wakes every agent waiting on targetID.
func (n *jobNotifier) notify(targetID uint) {
	n.mu.Lock()
	set := n.waiters[targetID]
	delete(n.waiters, targetID)
	n.mu.Unlock()
	for ch := range set {
		close(ch)
	}
}
