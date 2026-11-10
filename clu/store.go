package clu

import (
	"sync"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// Slot is one position of a registration or fetch result (spec §5.4.2
// item 4): Failed, or a Value the CLU reported (null included).
type Slot struct {
	Failed bool
	Value  model.Value
}

// Report is a set of values for keys, in order, from a registration reply
// or a clientReport notification.
type Report struct {
	Keys  []model.ValueRef
	Slots []Slot
}

// Change is one actual change of a stored value (spec §5.5 Δ events).
type Change struct {
	Key    model.ValueRef
	Old    model.Value
	HadOld bool // false when the key was not yet known
	New    model.Value
}

// Store is the state store of spec §5.5: the last value of every key, safe
// for concurrent reads and writes. Keys are compared with ValueRef.Key, so
// a variable's ObjectName does not matter.
type Store struct {
	mu        sync.RWMutex
	values    map[model.ValueRef]model.Value
	order     []model.ValueRef
	known     map[model.ValueRef]bool
	listeners map[int]*listener
	nextID    int
}

type listener struct {
	keys map[model.ValueRef]bool // nil: every key
	fn   func([]Change)
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{values: map[model.ValueRef]model.Value{}, known: map[model.ValueRef]bool{},
		listeners: map[int]*listener{}}
}

// Register adds a key in registration order; a known key is a no-op. Its
// value stays unknown until a slot sets it.
func (s *Store) Register(ref model.ValueRef) {
	k := ref.Key()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known[k] {
		return
	}
	s.known[k] = true
	s.order = append(s.order, k)
}

// Keys returns the registered keys in registration order.
func (s *Store) Keys() []model.ValueRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.ValueRef(nil), s.order...)
}

// Get returns the current value and false while it is unknown.
func (s *Store) Get(ref model.ValueRef) (model.Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[ref.Key()]
	return v, ok
}

// Update writes slots positionally: a failed slot is skipped, a value
// overwrites, null included. Listeners hear about actual changes, after the
// store holds them, in order.
func (s *Store) Update(keys []model.ValueRef, slots []Slot) {
	s.mu.Lock()
	var changes []Change
	for i, ref := range keys {
		if i >= len(slots) || slots[i].Failed {
			continue
		}
		k := ref.Key()
		old, had := s.values[k]
		v := slots[i].Value
		if had && old == v {
			continue
		}
		s.values[k] = v
		changes = append(changes, Change{Key: k, Old: old, HadOld: had, New: v})
	}
	var notify []func()
	if len(changes) > 0 {
		for _, l := range s.listeners {
			l := l
			var mine []Change
			for _, c := range changes {
				if l.keys == nil || l.keys[c.Key] {
					mine = append(mine, c)
				}
			}
			if len(mine) > 0 {
				notify = append(notify, func() { l.fn(mine) })
			}
		}
	}
	s.mu.Unlock()
	for _, n := range notify {
		n()
	}
}

// Listen calls fn with the changes of the given keys (all keys when none
// are given), synchronously from Update, after the store holds them. fn
// must not block or call back into the store's writers. The returned
// function stops listening.
func (s *Store) Listen(fn func([]Change), keys ...model.ValueRef) (cancel func()) {
	var set map[model.ValueRef]bool
	if len(keys) > 0 {
		set = map[model.ValueRef]bool{}
		for _, k := range keys {
			set[k.Key()] = true
		}
	}
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	s.listeners[id] = &listener{keys: set, fn: fn}
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.listeners, id)
		s.mu.Unlock()
	}
}
