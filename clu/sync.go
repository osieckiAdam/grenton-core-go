package clu

import "sync"

// The primitives behind the session's concurrency (Go plan §5). Every place
// a session goroutine blocks is a waiter, and every waiter reports to the
// Activity hook: −1 when its goroutine parks, +1 (by whoever fires it)
// before the goroutine wakes. With a counting hook, a count of zero means
// that no session work can happen without time passing or a datagram
// arriving, which is the idle barrier of conformance.md §4b.

// Activity observes the session's runnable work: Add(+1) when a goroutine
// starts or is woken, Add(−1) when it parks or ends. Production code leaves
// it nil; a test harness counts to know when the session is idle.
type Activity interface {
	Add(delta int)
}

type tracker struct{ a Activity }

func (t tracker) add(delta int) {
	if t.a != nil {
		t.a.Add(delta)
	}
}

// goTracked starts f on a goroutine that counts as activity while it runs.
func (t tracker) goTracked(f func()) {
	t.add(1)
	go func() {
		defer t.add(-1)
		f()
	}()
}

// waiter is a one-shot rendezvous for exactly one goroutine: the first fire
// stores the value and wakes it; later fires are no-ops. A waiter that is
// fired must be waited on, or its +1 would never be given back.
type waiter struct {
	t     tracker
	mu    sync.Mutex
	fired bool
	val   any
	ch    chan struct{}
}

func newWaiter(t tracker) *waiter {
	return &waiter{t: t, ch: make(chan struct{})}
}

// fire wakes the waiter with v; it reports whether this call was the first.
func (w *waiter) fire(v any) bool {
	w.mu.Lock()
	if w.fired {
		w.mu.Unlock()
		return false
	}
	w.fired, w.val = true, v
	w.mu.Unlock()
	w.t.add(1)
	close(w.ch)
	return true
}

// wait parks until fired and returns the value.
func (w *waiter) wait() any {
	w.t.add(-1)
	<-w.ch
	return w.val
}

// signal is an event any number of goroutines can wait for.
type signal struct {
	t       tracker
	mu      sync.Mutex
	done    bool
	waiters []*waiter
}

func newSignal(t tracker) *signal { return &signal{t: t} }

func (s *signal) wait() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	w := newWaiter(s.t)
	s.waiters = append(s.waiters, w)
	s.mu.Unlock()
	w.wait()
}

func (s *signal) fire() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	ws := s.waiters
	s.waiters = nil
	s.mu.Unlock()
	for _, w := range ws {
		w.fire(nil)
	}
}

// gate serializes whole operations, first come first served (spec §5.4.5).
type gate struct {
	t     tracker
	mu    sync.Mutex
	busy  bool
	queue []*waiter
}

func (g *gate) acquire() {
	g.mu.Lock()
	if !g.busy {
		g.busy = true
		g.mu.Unlock()
		return
	}
	w := newWaiter(g.t)
	g.queue = append(g.queue, w)
	g.mu.Unlock()
	w.wait()
}

// release hands the gate to the next in line, or frees it.
func (g *gate) release() {
	g.mu.Lock()
	if len(g.queue) == 0 {
		g.busy = false
		g.mu.Unlock()
		return
	}
	next := g.queue[0]
	g.queue = g.queue[1:]
	g.mu.Unlock()
	next.fire(nil)
}
