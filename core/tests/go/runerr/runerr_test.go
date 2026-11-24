// Package runerr checks gk.Run's contract under go test (docs/GO.md section 5): a mistake in app
// code ends the app and RunHeadless returns it as a *gk.Error with the app's line, and the
// caller's deferred functions run; a panic in app code ends the app and RunHeadless panics again
// with a *gk.PanicError holding the app's value. The app runs in TestMain, on the main goroutine
// (gk needs the main OS thread); the tests read what it left. GK_PROTO picks the case
// (core/tests/go/run-runerr.sh).
package runerr

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

var (
	mode     = os.Getenv("GK_PROTO")
	runErr   error
	deferred bool
	panicked any
	after    int // handler turns that ran after the failure (none may)
	failedAt bool
)

const script = "size 200 100\nframe\nclick 100 50\nframe\nclick 100 50\nframe\nframe\n"

func start(cx *gk.Cx) {
	kept := cx
	var root gk.View
	root = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.Div(s.SizeFull).ID("all").OnClickDo(func(cx *gk.Cx) {
			if failedAt {
				after++
				return
			}
			failedAt = true
			switch mode {
			case "gotest-misuse":
				_ = kept.Window() // the cx of start, kept past its call: a *gk.Error
			case "gotest-panic":
				var m map[string]int
				m["x"] = 1 // the app's own bug
			}
		})
	})
	gk.OpenWindow(cx, "runerr", 200, 100, root)
}

func TestMain(m *testing.M) {
	func() {
		defer func() { deferred = true }()
		defer func() { panicked = recover() }()
		runErr = gk.RunHeadless(script, start)
	}()
	os.Exit(m.Run())
}

func TestMisuse(t *testing.T) {
	if mode != "gotest-misuse" {
		t.Skip("GK_PROTO=gotest-misuse")
	}
	var e *gk.Error
	if !errors.As(runErr, &e) {
		t.Fatalf("RunHeadless returned %v (%T), not a *gk.Error", runErr, runErr)
	}
	fmt.Printf("runerr misuse error=%q line=%v deferred=%v after=%d\n", e.Msg[:39], e.Line != 0, deferred, after)
	if !deferred || after != 0 || e.Line == 0 {
		t.Fatal("the deferred function did not run, a handler ran after the stop, or no line")
	}
}

func TestPanic(t *testing.T) {
	if mode != "gotest-panic" {
		t.Skip("GK_PROTO=gotest-panic")
	}
	p, ok := panicked.(*gk.PanicError)
	if !ok {
		t.Fatalf("RunHeadless panicked with %v (%T), not a *gk.PanicError (returned %v)", panicked, panicked, runErr)
	}
	var re runtime.Error
	fmt.Printf("runerr panic value=%q runtime_error=%v where=%q stack=%v deferred=%v after=%d\n",
		fmt.Sprint(p.Value), errors.As(p, &re), p.Where, len(p.Stack) > 0, deferred, after)
	if !deferred || after != 0 || !errors.As(p, &re) {
		t.Fatal("the deferred function did not run, a handler ran after the panic, or the value is lost")
	}
}
