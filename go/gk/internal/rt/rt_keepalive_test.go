package rt

import (
	"testing"
	"time"
)

func TestPKeepAliveCadencesAndIdle(t *testing.T) {
	// 1. Fast cadence (< 10 ms, e.g. 120 Hz / 1000 Hz): goroutine never starts.
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

	// 2. Slow cadence (~16.6 ms = 60 Hz) followed by idle: fires during active frames,
	// then stops completely and does not wake periodically while idle.
	startPKeepAlive()
	for i := 0; i < 6; i++ {
		pkaNoteTurn()
		time.Sleep(16600 * time.Microsecond)
	}
	if pkaState == nil {
		stopPKeepAlive()
		t.Fatal("expected keep-alive goroutine to start at 60 Hz cadence")
	}
	// Let the final one-shot refresh and post-frame window settle into idle park.
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
	// Stay idle for 100 ms: zero further refreshes must happen.
	time.Sleep(100 * time.Millisecond)
	afterIdle := pkaState.refreshes.Load()
	if afterIdle != afterActive {
		stopPKeepAlive()
		t.Fatalf("keep-alive woke while idle: refreshes went from %d to %d", afterActive, afterIdle)
	}
	stopPKeepAlive()
}
