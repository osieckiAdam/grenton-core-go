package clu

// SetNextNumber lets a test reach the six-digit rollover without a million
// registrations.
func SetNextNumber(s *Session, n int64) {
	s.c.mu.Lock()
	s.c.next = n
	s.c.mu.Unlock()
}
