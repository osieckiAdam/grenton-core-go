// Package clutest is the test harness of the Runtime: a clock that moves
// only when told, a transport that records what a session sends and
// delivers what a test injects, and an activity counter that tells when a
// session is idle (Go plan §5). It holds no protocol logic.
package clutest

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
)

// NewClock returns a virtual clock at an arbitrary epoch; act counts the
// goroutines of timers that are already due when set.
func NewClock(act *Activity) *Clock {
	return &Clock{now: time.Unix(1_000_000, 0), timers: map[*vTimer]bool{}, act: act}
}

// activity counts runnable work; idle waits for zero.
type Activity struct {
	mu    sync.Mutex
	cond  *sync.Cond
	count int
}

func NewActivity() *Activity {
	a := &Activity{}
	a.cond = sync.NewCond(&a.mu)
	return a
}

func (a *Activity) Add(delta int) {
	a.mu.Lock()
	a.count += delta
	if a.count < 0 {
		panic("activity count below zero")
	}
	if a.count == 0 {
		a.cond.Broadcast()
	}
	a.mu.Unlock()
}

// idle waits until no work is runnable, at most two seconds of real time.
func (a *Activity) Idle() error {
	expired := false
	timer := time.AfterFunc(2*time.Second, func() {
		a.mu.Lock()
		expired = true
		a.cond.Broadcast()
		a.mu.Unlock()
	})
	defer timer.Stop()
	a.mu.Lock()
	defer a.mu.Unlock()
	for a.count != 0 {
		if expired {
			return fmt.Errorf("the session never became idle (%d runnable)", a.count)
		}
		a.cond.Wait()
	}
	return nil
}

// virtualClock stands still until advance; due callbacks run synchronously
// in deadline order (then in the order they were set).
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	seq    int
	timers map[*vTimer]bool
	act    *Activity
}

type vTimer struct {
	c   *Clock
	at  time.Time
	seq int
	f   func()
}

func (t *vTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if !t.c.timers[t] {
		return false
	}
	delete(t.c.timers, t)
	return true
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) AfterFunc(d time.Duration, f func()) clu.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &vTimer{c: c, at: c.now.Add(d), seq: c.seq, f: f}
	if d <= 0 {
		// Already due: like time.AfterFunc, on its own goroutine.
		c.act.Add(1)
		go func() {
			defer c.act.Add(-1)
			f()
		}()
		return t
	}
	c.timers[t] = true
	return t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var due []*vTimer
		for t := range c.timers {
			if !t.at.After(c.now) {
				due = append(due, t)
			}
		}
		if len(due) == 0 {
			c.mu.Unlock()
			return
		}
		sort.Slice(due, func(i, j int) bool {
			if !due[i].at.Equal(due[j].at) {
				return due[i].at.Before(due[j].at)
			}
			return due[i].seq < due[j].seq
		})
		t := due[0]
		delete(c.timers, t)
		c.mu.Unlock()
		t.f()
	}
}

// scriptedTransport records what the session sends and delivers what the
// scenario injects, synchronously.
type Transport struct {
	mu      sync.Mutex
	sockets []*Socket
	outbox  []Sent
}

// Sent is one datagram a session sent.
type Sent struct {
	Socket *Socket
	Data   []byte
}

type Socket struct {
	t      *Transport
	Serial int
	R      clu.Receiver
	Host   string // where the session opened it
	Port   int
	mu     sync.Mutex
	closed bool
}

func (t *Transport) Open(host string, port int, r clu.Receiver) (clu.Socket, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := &Socket{t: t, Serial: len(t.sockets) + 1, R: r, Host: host, Port: port}
	t.sockets = append(t.sockets, s)
	return s, nil
}

func (s *Socket) Send(data []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errors.New("closed")
	}
	s.t.mu.Lock()
	s.t.outbox = append(s.t.outbox, Sent{s, append([]byte(nil), data...)})
	s.t.mu.Unlock()
	return nil
}

func (s *Socket) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *Socket) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Socket) Deliver(data []byte) {
	if !s.IsClosed() {
		s.R.Datagram(data)
	}
}

func (t *Transport) TakeSent() (Sent, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.outbox) == 0 {
		return Sent{}, false
	}
	first := t.outbox[0]
	t.outbox = t.outbox[1:]
	return first, true
}

// Sockets lists every socket opened so far.
func (t *Transport) Sockets() []*Socket {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*Socket(nil), t.sockets...)
}
