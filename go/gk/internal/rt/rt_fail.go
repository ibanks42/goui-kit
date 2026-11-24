package rt

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

// How the app stops early (docs/GO.md section 5). Nothing in the package exits the process:
//
//   - a mistake in app code (a *Error: a kept cx, a released handle, a write in a view, a UI call
//     from another goroutine) is recorded, the library is asked to quit, no app code runs in the
//     turns that come before it has, and gk.Run returns the *Error;
//   - a panic in app code (a handler, a view, init, a posted function, gk.Go's work function) is
//     recorded the same way and gk.Run panics again, after the library has shut down, with a
//     *PanicError that holds the app's value and the stack where it panicked;
//   - a panic inside the library is recorded (the PANIC turn, or ErrPoisoned from a call) and
//     gk.Run returns a *LibraryError.
//
// Deferred functions of main run, and an app (or a go test) may recover or report as it likes.

// PanicError is a panic in app code, carried out of gk.Run once the library has shut down:
// gk.Run panics with it. Value is what the app panicked with (Unwrap gives it when it is an error,
// so errors.As finds a runtime.Error); Stack is the goroutine's stack where it panicked.
type PanicError struct {
	Value any
	Where string // "app code", or "the work function of the gk.Go at file:line"
	File  string // the app's line the panic came from
	Line  int
	Func  string
	Stack []byte
}

func (p *PanicError) Error() string {
	s := fmt.Sprintf("gk: panic in %s: %v", p.Where, p.Value)
	if p.Line != 0 {
		s += fmt.Sprintf("\n    at %s:%d (%s)", p.File, p.Line, p.Func)
	}
	return s + "\n    gk.Run ended the app and panics again with this value (a *gk.PanicError; Value is the app's). Where it panicked:\n" + string(p.Stack)
}

// Unwrap is the app's panic value when it is an error.
func (p *PanicError) Unwrap() error {
	if e, ok := p.Value.(error); ok {
		return e
	}
	return nil
}

// LibraryError is a panic inside the library (GPUI-kit or the binding's runtime), not in the
// app's Go code: gk.Run returns it once the library has shut down. File and Line are the app's
// call that was running, when the panic came inside one.
type LibraryError struct {
	Msg  string
	File string
	Line int
	Func string
}

func (e *LibraryError) Error() string {
	s := "gk: the library panicked: " + e.Msg
	if e.Line != 0 {
		s += fmt.Sprintf("\n    in the call at %s:%d (%s)", e.File, e.Line, e.Func)
	}
	return s + "\n    This is a fault inside the library (GPUI-kit or the binding's runtime), not in the app's Go code: the app cannot go on. Run it again with GK_DEBUG=1, which sends every change at once so the call above is the one that failed, and report the message with that line."
}

var (
	failMu   sync.Mutex
	failErr  error // the first failure: *Error, *PanicError or *LibraryError
	failed   atomic.Bool
	quitSent bool // the library was asked to quit (UI thread only)
)

// abortTurn unwinds app code above a turn that failed: a nested callback failed while the outer
// app code was inside a call. The boundary of the outer callback drops it.
type abortTurn struct{}

// fail records the first failure of the run; later ones are dropped (the app is stopping).
func fail(err error) {
	failMu.Lock()
	if failErr == nil {
		failErr = err
		failed.Store(true)
	}
	failMu.Unlock()
}

// Failed reports whether the app is stopping after a failure (tests).
func Failed() bool { return failed.Load() }

// askQuit asks the library to end gk_run. UI thread, inside a callback; a poisoned library
// still takes QUIT. Where no call can be made (the PANIC turn has no call scope) it is asked
// again at the next turn.
func askQuit() {
	if quitSent {
		return
	}
	if rc, _ := call(*NewReq(CQuit)); rc == 0 {
		quitSent = true
	}
}

// failOffThread records a failure found on a goroutine that is not the UI thread: there is no
// boundary below to unwind to, so the UI thread is woken (its next turn asks the library to quit)
// and this goroutine ends.
func failOffThread(err error) {
	fail(err)
	postWake()
	runtime.Goexit()
}

// recovered turns what a boundary recovered into the run's failure.
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

// unwindTurns restores the turn state of a callback that ended in a panic: the turns it began are
// ended, their cx stale.
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

// runResult is what gk.Run gives back once gk_run has returned: the recorded failure (a panic
// in app code panics again here), else the library's status.
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
