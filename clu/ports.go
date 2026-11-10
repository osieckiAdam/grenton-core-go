// Package clu is the Runtime module of Grenton Core (spec §5): the
// encrypted UDP session with one CLU, the state store, and the port
// contract (spec §7) through which every side effect enters.
package clu

import (
	"crypto/rand"
	"io"
	"time"
)

// Transport opens UDP sockets to a CLU (spec §7.1). Every socket has its
// own ephemeral local port, the port the CLU sends notifications to.
type Transport interface {
	// Open binds a socket for the CLU at host:port. The transport delivers
	// every datagram it receives on it to r, from one goroutine, in order,
	// and calls r.Closed once if the socket fails without Close.
	Open(host string, port int, r Receiver) (Socket, error)
}

// Receiver gets what a socket receives.
type Receiver interface {
	Datagram(data []byte)
	Closed(err error)
}

// Socket sends to the CLU it was opened for.
type Socket interface {
	// Send sends one datagram. It may block; the session never waits for
	// it longer than one response timeout.
	Send(data []byte) error
	// Close closes the socket; a blocked Send returns.
	Close() error
}

// Clock is the session's time (spec §7.4). Every timer of the session goes
// through it, so a test can move time by hand.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f on its own goroutine once d has passed.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending AfterFunc call.
type Timer interface {
	// Stop prevents the call if it has not happened; it reports whether
	// it did.
	Stop() bool
}

// SystemClock is the real clock.
type SystemClock struct{}

// Now returns time.Now().
func (SystemClock) Now() time.Time { return time.Now() }

// AfterFunc is time.AfterFunc.
func (SystemClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// randomSource defaults to crypto/rand (spec §7.5).
func randomSource(r io.Reader) io.Reader {
	if r == nil {
		return rand.Reader
	}
	return r
}
