package rt

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

// Timing thresholds for the scheduler keep-alive goroutine.
//
// Go's background sysmon thread retakes idle Ps (logical processors) after roughly
// 10 ms without Go execution. In an interactive GUI app running at 60 Hz (one
// frame every ~16.6 ms), the locked UI OS thread spends most of each frame
// blocked inside the native Rust event loop; when the next C-to-Go callback
// arrives, Go has to wake and reacquire a P, adding jitter to frame latency.
//
// To avoid that stall without burning CPU when idle, this file watches the
// inter-turn gap on the UI thread:
//   - Fast bursts (< pkaMinGapNs, 10 ms): Go never parks a P, so no goroutine runs.
//   - Steady display cadences (pkaMinGapNs..pkaMaxGapNs, 10..250 ms): a single
//     background goroutine wakes pkaLeadNs (550 us) before the predicted next
//     turn and calls runtime.Gosched() to keep a P warm. For gaps >= 13.5 ms, an
//     earlier nudge runs pkaNudgeLeadNs (2 ms) before the main wake-up.
//   - Multiple callbacks within one frame (< pkaSameFrameNs, 2 ms, while inside
//     the pkaSlowHoldNs 50 ms hold window): refresh lastTurn without distorting
//     the smoothed cadence estimate.
//   - Idle: once no new turn arrives after a refresh, the goroutine sets
//     sleeping=true, re-checks lastTurn to close the race window, and blocks on
//     s.wake with zero CPU usage.
const (
	pkaSameFrameNs = int64(2 * time.Millisecond)
	pkaMinGapNs    = int64(10 * time.Millisecond)
	pkaMaxGapNs    = int64(250 * time.Millisecond)
	pkaSlowHoldNs  = int64(50 * time.Millisecond)
	pkaLeadNs      = int64(550 * time.Microsecond)
	pkaNudgeLeadNs = int64(2000 * time.Microsecond)
	pkaMinDelayNs  = int64(11500 * time.Microsecond)
	pkaMaxDelayNs  = int64(16100 * time.Microsecond)
	pkaMinPostNs   = int64(4 * time.Millisecond)
	pkaMaxPostNs   = int64(14 * time.Millisecond)
)

// UI-thread-only keep-alive state for the active Run session.
// pkaState is nil until the first turn gap in [pkaMinGapNs, pkaMaxGapNs]
// triggers lazy creation of the keep-alive goroutine.
var (
	pkaActive    bool
	pkaStats     bool
	pkaEpoch     time.Time
	pkaPrevNs    int64
	pkaSlowUntil int64
	pkaState     *pkaSession
)

// pkaSession holds the atomic timestamps and lifecycle channels shared between
// the UI thread (writer of lastTurn/lastGap) and the keep-alive goroutine.
type pkaSession struct {
	lastTurn  atomic.Int64  // monotonic ns since pkaEpoch of the latest UI turn
	lastGap   atomic.Int64  // smoothed inter-turn interval in ns
	sleeping  atomic.Bool   // true while the goroutine is parked on wake
	refreshes atomic.Uint64 // diagnostic counter printed when GK_KEEPALIVE_STATS is set
	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
}

// startPKeepAlive initializes keep-alive configuration at the start of a run.
// Set GK_NO_KEEPALIVE to disable the goroutine or GK_KEEPALIVE_STATS to print
// the refresh count on exit.
func startPKeepAlive() {
	pkaActive = os.Getenv("GK_NO_KEEPALIVE") == ""
	pkaStats = os.Getenv("GK_KEEPALIVE_STATS") != ""
	pkaEpoch = time.Now()
	pkaPrevNs = 0
	pkaSlowUntil = 0
	pkaState = nil
}

// stopPKeepAlive stops the keep-alive goroutine if it was started and waits
// for it to exit.
func stopPKeepAlive() {
	pkaActive = false
	s := pkaState
	pkaState = nil
	if s != nil {
		close(s.stop)
		<-s.done
		if pkaStats {
			fmt.Fprintf(os.Stderr, "gk-keepalive refreshes=%d\n", s.refreshes.Load())
		}
	} else if pkaStats {
		fmt.Fprintf(os.Stderr, "gk-keepalive refreshes=0\n")
	}
}

func pkaNowNs() int64 {
	return time.Since(pkaEpoch).Nanoseconds()
}

// pkaNoteTurn records the arrival time of a UI turn on the locked UI thread.
// Gaps in [pkaMinGapNs, pkaMaxGapNs] lazily start or wake the keep-alive
// goroutine and update the smoothed inter-turn interval; closely spaced
// same-frame turns refresh lastTurn without perturbing the cadence estimate.
func pkaNoteTurn() {
	now := pkaNowNs()
	prev := pkaPrevNs
	pkaPrevNs = now
	if prev == 0 {
		return
	}
	gap := now - prev
	if gap >= pkaMinGapNs && gap <= pkaMaxGapNs {
		pkaSlowUntil = now + pkaSlowHoldNs
	} else if gap < pkaSameFrameNs && now < pkaSlowUntil {
		if s := pkaState; s != nil {
			s.lastTurn.Store(now)
		}
		return
	} else {
		pkaSlowUntil = 0
		return
	}
	s := pkaState
	if s == nil {
		s = &pkaSession{
			wake: make(chan struct{}, 1),
			stop: make(chan struct{}),
			done: make(chan struct{}),
		}
		s.lastGap.Store(gap)
		s.lastTurn.Store(now)
		pkaState = s
		go s.run()
		return
	}
	// Exponentially smooth steady cadence changes (3/4 old, 1/4 new).
	if prevGap := s.lastGap.Load(); prevGap > 0 && gap >= prevGap/2 && gap <= prevGap*2 {
		gap = (prevGap*3 + gap) / 4
	}
	s.lastGap.Store(gap)
	s.lastTurn.Store(now)
	if s.sleeping.Load() {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// sleepUntil waits for duration d on timer t, returning false if stop closes first.
func (s *pkaSession) sleepUntil(t *time.Timer, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t.Reset(d)
	select {
	case <-s.stop:
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		return false
	case <-t.C:
		return true
	}
}

// run sleeps until shortly before each expected frame callback, yields to keep
// a scheduler P active, and parks on s.wake whenever turns stop arriving.
func (s *pkaSession) run() {
	defer close(s.done)
	t := time.NewTimer(time.Hour)
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	var handled int64
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		lt := s.lastTurn.Load()
		if lt != handled && lt != 0 {
			gap := s.lastGap.Load()
			delay := gap - pkaLeadNs
			if delay < pkaMinDelayNs {
				delay = pkaMinDelayNs
			}
			if delay > pkaMaxDelayNs {
				delay = pkaMaxDelayNs
			}
			// For longer intervals, issue an early nudge before the main wake-up.
			if delay >= pkaMinDelayNs+pkaNudgeLeadNs {
				nudgeRem := (lt + delay - pkaNudgeLeadNs) - pkaNowNs()
				if nudgeRem > 0 && !s.sleepUntil(t, time.Duration(nudgeRem)) {
					return
				}
				if s.lastTurn.Load() == lt {
					runtime.Gosched()
				}
			}
			rem := (lt + delay) - pkaNowNs()
			if rem > 0 && !s.sleepUntil(t, time.Duration(rem)) {
				return
			}
			if s.lastTurn.Load() == lt {
				handled = lt
				runtime.Gosched()
				s.refreshes.Add(1)
			}
			// Wait past the expected frame arrival before checking for another turn.
			w := gap - delay + pkaLeadNs
			if w < pkaMinPostNs {
				w = pkaMinPostNs
			}
			if w > pkaMaxPostNs {
				w = pkaMaxPostNs
			}
			if !s.sleepUntil(t, time.Duration(w)) {
				return
			}
			continue
		}
		// Mark sleeping and re-check lastTurn to avoid missing a concurrent wake.
		s.sleeping.Store(true)
		if s.lastTurn.Load() != handled {
			s.sleeping.Store(false)
			continue
		}
		select {
		case <-s.stop:
			s.sleeping.Store(false)
			return
		case <-s.wake:
			s.sleeping.Store(false)
		}
	}
}
