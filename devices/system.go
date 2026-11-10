package devices

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/model"
)

// ErrUnknownCLU: an action names a CLU the interface does not have.
var ErrUnknownCLU = errors.New("devices: the action's CLU is not in the interface")

// SystemConfig configures a [System]; every zero field takes the session
// default (see clu.Config).
type SystemConfig struct {
	Transport        clu.Transport
	Clock            clu.Clock
	Random           io.Reader
	Logger           *slog.Logger
	ResponseTimeout  time.Duration
	PingInterval     time.Duration
	RegisterInterval time.Duration
	FrameLimit       int
	// Activity is the test hook of clu.Config, shared by every session.
	Activity clu.Activity
}

// System runs a whole interface: one session per CLU, one state store,
// the devices bound to both. Commands go to the session of their CLU.
type System struct {
	in       *model.Interface
	store    *clu.Store
	mapping  Mapping
	devices  []BoundDevice
	sessions map[string]*clu.Session
	order    []string // CLU ids in interface order
	activity clu.Activity
}

// parallel runs f for every CLU and waits; its goroutines report to the
// Activity hook like the sessions' own.
func (s *System) parallel(f func(id string)) {
	var wg sync.WaitGroup
	for _, id := range s.order {
		wg.Add(1)
		if s.activity != nil {
			s.activity.Add(1)
		}
		go func() {
			defer wg.Done()
			if s.activity != nil {
				defer s.activity.Add(-1)
			}
			f(id)
		}()
	}
	if s.activity != nil {
		s.activity.Add(-1) // the caller is parked while it waits
		defer s.activity.Add(1)
	}
	wg.Wait()
}

// NewSystem maps the interface and creates its sessions; nothing is sent
// before Start. A bad key or IV is clu.ErrInvalidKeyMaterial.
func NewSystem(in *model.Interface, cfg SystemConfig) (*System, error) {
	cipher, err := clu.NewCipher(in.Encryption.Key, in.Encryption.IV)
	if err != nil {
		return nil, err
	}
	s := &System{in: in, store: clu.NewStore(), mapping: Map(in), sessions: map[string]*clu.Session{},
		activity: cfg.Activity}
	for _, c := range in.Clus {
		if _, dup := s.sessions[c.ID]; dup {
			continue
		}
		s.sessions[c.ID] = clu.NewSession(clu.Config{CLU: c, Cipher: cipher, Transport: cfg.Transport,
			Clock: cfg.Clock, Random: cfg.Random, Logger: cfg.Logger, ResponseTimeout: cfg.ResponseTimeout,
			PingInterval: cfg.PingInterval, RegisterInterval: cfg.RegisterInterval, FrameLimit: cfg.FrameLimit,
			Activity: cfg.Activity, OnReport: func(r clu.Report) { s.store.Update(r.Keys, r.Slots) }})
		s.order = append(s.order, c.ID)
	}
	s.devices = Bind(s.mapping.Devices, s.store, s)
	return s, nil
}

// keysOf lists the store's keys of one CLU, in registration order.
func (s *System) keysOf(cluID string) []model.ValueRef {
	var out []model.ValueRef
	for _, k := range s.store.Keys() {
		if k.CluID == cluID {
			out = append(out, k)
		}
	}
	return out
}

// Start starts every session and registers its keys, all CLUs in
// parallel; it returns when every CLU has answered or timed out. A CLU
// that does not answer is degraded and keeps retrying; the others work.
func (s *System) Start() {
	s.parallel(func(id string) {
		sess, keys := s.sessions[id], s.keysOf(id)
		sess.Start()
		if len(keys) > 0 {
			s.store.Update(keys, sess.Register(keys))
		}
	})
}

// Stop stops every session; afterwards the system holds no socket, timer
// or goroutine. It may be started again.
func (s *System) Stop() {
	s.parallel(func(id string) { s.sessions[id].Stop() })
}

// Execute sends an action through the session of its CLU.
func (s *System) Execute(a model.ActionRef) error {
	sess, ok := s.sessions[a.CluID]
	if !ok {
		return ErrUnknownCLU
	}
	return sess.Execute(a)
}

// Interface returns the interface the system runs.
func (s *System) Interface() *model.Interface { return s.in }

// Devices returns the bound devices in interface order.
func (s *System) Devices() []BoundDevice { return s.devices }

// PhysicalObjects returns the grouping by CLU object (spec §6.6).
func (s *System) PhysicalObjects() []PhysicalObject { return s.mapping.PhysicalObjects }

// Skipped lists, per page, the types of the widgets that did not parse.
func (s *System) Skipped() [][]*string { return s.mapping.Skipped }

// Entity returns the entity with the id, or nil.
func (s *System) Entity(id string) Entity {
	for _, d := range s.devices {
		for _, e := range d.Entities {
			if e.Info().ID == id {
				return e
			}
		}
	}
	return nil
}

// Store returns the state store.
func (s *System) Store() *clu.Store { return s.store }

// CLUs lists the CLU ids in interface order.
func (s *System) CLUs() []string { return append([]string(nil), s.order...) }

// Session returns the session of a CLU, or nil.
func (s *System) Session(cluID string) *clu.Session { return s.sessions[cluID] }
