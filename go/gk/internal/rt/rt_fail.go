// Panic and failure isolation across the Go/Rust boundary.
//
// A Go panic must never unwind across C/Rust stack frames (doing so crashes the
// process under both purego and cgo). Instead, boundary() catches panics inside
// onTurn, records the first failure in failErr, unwinds Go turn state, and
// issues CQuit so gk_run exits cleanly. Once gk_run returns to Run:
//   - *Error and *LibraryError are returned as ordinary Go errors.
//   - *PanicError (from app code or a gk.Go worker) is re-panicked in Go.

package rt

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

// PanicError wraps a panic raised by application code inside a UI callback or
// a gk.Go worker. Run shuts the native event loop down cleanly and then
// re-panics with this value so callers can recover it in normal Go code.
type PanicError struct {
	Value any
	Where string
	File  string
	Line  int
	Func  string
	Stack []byte
}

// Error formats the panic value, the application call site, and the captured
// goroutine stack trace.
func (p *PanicError) Error() string {
	s := fmt.Sprintf("gk: panic in %s: %v", p.Where, p.Value)
	if p.Line != 0 {
		s += fmt.Sprintf("\n    at %s:%d (%s)", p.File, p.Line, p.Func)
	}
	return s + "\n    gk.Run ended the app and panics again with this value (a *gk.PanicError; Value is the app's). Where it panicked:\n" + string(p.Stack)
}

// Unwrap returns p.Value if it implements error, enabling errors.Is and
// errors.As inspection on recovered *PanicError values.
func (p *PanicError) Unwrap() error {
	if e, ok := p.Value.(error); ok {
		return e
	}
	return nil
}

// LibraryError reports an internal panic caught on the Rust side of the C ABI
// (surfaced via ErrPoisoned on a call or a KPanic callback).
type LibraryError struct {
	Msg  string
	File string
	Line int
	Func string
}

// Error formats the Rust panic message together with the Go call site that
// triggered it and suggests GK_DEBUG=1 to defeat deferred batching.
func (e *LibraryError) Error() string {
	s := "gk: the library panicked: " + e.Msg
	if e.Line != 0 {
		s += fmt.Sprintf("\n    in the call at %s:%d (%s)", e.File, e.Line, e.Func)
	}
	return s + "\n    This is a fault inside the library (GPUI-kit or the binding's runtime), not in the app's Go code: the app cannot go on. Run it again with GK_DEBUG=1, which sends every change at once so the call above is the one that failed, and report the message with that line."
}

// First-error latch shared between the UI thread and background goroutines.
// failed provides a lock-free fast path checked on every turn and MustCall.
var (
	failMu   sync.Mutex
	failErr  error
	failed   atomic.Bool
	quitSent bool
)

// abortTurn is a sentinel panic value raised when a nested call observes that
// another failure has already latched, unwinding back to boundary() without
// overwriting failErr.
type abortTurn struct{}

// fail latches err as the run's terminal failure if none has been recorded yet.
func fail(err error) {
	failMu.Lock()
	if failErr == nil {
		failErr = err
		failed.Store(true)
	}
	failMu.Unlock()
}

// Failed reports whether a fatal error or application panic has been latched.
func Failed() bool { return failed.Load() }

// askQuit sends at most one CQuit request so the Rust event loop exits after
// a latched failure.
func askQuit() {
	if quitSent {
		return
	}
	if rc, _ := call(*NewReq(CQuit)); rc == 0 {
		quitSent = true
	}
}

// failOffThread latches err from a non-UI goroutine (for example, when CheckCx
// detects an off-thread gk call), wakes the UI loop so it quits, and ends the
// offending goroutine via runtime.Goexit without touching UI-thread state.
func failOffThread(err error) {
	fail(err)
	postWake()
	runtime.Goexit()
}

// recovered classifies a panic caught at a turn boundary and latches it into
// failErr. abortTurn sentinels are skipped because their cause is already latched.
func recovered(r any, stack []byte, skip int) {
	switch e := r.(type) {
	case abortTurn:
	case *Error:
		fail(e)
	case *LibraryError:
		fail(e)
	default:
		p := &PanicError{Value: r, Where: "app code", Stack: stack}
		p.File, p.Line, p.Func = appFrame(skip + 1)
		fail(p)
	}
}

// unwindTurns pops active turn frames back to entry, invalidating each frame's
// Cx serial and clearing Vb.Active so no stale context or half-built tape survives.
func unwindTurns(entry int) {
	for depth > entry {
		t := cur
		t.cx.serial = 0
		t.inInlineEvent = false
		cur = t.parent
		depth--
	}
	Vb.Active = false
}

// runResult produces the final outcome of Run after gk_run returns: printing
// debug stats, re-panicking *PanicError if app code panicked, returning any
// latched *Error or *LibraryError, or translating a non-zero library exit code.
func runResult(rc int32) error {
	report()
	failMu.Lock()
	err := failErr
	failMu.Unlock()
	if p, ok := err.(*PanicError); ok {
		panic(p)
	}
	if err != nil {
		return err
	}
	if rc != 0 {
		return runFailed(rc)
	}
	return nil
}
