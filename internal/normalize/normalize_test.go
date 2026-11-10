package normalize

import (
	"testing"
	"time"
)

// An absurd precision from data.json returns at once, as Python does.
func TestHugePrecisionIsCheap(t *testing.T) {
	start := time.Now()
	if got := FormatNumber(1.5, 1<<40); got != "1.5" {
		t.Fatalf("got %q", got)
	}
	if got := FormatNumber(-1.5, -(1 << 40)); got != "-0.0" {
		t.Fatalf("got %q", got)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("took %v", d)
	}
}
