package rt

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

// Slow-cadence P refresher (60 Hz and below).
//
// When the locked UI thread stays inside C (gk_run) for more than 10 ms between callbacks,
// Go's sysmon retakes its P, pushes it onto sched.pidle, and enters deep sleep. On the next
// frame's callback, exitsyscallTryGetP pops the top P off sched.pidle and wakes sysmon.
// Because exitsyscall only increments P.syscalltick (never P.schedtick), sysmon wakes ~6 us
// later while the callback is still in Go, sees pd.schedwhen + 10ms <= now, and calls
// preemptone(P) (stackPreempt + SIGURG). That forces the locked UI thread through
// stoplockedm() (futex_wait on the main thread, +10 to 12 us/frame and extra main-thread
// voluntary context switches).
//
// Waking the UI thread itself mid-frame through GPUI's foreground executor adds more
// main-thread CPU (calloop event loop dispatch + epoll wakeup) than it saves. Instead, when
// consecutive real turns arrive 10 to 250 ms apart, a single background goroutine wakes
// ~1.5 ms before the next expected callback, calls runtime.Gosched() (routing through the
// global runq so execute(..., inheritTime=false) increments P.schedtick), and parks via
// time.Sleep (pushing that freshly ticked P onto the top of sched.pidle). The UI thread's
// next exitsyscallTryGetP then pops a P with a fresh schedtick, avoiding preemption and
// stoplockedm() at ~20 ns main-thread cost and zero extra main-thread wakeups.
//
// Fast cadences (120 Hz+, gap < 10 ms) never start the goroutine. Idle apps stop waking
// after one post-frame check and park on a channel receive until the next slow-cadence
// burst or until gk.Run returns.

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

var (
	pkaActive    bool        // UI thread only: true during a windowed gk.Run (false headless or GK_NO_KEEPALIVE)
	pkaStats     bool        // GK_KEEPALIVE_STATS=1 prints refresh count on exit
	pkaEpoch     time.Time   // monotonic origin for pkaNowNs
	pkaPrevNs    int64       // UI thread only: timestamp of previous real turn
	pkaSlowUntil int64       // UI thread only: same-frame burst window after a slow gap
	pkaState     *pkaSession // current gk.Run's background refresher session (nil until first slow gap)
)

type pkaSession struct {
	lastTurn  atomic.Int64
	lastGap   atomic.Int64
	sleeping  atomic.Bool
	refreshes atomic.Uint64
	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
}

func startPKeepAlive() {
	pkaActive = os.Getenv("GK_NO_KEEPALIVE") == ""
	pkaStats = os.Getenv("GK_KEEPALIVE_STATS") != ""
	pkaEpoch = time.Now()
	pkaPrevNs = 0
	pkaSlowUntil = 0
	pkaState = nil
}

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
		// Second turn in the same slow-cadence frame (e.g. EVENT followed by VIEWS):
		// slide lastTurn forward without changing lastGap.
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
			// On GOMAXPROCS > 1, nudge sysmon ~2.5 ms earlier (still past the 10 ms retake
			// threshold) so any P still attached to the UI thread in C is retaken and pushed
			// onto sched.pidle before the final refresh just ahead of the next frame.
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
				// Yield through the global runq so execute(..., inheritTime=false)
				// increments this P's schedtick; parking on the sleep below then pushes
				// this freshly ticked P onto the top of sched.pidle.
				runtime.Gosched()
				s.refreshes.Add(1)
			}
			// Wait past the next expected frame so steady 60 Hz needs zero channel wakes
			// from the UI thread; if the app went idle, fall through to infinite park.
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
