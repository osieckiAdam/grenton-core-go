package clu

import (
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/osieckiAdam/grenton-core-go/internal/wire"
	"github.com/osieckiAdam/grenton-core-go/model"
)

// State is the lifecycle state of a [Session] (spec §5.4.5).
type State int

// The session states.
const (
	StateIdle State = iota
	StateStarting
	StateReady
	StateDegraded
	StateStopped
)

func (s State) String() string {
	return [...]string{"idle", "starting", "ready", "degraded", "stopped"}[s]
}

// Errors of a session's requests (spec §8).
var (
	// ErrTimeout: no response within the response timeout.
	ErrTimeout = errors.New("clu: no response within the timeout")
	// ErrNotConnected: the session has no open socket (idle, stopped, or
	// degraded without a socket); nothing was sent.
	ErrNotConnected = errors.New("clu: not connected")
	// ErrConnectionClosed: the socket closed or failed, or the session
	// stopped, while the request waited.
	ErrConnectionClosed = errors.New("clu: connection closed")
	// ErrUnresolvedPlaceholder: the action's value is $value$ and no intent
	// overrode it (spec §3.3); nothing was sent.
	ErrUnresolvedPlaceholder = wire.ErrUnresolvedPlaceholder

	errDuplicateID = errors.New("clu: a request with this msg_id is in flight")
)

// The defaults of spec §5.4.
const (
	DefaultResponseTimeout  = 5 * time.Second
	DefaultPingInterval     = 5 * time.Second
	DefaultRegisterInterval = 30 * time.Second
	// DefaultFrameLimit is the register frame limit in UTF-8 bytes; a
	// session may use a lower one, never a higher one.
	DefaultFrameLimit = wire.DefaultFrameLimit
)

// maxNumber is the largest subscription number (six digits, §5.4.2).
const maxNumber = 999_999

// pingTimeoutsBeforeReplacing retires a socket (spec §5.4.5).
const pingTimeoutsBeforeReplacing = 3

// Config configures a [Session]. CLU and Cipher are required; every zero
// field takes its default.
type Config struct {
	// CLU is the controller: its IP and Port are dialed, Name and ID appear
	// in logs.
	CLU model.Clu
	// Cipher encrypts every datagram (spec §5.1).
	Cipher Cipher
	// Transport opens the UDP socket; default UDP{} (peer-filtered).
	Transport Transport
	// Clock times every timeout and loop; default SystemClock{}.
	Clock Clock
	// Random draws msg_ids; default crypto/rand.
	Random io.Reader
	// Logger receives the session's logs; default discards them.
	Logger *slog.Logger
	// ResponseTimeout, PingInterval and RegisterInterval default to 5 s,
	// 5 s and 30 s (spec §5.4.1, §5.4.4).
	ResponseTimeout  time.Duration
	PingInterval     time.Duration
	RegisterInterval time.Duration
	// FrameLimit lowers the register frame limit (a smaller MTU on the
	// path); it cannot raise it above DefaultFrameLimit.
	FrameLimit int
	// Activity is a test hook that observes the session's runnable work.
	Activity Activity
}

// Session is the connection to one CLU (spec §5.4): one UDP socket for
// pings, actions, registrations and notifications, subscriptions
// registered in chunks under their own numbers, the keep-alive and
// re-registration loops, and recovery when the CLU stops responding.
//
// All methods are safe for concurrent use. Start, Register, Fetch and Stop
// are serialized as whole operations. Stop ends every socket, timer and
// goroutine of the session; a Session dropped without Stop is closed by the
// garbage collector (without clientDestroy), but call Stop.
type Session struct{ c *core }

// NewSession creates an idle session.
func NewSession(cfg Config) *Session {
	c := newCore(cfg)
	s := &Session{c: c}
	runtime.AddCleanup(s, func(c *core) { c.abandon() }, c)
	return s
}

// Start opens the socket, pings, and starts the ping loop. It is
// idempotent. A CLU that does not answer leaves the session degraded, and
// the ping loop keeps retrying. After Stop the session may be started
// again, with no subscriptions.
func (s *Session) Start() { s.c.start() }

// Stop ends pending requests, deregisters every subscription, closes the
// socket and ends every loop; it returns when the session holds nothing.
// It is idempotent.
func (s *Session) Stop() { s.c.stop() }

// Ping checks that the CLU answers; a degraded session that gets an answer
// re-registers its keys and becomes ready. Before Start or after Stop it
// sends nothing and returns false.
func (s *Session) Ping() bool { return s.c.ping(false) }

// Register replaces the subscriptions with keys and returns their initial
// values, Failed where a chunk got no usable reply. Before Start or after
// Stop it sends nothing and returns all Failed.
func (s *Session) Register(keys []model.ValueRef) []Slot { return s.c.register(keys) }

// Fetch reads keys once without subscribing (spec §5.4.2).
func (s *Session) Fetch(keys []model.ValueRef) []Slot { return s.c.fetch(keys) }

// Execute sends an action and waits for the CLU's reply. Its fields reach
// the CLU verbatim: a value containing quotes or separators changes the
// meaning of the payload (spec §5.3.4).
func (s *Session) Execute(a model.ActionRef) error { return s.c.execute(a) }

// State returns the current state.
func (s *Session) State() State {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.state
}

// Reports returns a new stream of the session's reports: the values of
// every accepted notification, and of the registrations the session makes
// by itself (renewal, recovery). The stream keeps the 64 newest reports
// for a slow reader. cancel ends it.
func (s *Session) Reports() (reports <-chan Report, cancel func()) { return s.c.subscribe() }

// StateChanges returns a channel that receives a value after every state
// change, coalesced while the reader is busy; read State for the state.
func (s *Session) StateChanges() (changes <-chan struct{}, cancel func()) {
	return s.c.subscribeState()
}

// Diagnostics is one consistent observation of a session (spec §5.4.5).
type Diagnostics struct {
	State     State
	SampledAt time.Time
	// SinceLastReply, SinceLastReport and LastPingRoundTrip are nil until
	// the event happened in this run.
	SinceLastReply    *time.Duration
	SinceLastReport   *time.Duration
	LastPingRoundTrip *time.Duration
	// RequestedKeys is the length of the last Register's key list.
	RequestedKeys int
	// RecoveryPending: a chunk failed or the session degraded, and the
	// whole list has not been registered since.
	RecoveryPending bool
	// ConsecutivePingTimeouts counts on the current socket (or the last one).
	ConsecutivePingTimeouts int
}

// Diagnostics returns a snapshot; it sends and waits for nothing.
func (s *Session) Diagnostics() Diagnostics { return s.c.diagnostics() }

// The internals. The public Session is a handle: goroutines reference only
// the core, so a dropped handle can be collected and its cleanup run.

type chunk struct {
	number int64
	keys   []model.ValueRef
}

type reply struct {
	resp wire.Response
	err  error
}

type pendingRequest struct {
	token uint64
	w     *waiter
}

type mainSocket struct {
	s   Socket
	gen int
	out *sender
}

type core struct {
	clu              model.Clu
	cipher           Cipher
	transport        Transport
	clock            Clock
	random           io.Reader
	log              *slog.Logger
	timeout          time.Duration
	pingInterval     time.Duration
	registerInterval time.Duration
	frameLimit       int
	t                tracker
	keepAlive        string
	gate             gate

	mu       sync.Mutex
	state    State
	sock     *mainSocket
	gen      int
	opening  *signal
	teardown *signal
	run      *sync.WaitGroup // the goroutines of the current run

	chunks        []chunk
	subs          map[int64][]model.ValueRef // every number the CLU may push for
	next          int64
	desired       []model.ValueRef
	needsRecovery bool
	deadline      time.Time // the next renewal, set when a registration's first chunk goes out
	hasDeadline   bool
	regTimer      Timer
	regEpoch      int
	pingEpoch     int
	pingSleep     *waiter

	pending map[string]*pendingRequest
	token   uint64

	reportSubs map[int]chan Report
	stateSubs  map[int]chan struct{}
	subID      int

	lastReply, lastReport, pingSentAt time.Time
	hasReply, hasReport, hasPingSent  bool
	lastRTT                           time.Duration
	hasRTT                            bool
	pingTimeouts                      int
}

func newCore(cfg Config) *core {
	c := &core{
		clu: cfg.CLU, cipher: cfg.Cipher, transport: cfg.Transport, clock: cfg.Clock,
		random: randomSource(cfg.Random), log: cfg.Logger,
		timeout: cfg.ResponseTimeout, pingInterval: cfg.PingInterval, registerInterval: cfg.RegisterInterval,
		frameLimit: cfg.FrameLimit, t: tracker{cfg.Activity},
		subs: map[int64][]model.ValueRef{}, next: 1, pending: map[string]*pendingRequest{},
		reportSubs: map[int]chan Report{}, stateSubs: map[int]chan struct{}{}, run: &sync.WaitGroup{},
	}
	if c.transport == nil {
		c.transport = UDP{}
	}
	if c.clock == nil {
		c.clock = SystemClock{}
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	c.log = c.log.With("clu", c.clu.Name)
	if c.timeout <= 0 {
		c.timeout = DefaultResponseTimeout
	}
	if c.pingInterval <= 0 {
		c.pingInterval = DefaultPingInterval
	}
	if c.registerInterval <= 0 {
		c.registerInterval = DefaultRegisterInterval
	}
	if c.frameLimit <= 0 || c.frameLimit > DefaultFrameLimit {
		c.frameLimit = DefaultFrameLimit
	}
	c.gate.t = c.t
	c.keepAlive = c.newID()
	return c
}

// newID draws a msg_id: 4 random bytes as 8 hex digits, never the
// notification id and never the keep-alive id.
func (c *core) newID() string {
	for {
		var b [4]byte
		if _, err := io.ReadFull(c.random, b[:]); err != nil {
			panic("clu: random source failed: " + err.Error())
		}
		id := hex.EncodeToString(b[:])
		if id != wire.NotificationID && id != c.keepAlive {
			return id
		}
	}
}

// spawn runs f as a goroutine of the current run; nothing starts once the
// session stopped.
func (c *core) spawnLocked(f func()) {
	if c.state == StateStopped {
		return
	}
	wg := c.run
	wg.Add(1)
	c.t.goTracked(func() {
		defer wg.Done()
		f()
	})
}

func (c *core) setStateLocked(s State) {
	if c.state == s {
		return
	}
	c.state = s
	for _, ch := range c.stateSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Lifecycle (spec §5.4.5).

func (c *core) start() {
	for {
		c.mu.Lock()
		td := c.teardown
		c.mu.Unlock()
		if td != nil {
			td.wait() // a stop still tearing down owns the socket
		}
		c.gate.acquire()
		c.mu.Lock()
		if c.teardown == nil {
			break
		}
		c.mu.Unlock()
		c.gate.release()
	}
	defer c.gate.release()
	if c.state != StateIdle && c.state != StateStopped {
		c.mu.Unlock()
		return
	}
	c.setStateLocked(StateStarting)
	c.chunks, c.subs, c.next = nil, map[int64][]model.ValueRef{}, 1
	c.hasReply, c.hasReport, c.hasRTT, c.hasPingSent = false, false, false, false
	c.pingTimeouts = 0
	c.run = &sync.WaitGroup{}
	c.mu.Unlock()

	c.ping(true)

	c.mu.Lock()
	if c.state != StateStopped {
		c.startPingLoopLocked()
	}
	c.mu.Unlock()
}

func (c *core) stop() {
	c.mu.Lock()
	if td := c.teardown; td != nil {
		c.mu.Unlock()
		td.wait()
		return
	}
	if c.state == StateStopped {
		c.mu.Unlock()
		return
	}
	td := newSignal(c.t)
	c.teardown = td
	run := c.run
	c.setStateLocked(StateStopped)
	c.desired, c.needsRecovery, c.pingTimeouts = nil, false, 0
	c.hasReply, c.hasReport, c.hasRTT, c.hasPingSent = false, false, false, false
	c.endLoopsLocked()
	// At once: what waits fails now, so an operation in flight drains
	// instead of waiting for its timeouts.
	c.failAllPendingLocked(ErrConnectionClosed)
	c.mu.Unlock()

	c.gate.acquire() // the operation in flight finishes first
	c.mu.Lock()
	numbers := sortedNumbers(c.subs)
	c.subs, c.chunks, c.hasDeadline = map[int64][]model.ValueRef{}, nil, false
	ms := c.sock
	c.mu.Unlock()
	if ms != nil {
		// Destroys go out only behind a drained queue, and the socket closes
		// after at most a response timeout either way (spec §5.4.1 item 3).
		if ms.out.drain(c.clock, c.timeout) {
			c.sendDestroys(ms, numbers)
			ms.out.drain(c.clock, c.timeout)
		}
		c.mu.Lock()
		if c.sock == ms {
			c.closeSocketLocked()
		}
		c.mu.Unlock()
	}
	c.gate.release()
	run.Wait() // loops and senders end: nothing of the session remains
	c.mu.Lock()
	c.teardown = nil
	c.mu.Unlock()
	td.fire()
}

// abandon closes a session the application dropped without Stop: the
// socket and the loops end, the CLU's subscriptions simply expire.
func (c *core) abandon() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped {
		return
	}
	c.setStateLocked(StateStopped)
	c.endLoopsLocked()
	c.failAllPendingLocked(ErrConnectionClosed)
	if c.sock != nil {
		c.closeSocketLocked()
	}
}

func (c *core) endLoopsLocked() {
	c.pingEpoch++
	if c.pingSleep != nil {
		c.pingSleep.fire(false)
		c.pingSleep = nil
	}
	c.regEpoch++
	if c.regTimer != nil {
		c.regTimer.Stop()
		c.regTimer = nil
	}
}

func sortedNumbers(subs map[int64][]model.ValueRef) []int64 {
	out := make([]int64, 0, len(subs))
	for n := range subs {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Socket.

type sockReceiver struct {
	c   *core
	gen int
}

func (r sockReceiver) Datagram(data []byte) { r.c.handleDatagram(r.gen, data) }
func (r sockReceiver) Closed(err error)     { r.c.socketFailed(r.gen, err) }

// ensureSocket opens the main socket unless one is open; concurrent callers
// share one attempt.
func (c *core) ensureSocket() {
	c.mu.Lock()
	if c.sock != nil || c.state == StateStopped {
		c.mu.Unlock()
		return
	}
	if sig := c.opening; sig != nil {
		c.mu.Unlock()
		sig.wait()
		return
	}
	sig := newSignal(c.t)
	c.opening = sig
	c.gen++
	gen := c.gen
	c.mu.Unlock()

	s, err := c.transport.Open(c.clu.IP, int(c.clu.Port), sockReceiver{c, gen})

	c.mu.Lock()
	c.opening = nil
	switch {
	case err != nil:
		c.log.Error("opening the UDP socket failed", "err", err)
	case c.state == StateStopped || c.sock != nil:
		s.Close()
	default:
		// A new socket gets its own three tries (spec §5.4.5).
		c.pingTimeouts = 0
		ms := &mainSocket{s: s, gen: gen, out: &sender{t: c.t}}
		c.sock = ms
		c.spawnLocked(func() { ms.out.run(s) })
		c.log.Debug("opened the UDP socket")
	}
	c.mu.Unlock()
	sig.fire()
}

func (c *core) closeSocketLocked() {
	ms := c.sock
	c.sock = nil
	ms.out.close()
	ms.s.Close()
}

// socketFailed: the transport lost the socket outside Stop (§5.4.1 item 4).
func (c *core) socketFailed(gen int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped || c.sock == nil || c.sock.gen != gen {
		return
	}
	c.log.Warn("the UDP socket failed", "err", err)
	c.failAllPendingLocked(ErrConnectionClosed)
	ms := c.sock
	c.sock = nil
	ms.out.close()
	c.setStateLocked(StateDegraded)
	c.needsRecovery = c.needsRecovery || len(c.desired) > 0
}

// abandonSocketLocked retires a socket after three unanswered pings.
func (c *core) abandonSocketLocked() {
	c.log.Warn("pings unanswered; replacing the UDP socket", "count", c.pingTimeouts)
	c.failAllPendingLocked(ErrConnectionClosed)
	c.closeSocketLocked()
	if c.state != StateStopped {
		c.setStateLocked(StateDegraded)
		c.needsRecovery = len(c.desired) > 0
	}
}

// Requests (spec §5.4.1).

func (c *core) request(ms *mainSocket, payload, msgID string) (wire.Response, error) {
	id := strings.ToLower(msgID)
	frame := wire.Request(id, payload)
	data, err := c.cipher.Encrypt([]byte(frame))
	if err != nil {
		return wire.Response{}, err
	}
	c.mu.Lock()
	switch {
	case c.state == StateStopped || c.sock != ms:
		c.mu.Unlock()
		return wire.Response{}, ErrConnectionClosed
	case c.pending[id] != nil:
		c.mu.Unlock()
		return wire.Response{}, errDuplicateID
	}
	c.token++
	token := c.token
	w := newWaiter(c.t)
	c.pending[id] = &pendingRequest{token: token, w: w}
	c.mu.Unlock()

	// The deadline counts from handing the datagram over and bounds the
	// whole attempt, also a send that never completes.
	timer := c.clock.AfterFunc(c.timeout, func() { c.failPending(id, token, ErrTimeout) })
	c.log.Debug("sent", "frame", frame)
	ms.out.push(sendItem{data: data,
		before: func() bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			if p := c.pending[id]; p == nil || p.token != token || c.state == StateStopped {
				return false // ended before it went out: nothing is sent
			}
			if id == c.keepAlive {
				c.pingSentAt, c.hasPingSent = c.clock.Now(), true
			}
			return true
		},
		after: func(err error) {
			if err != nil {
				c.failPending(id, token, err)
			}
		}})
	r := w.wait().(reply)
	timer.Stop()
	return r.resp, r.err
}

func (c *core) failPending(id string, token uint64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.pending[id]; p != nil && p.token == token {
		delete(c.pending, id)
		p.w.fire(reply{err: err})
	}
}

func (c *core) failAllPendingLocked(err error) {
	for id, p := range c.pending {
		delete(c.pending, id)
		p.w.fire(reply{err: err})
	}
}

// handleDatagram is the single reader of the socket (spec §5.4.1, §5.4.3).
func (c *core) handleDatagram(gen int, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped || c.sock == nil || c.sock.gen != gen {
		return // stopped, or a socket that was replaced: dropped whole
	}
	plain, ok := c.cipher.Decrypt(data)
	if !ok {
		c.log.Warn("dropping a datagram that does not decrypt", "bytes", len(data))
		return
	}
	if !utf8.Valid(plain) {
		c.log.Warn("dropping a datagram that is not UTF-8")
		return
	}
	text := model.TrimSpace(string(plain))
	resp, ok := wire.ParseResponse(text)
	if !ok {
		c.log.Debug("dropping an unsupported frame", "frame", text)
		return
	}
	c.log.Debug("received", "frame", text)
	if p := c.pending[resp.MsgID]; p != nil {
		delete(c.pending, resp.MsgID)
		now := c.clock.Now()
		c.lastReply, c.hasReply = now, true
		if resp.MsgID == c.keepAlive && c.hasPingSent {
			c.lastRTT, c.hasRTT, c.hasPingSent = now.Sub(c.pingSentAt), true, false
		}
		p.w.fire(reply{resp: resp})
		return
	}
	if !resp.IsClientReport() {
		c.log.Debug("dropping a reply with no pending request", "msgId", resp.MsgID)
		return
	}
	// Routed by the subscription number (spec §5.4.3).
	r, ok := wire.ParseReport(resp.Payload)
	keys, known := c.subs[r.Session]
	if !ok || !known || len(r.Values) != len(keys) {
		c.log.Debug("dropping a report for an unknown subscription or with a wrong value count")
		return
	}
	c.lastReport, c.hasReport = c.clock.Now(), true
	c.publishLocked(Report{Keys: keys, Slots: valueSlots(r.Values)})
}

func valueSlots(values []model.Value) []Slot {
	out := make([]Slot, len(values))
	for i, v := range values {
		out[i] = Slot{Value: v}
	}
	return out
}

func failedSlots(n int) []Slot {
	out := make([]Slot, n)
	for i := range out {
		out[i].Failed = true
	}
	return out
}

func anyFailed(slots []Slot) bool {
	for _, s := range slots {
		if s.Failed {
			return true
		}
	}
	return false
}

func allFailed(slots []Slot) bool {
	for _, s := range slots {
		if !s.Failed {
			return false
		}
	}
	return true
}

// Ping and recovery (spec §5.3.1, §5.4.5).

func (c *core) ping(gateHeld bool) bool {
	c.mu.Lock()
	if c.state == StateStopped || c.state == StateIdle || c.pending[c.keepAlive] != nil {
		c.mu.Unlock()
		return false
	}
	if c.sock == nil {
		// A degraded session without a socket opens one first.
		c.mu.Unlock()
		c.ensureSocket()
		c.mu.Lock()
	}
	ms := c.sock
	switch {
	case c.state == StateStopped || c.pending[c.keepAlive] != nil:
		c.mu.Unlock()
		return false
	case ms == nil:
		c.setStateLocked(StateDegraded)
		c.needsRecovery = c.needsRecovery || len(c.desired) > 0
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	_, err := c.request(ms, wire.CheckAlive, c.keepAlive)

	c.mu.Lock()
	if c.state == StateStopped || c.sock != ms || errors.Is(err, errDuplicateID) {
		c.mu.Unlock()
		return false // a reply on a socket that is gone says nothing about the current one
	}
	if err == nil {
		c.pingTimeouts = 0
		if c.needsRecovery && len(c.desired) > 0 {
			c.mu.Unlock()
			c.recover(gateHeld)
			c.mu.Lock()
			if c.state == StateStopped || c.sock != ms {
				c.mu.Unlock()
				return false
			}
		}
		c.setStateLocked(StateReady)
		c.mu.Unlock()
		return true
	}
	c.log.Warn("ping failed", "err", err)
	c.setStateLocked(StateDegraded)
	c.needsRecovery = c.needsRecovery || len(c.desired) > 0
	if errors.Is(err, ErrTimeout) {
		c.pingTimeouts++
		if c.pingTimeouts >= pingTimeoutsBeforeReplacing {
			c.abandonSocketLocked()
		}
	}
	c.mu.Unlock()
	return false
}

// recover registers the desired keys again, values through the report path.
func (c *core) recover(gateHeld bool) {
	if !gateHeld {
		c.gate.acquire()
		defer c.gate.release()
	}
	c.mu.Lock()
	if c.state == StateStopped || !c.needsRecovery || len(c.desired) == 0 {
		c.mu.Unlock()
		return
	}
	keys, ms := c.desired, c.sock
	c.mu.Unlock()
	slots := c.registerLocked(keys, true)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped {
		return
	}
	c.scheduleRenewalLocked()
	if allFailed(slots) {
		return
	}
	// A partial failure keeps the flag; so does a socket lost meanwhile.
	c.needsRecovery = anyFailed(slots) || c.sock != ms
}

// Registration (spec §5.4.2).

func (c *core) register(keys []model.ValueRef) []Slot {
	c.gate.acquire()
	defer c.gate.release()
	c.mu.Lock()
	if c.state == StateStopped || c.state == StateIdle {
		c.mu.Unlock()
		return failedSlots(len(keys))
	}
	c.desired = slices.Clone(keys)
	ms := c.sock
	c.mu.Unlock()
	slots := c.registerLocked(keys, false)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped {
		return slots
	}
	c.needsRecovery = len(keys) > 0 && (anyFailed(slots) || c.sock != ms)
	c.scheduleRenewalLocked()
	return slots
}

// registerLocked is the body of a registration; the caller holds the gate.
// With publish (renewal, recovery) every answered chunk reaches the report
// stream as soon as it is answered.
func (c *core) registerLocked(keys []model.ValueRef, publish bool) []Slot {
	c.mu.Lock()
	c.hasDeadline = false
	if c.state == StateStopped {
		c.mu.Unlock()
		return failedSlots(len(keys))
	}
	if len(keys) == 0 {
		c.needsRecovery = false
		numbers := sortedNumbers(c.subs)
		c.chunks = nil
		c.mu.Unlock()
		c.destroy(numbers)
		return []Slot{}
	}
	lists := wire.RegisterChunks(keys, c.frameLimit)
	if c.next-1+int64(len(lists)-c.keptLocked(lists)) > maxNumber {
		c.mu.Unlock()
		c.restartNumbering()
		c.mu.Lock()
	}
	// A chunk keeps its number only while its key list is identical, so a
	// late notification of an old list never lands on new keys.
	plan := make([]chunk, len(lists))
	for i, l := range lists {
		if i < len(c.chunks) && slices.Equal(c.chunks[i].keys, l) {
			plan[i] = c.chunks[i]
		} else {
			plan[i] = chunk{number: c.next, keys: l}
			c.next++
		}
	}
	c.mu.Unlock()

	// One chunk at a time: CLUs drop registrations sent back to back.
	results := make([]Slot, 0, len(keys))
	for _, ch := range plan {
		slots := c.registerChunk(ch)
		results = append(results, slots...)
		c.mu.Lock()
		if c.state != StateStopped {
			if anyFailed(slots) {
				c.needsRecovery = true
			} else if publish {
				c.publishLocked(Report{Keys: ch.keys, Slots: slots})
			}
		}
		c.mu.Unlock()
	}

	c.mu.Lock()
	if c.state == StateStopped {
		c.mu.Unlock()
		return results // stop destroys them
	}
	c.chunks = plan
	used := map[int64]bool{}
	for _, ch := range plan {
		used[ch.number] = true
	}
	var stale []int64
	for _, n := range sortedNumbers(c.subs) {
		if !used[n] {
			stale = append(stale, n)
		}
	}
	c.mu.Unlock()
	c.destroy(stale)
	return results
}

func (c *core) keptLocked(lists [][]model.ValueRef) int {
	n := 0
	for i, l := range lists {
		if i < len(c.chunks) && slices.Equal(c.chunks[i].keys, l) {
			n++
		}
	}
	return n
}

func (c *core) registerChunk(ch chunk) []Slot {
	c.mu.Lock()
	ms := c.sock
	if c.state == StateStopped || ms == nil {
		c.mu.Unlock()
		return failedSlots(len(ch.keys))
	}
	// Remembered from the moment it is sent: the CLU may accept it even
	// when the reply is lost.
	c.subs[ch.number] = ch.keys
	if !c.hasDeadline {
		c.deadline, c.hasDeadline = c.clock.Now().Add(c.registerInterval), true
	}
	c.mu.Unlock()
	resp, err := c.request(ms, wire.ClientRegister(ch.number, ch.keys), c.newID())
	if err != nil {
		c.log.Warn("registration failed", "subscription", ch.number, "err", err)
		return failedSlots(len(ch.keys))
	}
	// Correlated by msg_id; the number in the reply is not checked.
	r, ok := wire.ParseReport(resp.Payload)
	if !ok || len(r.Values) != len(ch.keys) {
		c.log.Error("malformed registration reply", "subscription", ch.number)
		return failedSlots(len(ch.keys))
	}
	return valueSlots(r.Values)
}

// destroy forgets numbers and sends their clientDestroy without waiting.
func (c *core) destroy(numbers []int64) {
	c.mu.Lock()
	for _, n := range numbers {
		delete(c.subs, n)
	}
	ms := c.sock
	c.mu.Unlock()
	if ms != nil {
		c.sendDestroys(ms, numbers)
	}
}

func (c *core) sendDestroys(ms *mainSocket, numbers []int64) {
	for _, n := range numbers {
		frame := wire.Request(c.newID(), wire.ClientDestroy(n))
		data, err := c.cipher.Encrypt([]byte(frame))
		if err != nil {
			continue
		}
		c.log.Debug("sent", "frame", frame)
		ms.out.push(sendItem{data: data})
	}
}

// restartNumbering: out of six-digit numbers, destroy everything and start
// over on a new socket, whose new port makes every number new.
func (c *core) restartNumbering() {
	c.mu.Lock()
	numbers := sortedNumbers(c.subs)
	ms := c.sock
	c.mu.Unlock()
	c.destroy(numbers)
	if ms != nil {
		ms.out.drain(c.clock, c.timeout)
	}
	c.mu.Lock()
	c.chunks, c.next = nil, 1
	c.failAllPendingLocked(ErrConnectionClosed)
	if c.sock != nil {
		c.closeSocketLocked()
	}
	c.mu.Unlock()
	c.ensureSocket()
}

// Fetch (spec §5.4.2).

func (c *core) fetch(keys []model.ValueRef) []Slot {
	c.gate.acquire()
	defer c.gate.release()
	c.mu.Lock()
	ready := c.state != StateStopped && c.sock != nil
	c.mu.Unlock()
	if !ready {
		return failedSlots(len(keys))
	}
	results := make([]Slot, 0, len(keys))
	for _, chunk := range wire.FetchChunks(keys, c.frameLimit) {
		results = append(results, c.fetchChunk(chunk)...)
	}
	return results
}

func (c *core) fetchChunk(keys []model.ValueRef) []Slot {
	c.mu.Lock()
	ms := c.sock
	stopped := c.state == StateStopped
	c.mu.Unlock()
	if stopped || ms == nil {
		return failedSlots(len(keys))
	}
	resp, err := c.request(ms, wire.FetchValues(keys), c.newID())
	if err != nil {
		c.log.Warn("fetchValues failed", "err", err)
		return failedSlots(len(keys))
	}
	values, ok := wire.ParseValues(resp.Payload)
	if !ok || len(values) != len(keys) {
		c.log.Error("malformed fetchValues reply")
		return failedSlots(len(keys))
	}
	return valueSlots(values)
}

// Actions (spec §5.3.4).

func (c *core) execute(a model.ActionRef) error {
	c.mu.Lock()
	ms := c.sock
	stopped := c.state == StateStopped
	c.mu.Unlock()
	if stopped || ms == nil {
		return ErrNotConnected
	}
	payload, err := wire.Action(a)
	if err != nil {
		return err
	}
	_, err = c.request(ms, payload, c.newID())
	return err
}

// Loops (spec §5.4.4).

func (c *core) startPingLoopLocked() {
	c.pingEpoch++
	epoch := c.pingEpoch
	c.spawnLocked(func() {
		for c.sleepPing(epoch) {
			c.ping(false)
		}
	})
}

// sleepPing waits one ping interval; false when the loop must end.
func (c *core) sleepPing(epoch int) bool {
	c.mu.Lock()
	if c.state == StateStopped || epoch != c.pingEpoch {
		c.mu.Unlock()
		return false
	}
	w := newWaiter(c.t)
	c.pingSleep = w
	c.mu.Unlock()
	timer := c.clock.AfterFunc(c.pingInterval, func() { w.fire(true) })
	woke := w.wait() == true
	timer.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pingSleep == w {
		c.pingSleep = nil
	}
	return woke && c.state != StateStopped && epoch == c.pingEpoch
}

// scheduleRenewalLocked arms the re-registration: registerInterval after
// the latest registration started, or from now when no chunk went out.
func (c *core) scheduleRenewalLocked() {
	if c.regTimer != nil {
		c.regTimer.Stop()
		c.regTimer = nil
	}
	c.regEpoch++
	if c.state == StateStopped || len(c.desired) == 0 {
		return
	}
	d := c.registerInterval
	if c.hasDeadline {
		d = c.deadline.Sub(c.clock.Now())
	}
	epoch := c.regEpoch
	c.regTimer = c.clock.AfterFunc(d, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if epoch == c.regEpoch {
			c.spawnLocked(func() { c.renew(epoch) })
		}
	})
}

// renew is the loop's re-registration.
func (c *core) renew(epoch int) {
	c.gate.acquire()
	defer c.gate.release()
	c.mu.Lock()
	if c.state == StateStopped || len(c.desired) == 0 || epoch != c.regEpoch {
		c.mu.Unlock()
		return // stopped, or another registration rescheduled the loop
	}
	if c.state == StateDegraded {
		// No renewal while degraded: the deadline is forfeited, and the
		// recovery after the next answered ping registers anyway.
		c.hasDeadline = false
		c.mu.Unlock()
		return
	}
	keys, ms := c.desired, c.sock
	c.mu.Unlock()
	slots := c.registerLocked(keys, true)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateStopped {
		return
	}
	c.needsRecovery = anyFailed(slots) || c.sock != ms
	c.scheduleRenewalLocked()
}

// Subscribers.

func (c *core) subscribe() (<-chan Report, func()) {
	ch := make(chan Report, 64)
	c.mu.Lock()
	id := c.subID
	c.subID++
	c.reportSubs[id] = ch
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.reportSubs, id)
		c.mu.Unlock()
	}
}

// publishLocked delivers a report to every stream; a full stream drops its
// oldest report.
func (c *core) publishLocked(r Report) {
	for _, ch := range c.reportSubs {
		for {
			select {
			case ch <- r:
			default:
				select {
				case <-ch:
				default:
				}
				continue
			}
			break
		}
	}
}

func (c *core) subscribeState() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	c.mu.Lock()
	id := c.subID
	c.subID++
	c.stateSubs[id] = ch
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.stateSubs, id)
		c.mu.Unlock()
	}
}

func (c *core) diagnostics() Diagnostics {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	d := Diagnostics{State: c.state, SampledAt: now, RequestedKeys: len(c.desired), RecoveryPending: c.needsRecovery,
		ConsecutivePingTimeouts: c.pingTimeouts}
	if c.hasReply {
		v := now.Sub(c.lastReply)
		d.SinceLastReply = &v
	}
	if c.hasReport {
		v := now.Sub(c.lastReport)
		d.SinceLastReport = &v
	}
	if c.hasRTT {
		v := c.lastRTT
		d.LastPingRoundTrip = &v
	}
	return d
}
