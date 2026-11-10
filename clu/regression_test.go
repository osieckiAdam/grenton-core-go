package clu_test

// Regressions from an independent review of the session: cases no
// conformance scenario builds.

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/internal/clutest"
	"github.com/osieckiAdam/grenton-core-go/model"
)

// failTransport's sockets fail every send at once (e.g. EADDRNOTAVAIL
// after the host's address changed).
type failTransport struct {
	mu    sync.Mutex
	opens int
}

type failSock struct{}

func (f *failTransport) Open(string, int, clu.Receiver) (clu.Socket, error) {
	f.mu.Lock()
	f.opens++
	f.mu.Unlock()
	return failSock{}, nil
}
func (failSock) Send([]byte) error { return errors.New("sendto: can't assign requested address") }
func (failSock) Close() error      { return nil }

// A socket that cannot send has failed (§5.4.1 item 4): it is replaced,
// and requests end with an error of the taxonomy.
func TestSendFailureReplacesTheSocket(t *testing.T) {
	act := clutest.NewActivity()
	clock := clutest.NewClock(act)
	tr := &failTransport{}
	s := clu.NewSession(clu.Config{CLU: testCLU, Cipher: testCipher(t), Transport: tr, Clock: clock, Activity: act})
	defer s.Stop()
	act.Add(1)
	go func() { defer act.Add(-1); s.Start() }()
	act.Idle()
	for range 3 {
		clock.Advance(clu.DefaultPingInterval)
		act.Idle()
	}
	tr.mu.Lock()
	opens := tr.opens
	tr.mu.Unlock()
	if opens < 2 {
		t.Fatalf("the socket was never replaced (%d opens)", opens)
	}
	if s.State() != clu.StateDegraded {
		t.Fatalf("state %v, want degraded", s.State())
	}
	err := s.Execute(model.ActionRef{CallType: model.ActionScript, ObjectName: "x"})
	if !errors.Is(err, clu.ErrNotConnected) && !errors.Is(err, clu.ErrConnectionClosed) {
		t.Fatalf("Execute: %v, want notConnected or connectionClosed", err)
	}
}

// Callers that ping while a ping is on the wire share its result.
func TestConcurrentPingsShareTheResult(t *testing.T) {
	h := newHarness(t)
	defer h.session.Stop()
	s := h.start()
	var a, b bool
	first := h.async(func() { a = h.session.Ping() })
	frame, _ := h.sent()
	second := h.async(func() { b = h.session.Ping() })
	h.idle()
	if _, extra := h.transport.TakeSent(); extra {
		t.Fatal("the second ping went on the wire")
	}
	h.reply(frame, s, "1")
	h.wait(first)
	h.wait(second)
	if !a || !b {
		t.Fatalf("first=%v second=%v, want both true", a, b)
	}
}

// The rollover at 999 999 replaces the socket as part of the registration;
// a registration answered on the new socket clears the recovery flag.
func TestRolloverClearsRecovery(t *testing.T) {
	h := newHarness(t)
	defer h.session.Stop()
	h.start()
	clu.SetNextNumber(h.session, 999_999)
	done := h.async(func() { h.session.Register([]model.ValueRef{model.Variable("CLU_1", "x")}) })
	frame, s := h.sent()
	h.reply(frame, s, "clientReport:999999:{1}")
	h.wait(done)
	done = h.async(func() { h.session.Register([]model.ValueRef{model.Variable("CLU_1", "y")}) })
	var replaced *clutest.Socket
	for {
		h.idle()
		st, more := h.transport.TakeSent()
		if !more {
			break
		}
		plain, _ := h.cipher.Decrypt(st.Data)
		if st.Socket != s {
			replaced = st.Socket
			h.reply(string(plain), st.Socket, "clientReport:1:{2}")
		}
	}
	h.wait(done)
	if replaced == nil || !s.IsClosed() {
		t.Fatal("the rollover did not move to a new socket")
	}
	if d := h.session.Diagnostics(); d.RecoveryPending {
		t.Fatalf("recovery pending after a clean registration: %+v", d)
	}
}

// joinSock's Close waits for a delivery in progress, as many transports do.
type joinTransport struct{ s *joinSock }

type joinSock struct {
	mu sync.Mutex // held during a delivery and by Close
	r  clu.Receiver
}

func (j *joinTransport) Open(_ string, _ int, r clu.Receiver) (clu.Socket, error) {
	j.s = &joinSock{r: r}
	return j.s, nil
}
func (s *joinSock) Send([]byte) error { return nil }
func (s *joinSock) Close() error      { s.mu.Lock(); s.mu.Unlock(); return nil }

// Stop during a delivery does not deadlock: Close runs outside the
// session's mutex (spec §5.6 item 4).
func TestStopDuringDeliveryDoesNotDeadlock(t *testing.T) {
	cipher := testCipher(t)
	tr := &joinTransport{}
	s := clu.NewSession(clu.Config{CLU: testCLU, Cipher: cipher, Transport: tr, ResponseTimeout: 50 * time.Millisecond})
	s.Start() // nobody answers: degraded, socket kept
	data, _ := cipher.Encrypt([]byte("resp:127.0.0.1:00000000:clientReport:7:{1}"))
	tr.s.mu.Lock() // the reader is delivering
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	time.Sleep(100 * time.Millisecond)
	go func() { tr.s.r.Datagram(data); tr.s.mu.Unlock() }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked with a delivery")
	}
}
