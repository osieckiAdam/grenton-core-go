package clu_test

// The session's own tests, for what the conformance suite does not cover
// (spec §9.5 item 8): resource release, abandoned sessions and the real UDP
// transports.

import (
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/internal/clutest"
	"github.com/osieckiAdam/grenton-core-go/model"
)

const (
	testKey = "AAECAwQFBgcICQoLDA0ODw=="
	testIV  = "EBESExQVFhcYGRobHB0eHw=="
)

func testCipher(t *testing.T) *clu.AESCipher {
	t.Helper()
	c, err := clu.NewCipher(testKey, testIV)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var testCLU = model.Clu{ID: "CLU_1", Name: "CLU_1", IP: "127.0.0.1", Port: 1234}

// harness drives a session on the virtual clock and scripted transport.
type harness struct {
	t         *testing.T
	act       *clutest.Activity
	clock     *clutest.Clock
	transport *clutest.Transport
	cipher    *clu.AESCipher
	session   *clu.Session
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, act: clutest.NewActivity(), transport: &clutest.Transport{}, cipher: testCipher(t)}
	h.clock = clutest.NewClock(h.act)
	h.session = clu.NewSession(clu.Config{CLU: testCLU, Cipher: h.cipher, Transport: h.transport, Clock: h.clock,
		Activity: h.act, PingInterval: time.Hour, RegisterInterval: time.Hour})
	return h
}

func (h *harness) idle() {
	h.t.Helper()
	if err := h.act.Idle(); err != nil {
		h.t.Fatal(err)
	}
}

// async runs f as runner work and returns its completion.
func (h *harness) async(f func()) <-chan struct{} {
	done := make(chan struct{})
	h.act.Add(1)
	go func() {
		defer h.act.Add(-1)
		f()
		close(done)
	}()
	h.idle()
	return done
}

// sent returns the next sent frame as text, and its socket.
func (h *harness) sent() (string, *clutest.Socket) {
	h.t.Helper()
	h.idle()
	s, ok := h.transport.TakeSent()
	if !ok {
		h.t.Fatal("nothing was sent")
	}
	plain, _ := h.cipher.Decrypt(s.Data)
	return string(plain), s.Socket
}

// reply answers a request frame on its socket.
func (h *harness) reply(frame string, s *clutest.Socket, payload string) {
	h.t.Helper()
	id, _, _ := strings.Cut(strings.TrimPrefix(frame, "req::"), ":")
	data, _ := h.cipher.Encrypt([]byte("resp:127.0.0.1:" + id + ":" + payload))
	s.Deliver(data)
	h.idle()
}

func (h *harness) wait(done <-chan struct{}) {
	h.t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		h.t.Fatal("the call did not finish")
	}
}

func (h *harness) start() *clutest.Socket {
	done := h.async(h.session.Start)
	frame, s := h.sent()
	h.reply(frame, s, "1")
	h.wait(done)
	return s
}

// waitGoroutines polls until the goroutine count is back to at most n.
func waitGoroutines(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > n {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			t.Fatalf("%d goroutines remain, want %d:\n%s", runtime.NumGoroutine(), n, buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// After Stop the session holds no goroutine (spec §5.4.5).
func TestStopLeavesNoGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	h := newHarness(t)
	s := h.start()
	done := h.async(func() { h.session.Register([]model.ValueRef{model.Variable("CLU_1", "x")}) })
	frame, _ := h.sent()
	h.reply(frame, s, "clientReport:1:{1}")
	h.wait(done)
	h.session.Stop()
	if !s.IsClosed() {
		t.Fatal("the socket is open after Stop")
	}
	waitGoroutines(t, before)
}

// The same over the real UDP transport, with the real clock and real
// loopback sockets: the read goroutine ends too.
func TestStopOverUDPLeavesNoGoroutine(t *testing.T) {
	cluConn := listenLoopback(t)
	defer cluConn.Close()
	before := runtime.NumGoroutine()
	cfg := clu.Config{CLU: cluAt(cluConn), Cipher: testCipher(t), ResponseTimeout: 200 * time.Millisecond}
	s := clu.NewSession(cfg)
	s.Start() // nobody answers: degraded, the ping loop runs
	s.Stop()
	waitGoroutines(t, before)
}

// A session dropped without Stop closes its socket (spec §5.4.5).
func TestAbandonedSessionClosesItsSocket(t *testing.T) {
	h := newHarness(t)
	s := h.start()
	h.session = nil
	deadline := time.Now().Add(2 * time.Second)
	for !s.IsClosed() {
		if time.Now().After(deadline) {
			t.Fatal("the socket of an abandoned session stays open")
		}
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
}

func listenLoopback(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback UDP: %v", err)
	}
	return c
}

func cluAt(c *net.UDPConn) model.Clu {
	a := c.LocalAddr().(*net.UDPAddr)
	return model.Clu{ID: "CLU_1", Name: "CLU_1", IP: a.IP.String(), Port: int64(a.Port)}
}

// recorder is a Receiver that collects datagrams.
type recorder struct{ got chan []byte }

func (r recorder) Datagram(d []byte) { r.got <- d }
func (r recorder) Closed(error)      {}

// Any-source mode (spec §7.1): a datagram from a foreign source port
// reaches the socket; peer-filtered mode drops it.
func TestUDPModes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		anySource bool
	}{{"filtered", false}, {"any source", true}} {
		t.Run(tc.name, func(t *testing.T) {
			cluConn := listenLoopback(t)
			defer cluConn.Close()
			foreign := listenLoopback(t)
			defer foreign.Close()
			r := recorder{got: make(chan []byte, 4)}
			sock, err := clu.UDP{AnySource: tc.anySource}.Open("127.0.0.1", int(cluAt(cluConn).Port), r)
			if err != nil {
				t.Fatal(err)
			}
			defer sock.Close()
			// The CLU learns the socket's port from a datagram it receives.
			if err := sock.Send([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 64)
			cluConn.SetReadDeadline(time.Now().Add(time.Second))
			_, from, err := cluConn.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			foreign.WriteToUDP([]byte("foreign"), from)
			cluConn.WriteToUDP([]byte("peer"), from)
			var got []string
			timeout := time.After(300 * time.Millisecond)
		collect:
			for {
				select {
				case d := <-r.got:
					got = append(got, string(d))
				case <-timeout:
					break collect
				}
			}
			want := "[peer]"
			if tc.anySource {
				want = "[foreign peer]"
			}
			if g := strings.Join(sortStrings(got), " "); "["+g+"]" != want {
				t.Fatalf("received %v, want %s", got, want)
			}
		})
	}
}

func sortStrings(s []string) []string {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return s
}
