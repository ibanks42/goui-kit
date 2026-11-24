package rt

import (
	"testing"
	"time"
)

// TestPKeepAliveCadencesAndIdle verifies that fast cadences (<10 ms) never
// spawn the keep-alive goroutine, 60 Hz cadences start it and trigger refreshes,
// and stopping turn notifications parks it with no further refreshes.
func TestPKeepAliveCadencesAndIdle(t *testing.T) {
	startPKeepAlive()
	for i := 0; i < 10; i++ {
		pkaNoteTurn()
		time.Sleep(2 * time.Millisecond)
	}
	if pkaState != nil {
		stopPKeepAlive()
		t.Fatalf("expected no keep-alive goroutine at fast cadence (<10ms), got session with refreshes=%d", pkaState.refreshes.Load())
	}
	stopPKeepAlive()

	startPKeepAlive()
	for i := 0; i < 6; i++ {
		pkaNoteTurn()
		time.Sleep(16600 * time.Microsecond)
	}
	if pkaState == nil {
		stopPKeepAlive()
		t.Fatal("expected keep-alive goroutine to start at 60 Hz cadence")
	}
	time.Sleep(60 * time.Millisecond)
	afterActive := pkaState.refreshes.Load()
	if afterActive == 0 {
		stopPKeepAlive()
		t.Fatal("expected at least one refresh during 60 Hz burst")
	}
	if !pkaState.sleeping.Load() {
		stopPKeepAlive()
		t.Fatal("expected keep-alive goroutine to park when idle")
	}
	time.Sleep(100 * time.Millisecond)
	afterIdle := pkaState.refreshes.Load()
	if afterIdle != afterActive {
		stopPKeepAlive()
		t.Fatalf("keep-alive woke while idle: refreshes went from %d to %d", afterActive, afterIdle)
	}
	stopPKeepAlive()
}
