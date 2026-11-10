package clu

import (
	"sync"
	"time"
)

// sender sends a socket's datagrams in order from one goroutine, so that a
// send that never completes blocks neither the session nor Stop, and a
// clientDestroy can never overtake the registration it ends.
type sender struct {
	t       tracker
	mu      sync.Mutex
	queue   []sendItem
	idle    *waiter // set while run is parked on an empty queue
	busy    bool    // a Send is in progress
	closed  bool
	drained []*waiter
}

type sendItem struct {
	data   []byte
	before func() bool // false: drop the item without sending
	after  func(error)
}

func (q *sender) push(it sendItem) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		if it.after != nil {
			it.after(ErrConnectionClosed)
		}
		return
	}
	q.queue = append(q.queue, it)
	w := q.idle
	q.idle = nil
	q.mu.Unlock()
	if w != nil {
		w.fire(nil)
	}
}

// run sends until the sender is closed.
func (q *sender) run(s Socket) {
	for {
		q.mu.Lock()
		for len(q.queue) == 0 && !q.closed {
			q.busy = false
			q.notifyDrainedLocked()
			w := newWaiter(q.t)
			q.idle = w
			q.mu.Unlock()
			w.wait()
			q.mu.Lock()
		}
		if q.closed {
			dropped := q.queue
			q.queue, q.busy = nil, false
			q.notifyDrainedLocked()
			q.mu.Unlock()
			for _, it := range dropped {
				if it.after != nil {
					it.after(ErrConnectionClosed)
				}
			}
			return
		}
		it := q.queue[0]
		q.queue = q.queue[1:]
		q.busy = true
		q.mu.Unlock()
		if it.before == nil || it.before() {
			err := s.Send(it.data)
			if it.after != nil {
				it.after(err)
			}
		}
	}
}

func (q *sender) notifyDrainedLocked() {
	for _, w := range q.drained {
		w.fire(true)
	}
	q.drained = nil
}

// drain waits until every queued datagram was sent, at most timeout; it
// reports whether the queue drained.
func (q *sender) drain(clock Clock, timeout time.Duration) bool {
	q.mu.Lock()
	if q.closed || (!q.busy && len(q.queue) == 0) {
		q.mu.Unlock()
		return true
	}
	w := newWaiter(q.t)
	q.drained = append(q.drained, w)
	q.mu.Unlock()
	timer := clock.AfterFunc(timeout, func() { w.fire(false) })
	ok := w.wait() == true
	timer.Stop()
	return ok
}

// close ends run after the Send in progress; queued items are dropped.
func (q *sender) close() {
	q.mu.Lock()
	q.closed = true
	w := q.idle
	q.idle = nil
	q.mu.Unlock()
	if w != nil {
		w.fire(nil)
	}
}
