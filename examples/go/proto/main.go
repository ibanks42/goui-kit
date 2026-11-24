// proto: the protocol tests of docs/PROTOCOLS.md in Go, the port of core/tests/proto (Odin).
// Each mode runs offscreen (the diag library) under a gk-meter script and prints lines that
// gates/go-proto.sh checks, together with the exit code. Nothing here is an example of good app
// code; it pokes at the edges on purpose.
//
//	GK_PROTO=handlers | state | stale | stale-write | wrong-type | lease | write-rule | frames |
//	         view-write | model | post | panic-go | panic-call | panic-later | thread | version |
//	         released-shown | post-memory | handler-memory | rows | stale-cx | stale-element
package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type RowID struct {
	ID   int
	Name string
}

type App struct {
	mode                          string
	root, side                    gk.View
	name                          gk.InputState
	rows                          gk.Rows
	focus                         gk.FocusHandle
	sub                           gk.Subscription
	views, sides                  int
	checked                       bool
	clicks, events, frame, ticks  int
	count                         gk.Model[int]
	seen                          int
	src                           gk.RowSource
	lines, bytes                  int
	rss0, rss1                    int
	described                     map[int]int
	keptCx                        *gk.Cx
	keptEl                        gk.Element
	dlg                           gk.Built[gk.DialogEl]
	list                          gk.ListState
	dlgDescribed, cancels, dcount int
}

// --- handlers of every shape ------------------------------------------------------------------

func (a *App) clicked(cx *gk.Cx, e gk.ClickEvent) {
	a.clicks++
	fmt.Printf("proto click n=%d button=%v count=%d kind=%v x=%.0f y=%.0f\n", a.clicks, e.Button, e.ClickCount, e.Kind, e.Position[0], e.Position[1])
	if a.clicks == 1 {
		yes := gk.NewHandler(func(cx *gk.Cx) bool { return true })
		no := gk.NewHandler(func(cx *gk.Cx) bool { return false })
		fmt.Printf("proto returns true=%d false=%d\n", gk.TestFire(cx, yes), gk.TestFire(cx, no))
		back := gk.TestFireValue(cx, func(cx *gk.Cx, p gk.TestPayload) string { return fmt.Sprintf("%s/%d", p.Text, p.N) }, "payload", 7)
		fmt.Printf("proto value back=%q\n", back)
		// A closure carries its data: the Go form of a handler with a value.
		d := RowID{9, fmt.Sprintf("row-%d", 9)}
		back = gk.TestFireValue(cx, func(cx *gk.Cx, p gk.TestPayload) string {
			return fmt.Sprintf("%s/%d/%d/%s", p.Text, p.N, d.ID, d.Name)
		}, "with-data", 8)
		fmt.Printf("proto value back=%q\n", back)
		back = gk.TestFireNoValue(cx, func(cx *gk.Cx) {}, "x", 1)
		fmt.Printf("proto value default=%q\n", back)
	}
}

func (a *App) toggled(cx *gk.Cx, checked bool) {
	a.checked = checked
	fmt.Printf("proto toggle checked=%v\n", checked)
}

func (a *App) hovered(cx *gk.Cx, inside bool) {
	gk.Quiet(cx)
	fmt.Printf("proto hover inside=%v views=%d\n", inside, a.views)
}

func (a *App) rowClicked(row RowID) func(cx *gk.Cx, e gk.ClickEvent) {
	return func(cx *gk.Cx, e gk.ClickEvent) {
		fmt.Printf("proto row id=%d name=%q button=%v\n", row.ID, row.Name, e.Button)
	}
}

func (a *App) pressed(cx *gk.Cx, e gk.MouseEvent) {
	fmt.Printf("proto mouse button=%v count=%d\n", e.Button, e.ClickCount)
	gk.Focus(cx, a.focus)
}

func (a *App) key(cx *gk.Cx, e gk.KeyEvent) {
	ch, has := e.KeyChar.Get()
	fmt.Printf("proto key key=%q char=%q has_char=%v held=%v mods=%v\n", e.Key, ch, has, e.IsHeld, e.Modifiers)
}

func (a *App) inputEvent(cx *gk.Cx, e gk.InputEvent) {
	a.events++
	switch v := e.(type) {
	case gk.InputEventChange:
		fmt.Printf("proto input change value=%q\n", a.name.Value(cx))
	case gk.InputEventPressEnter:
		fmt.Printf("proto input enter secondary=%v shift=%v\n", v.Secondary, v.Shift)
		if a.mode == "state" {
			// Read your own write: a read flushes the buffered write first.
			a.name.SetValue(cx, "written")
			fmt.Printf("proto read-after-write value=%q\n", a.name.Value(cx))
			gk.Release(cx, a.sub)
			fmt.Println("proto unsubscribed")
		}
	case gk.InputEventFocus:
		fmt.Println("proto input focus")
	case gk.InputEventBlur:
		fmt.Println("proto input blur")
	}
}

func (a *App) boom(cx *gk.Cx, _ gk.ClickEvent) {
	switch a.mode {
	case "panic-go":
		panic("the app's own panic")
	case "panic-call":
		gk.TestPanic(cx, false)
	case "panic-later":
		gk.TestPanic(cx, true)
	case "stale":
		gk.Release(cx, a.name)
		fmt.Printf("proto stale value=%q\n", a.name.Value(cx))
	case "stale-write":
		gk.Release(cx, a.name)
		a.name.SetValue(cx, "to a released handle") // buffered: fails after this handler
		fmt.Println("proto stale-write buffered")
	case "wrong-type":
		fake := gk.TestInputState(gk.Handle(a.rows))
		fmt.Printf("proto wrong value=%q\n", fake.Value(cx))
	case "thread":
		done := make(chan struct{})
		go func() {
			defer close(done)
			gk.Notify(cx, a.root)
		}()
		<-done
	case "lease":
		a.lease(cx)
	case "released-shown":
		gk.Release(cx, a.name)
		fmt.Println("proto released-shown released")
	case "post-memory":
		if a.rss0 == 0 {
			a.rss0 = rssKB()
		}
		postLines(a)
		a.reportMemory(fmt.Sprintf("post-memory queued round=%d", a.clicks+1))
		a.clicks++
	case "stale-cx":
		a.keptCx = cx
		gk.After(cx, 0, func(cx *gk.Cx) { gk.Notify(a.keptCx, a.root) })
	case "stale-element":
		gk.After(cx, 0, func(cx *gk.Cx) { fmt.Println("proto stale-element kept"); gk.Notify(cx, a.root) })
	}
}

// --- row sources ------------------------------------------------------------------------------

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.described[at.Row]++
	row := at.Row
	return gk.HFlex(s.SizeFull).Child(gk.Button(fmt.Sprintf("row-%d", at.Row)).Label(fmt.Sprintf("row %d", at.Row)).
		OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			gk.Quiet(cx)
			fmt.Printf("proto row clicked row=%d\n", row)
		}))
}

func (a *App) sourceEvent(cx *gk.Cx, e gk.SourceTest) {
	fmt.Printf("proto source request id=%d text=%q\n", e.ID, e.Text)
	gk.Answer(cx, a.src, e.ID, "pong "+e.Text)
}

func (a *App) rowsStep(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	switch a.clicks {
	case 1:
		gk.ScrollTo(cx, a.src, 5000)
	case 2:
		gk.ScrollTo(cx, a.src, 0)
	case 3:
		gk.SetCount(cx, a.src, 100)
		gk.TestSourceRequest(cx, a.src, "ping")
	case 4:
		text, ok := gk.TestSourceTake(cx, a.src)
		fmt.Printf("proto answered ok=%v text=%q bound=%d\n", ok, text, gk.TestSourceRows(a.src))
	}
	fmt.Printf("proto rows step=%d bound=%d\n", a.clicks, gk.TestSourceRows(a.src))
}

func (a *App) rowsView(cx *gk.Cx) gk.Element {
	a.views++
	return gk.VFlex(s.SizeFull, s.P4).Bg(theme.Background).TextColor(theme.Foreground).
		Child(cell(gk.Button("first").Label("go").OnClick(a.rowsStep))).
		Child(gk.SourceList("rows", a.src, 24).W(gk.Px(300)).H(gk.Px(240)))
}

// --- handlers of views described often --------------------------------------------------------

func (a *App) memoryFrame(cx *gk.Cx) {
	a.frame++
	gk.Notify(cx, a.root)
	if a.frame == 50 {
		runtime.GC()
		a.rss0 = rssKB()
	}
	if a.frame%200 == 0 {
		a.reportMemory(fmt.Sprintf("handler-memory frame=%d", a.frame))
	}
}

func (a *App) memoryView(cx *gk.Cx) gk.Element {
	a.views++
	col := gk.VFlex(s.SizeFull)
	for i := range 100 {
		// A value that is new on every build, as a clock or a counter in a label would be.
		col.Child(gk.Div().ID(fmt.Sprintf("m-%d", i)).H(gk.Px(2)).
			OnClick(a.rowClicked(RowID{i, fmt.Sprintf("row %d of build %d, made in a view and never clicked", i, a.views)})))
	}
	return col
}

// A handler the component calls from inside its own update: a read reports busy, a write is put
// back and lands after; a queued event runs after the update and reads freely.
func (a *App) lease(cx *gk.Cx) {
	inside := gk.NewHandler(func(cx *gk.Cx) bool {
		_, ok := a.name.TryValue(cx)
		a.name.SetValue(cx, "written inside the update")
		fmt.Printf("proto lease busy=%v\n", !ok)
		return true
	})
	ret := gk.TestFireInUpdate(cx, a.name, inside)
	fmt.Printf("proto lease ret=%d value_after=%q\n", ret, a.name.Value(cx))
	gk.TestFireQueued(cx, a.name, func(cx *gk.Cx, row int) {
		text, ok := a.name.TryValue(cx)
		fmt.Printf("proto queued row=%d ok=%v value=%q\n", row, ok, text)
	}, 42)
	fmt.Println("proto queued not yet")
}

// --- the per-frame handler and the write rule ---------------------------------------------------

func (a *App) writeFrame(cx *gk.Cx) {
	a.frame++
	a.name.SetValue(cx, fmt.Sprintf("frame %d", a.frame))
}

func (a *App) countFrame(cx *gk.Cx) { a.frame++ }

// --- app state --------------------------------------------------------------------------------

func (a *App) bump(cx *gk.Cx, _ gk.ClickEvent) {
	gk.Quiet(cx)
	a.count.Update(cx, func(v *int) { *v++ })
	fmt.Printf("proto model set count=%d observer_saw=%d\n", a.count.Peek(), a.seen)
}

func (a *App) sideView(cx *gk.Cx) gk.Element {
	a.sides++
	fmt.Printf("proto side view run=%d count=%d\n", a.sides, a.count.Get())
	return gk.HFlex().Child(gk.Label(fmt.Sprintf("count %d", a.count.Get())))
}

// --- background posts ---------------------------------------------------------------------------

const posts = 20_000

var sent atomic.Int64

type Tick struct {
	N    int
	Text string
}

func (a *App) tick(t Tick) func(cx *gk.Cx) {
	return func(cx *gk.Cx) {
		a.ticks++
		if t.Text != "tick" {
			fmt.Printf("proto post bad text %q\n", t.Text)
		}
		a.count.Set(cx, t.N)
	}
}

func (a *App) postBatch() {
	items := []Tick{{0, "item 0"}, {1, "item 1"}, {2, "item 2"}}
	nums := []int{1, 2, 3, 4}
	gk.Post(func(cx *gk.Cx) {
		sum := 0
		for _, n := range nums {
			sum += n
		}
		fmt.Printf("proto post batch items=%d first=%q last=%q nums=%d sum=%d\n", len(items), items[0].Text, items[len(items)-1].Text, len(nums), sum)
	})
}

func (a *App) poster() {
	for i := range posts {
		gk.Post(a.tick(Tick{i + 1, "ti" + "ck"}))
		sent.Add(1)
		if i%1000 == 999 {
			time.Sleep(25 * time.Millisecond)
		}
	}
}

// --- memory: posted lines are freed ------------------------------------------------------------

const lines = 300_000

func (a *App) line(text string) func(cx *gk.Cx) {
	return func(cx *gk.Cx) {
		a.lines++
		a.bytes += len(text)
		if a.lines%lines == 0 {
			a.reportMemory(fmt.Sprintf("post-memory round=%d", a.lines/lines))
		}
		if a.lines == lines {
			runtime.GC()
			a.rss1 = rssKB()
		} else if a.lines == 2*lines {
			runtime.GC()
			fmt.Printf("proto post-memory second round grew_mb=%d\n", (rssKB()-a.rss1)/1024)
		}
	}
}

var round int

func postLines(a *App) {
	round++
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range lines {
			gk.Post(a.line(fmt.Sprintf("round %d, line %d of a log that a worker goroutine streams to the UI, padded to a hundred bytes", round, i)))
		}
	}()
	<-done
}

func rssKB() int {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return -1
	}
	f := strings.Fields(string(data))
	pages, _ := strconv.Atoi(f[1])
	return pages * 4
}

func (a *App) reportMemory(what string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Printf("proto %s rss_mb=%d growth_mb=%d heap_kb=%d\n", what, rssKB()/1024, (rssKB()-a.rss0)/1024, ms.HeapAlloc/1024)
}

// --- a list over a row source (PROTOCOLS.md 9.5; Odin: core/tests/run-rows.sh list) -------------

func (a *App) listRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	a.described[at.Row]++
	return gk.ListItem(gk.Key("item", at.Row)).Child(gk.Label(fmt.Sprintf("item %d.%d", at.Section, at.Row)))
}

func (a *App) listEvent(cx *gk.Cx, e gk.ListDelegateEvent) {
	a.events++
	ix, has := e.Index.Get()
	fmt.Printf("proto list %v has=%v section=%d row=%d secondary=%v id=%d query=%q\n", e.Kind, has, ix.Section, ix.Row, e.Secondary, e.ID, e.Query)
	if e.Kind == gk.ListConfirm {
		sel, ok := a.list.SelectedIndex(cx).Get()
		fmt.Printf("proto list-state read selected_index ok=%v row=%d\n", ok, sel.Row)
	}
}

func (a *App) listView(cx *gk.Cx) gk.Element {
	a.views++
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).Child(gk.Button("step").Label("step").OnClick(a.boom))).
		Child(gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gk.List(a.list, s.SizeFull)))
}

// --- a dialog over a view-built closure (PROTOCOLS.md 11.6) --------------------------------------

func (a *App) confirmDialog(cx *gk.Cx) gk.DialogEl {
	a.dlgDescribed++
	fmt.Printf("proto dialog described n=%d count=%d\n", a.dlgDescribed, a.dcount)
	seen := a.dcount // the count of this description: which tape the kit used
	return gk.Dialog().
		Title(gk.Label("Delete?")).
		Child(gk.Label(fmt.Sprintf("count %d", a.dcount))).
		OnCancel(func(cx *gk.Cx, _ gk.ClickEvent) bool {
			a.cancels++
			fmt.Printf("proto dialog cancel n=%d seen=%d count=%d\n", a.cancels, seen, a.dcount)
			return true // closes it
		})
}

func (a *App) dialogSteps(cx *gk.Cx) {
	gk.After(cx, 10*time.Millisecond, func(cx *gk.Cx) {
		gk.OpenDialog(cx, a.dlg)
		fmt.Println("proto dialog opened")
	})
	gk.After(cx, 30*time.Millisecond, func(cx *gk.Cx) {
		fmt.Printf("proto dialog active=%v\n", gk.HasActiveDialog(cx))
		a.dcount++
		gk.Notify(cx, a.dlg.View) // described again while open
	})
	gk.After(cx, 50*time.Millisecond, func(cx *gk.Cx) {
		fmt.Printf("proto dialog closed active=%v\n", gk.HasActiveDialog(cx))
		gk.OpenDialog(cx, a.dlg) // the same view, opened again
	})
	gk.After(cx, 70*time.Millisecond, func(cx *gk.Cx) { gk.CloseDialog(cx) })
	gk.After(cx, 90*time.Millisecond, func(cx *gk.Cx) {
		fmt.Printf("proto dialog done active=%v described=%d cancels=%d count=%d\n", gk.HasActiveDialog(cx), a.dlgDescribed, a.cancels, a.dcount)
	})
}

// --- the view -----------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	a.name = gk.NewInputState(cx).DefaultValue("abc").Build()
	a.rows = gk.NewRows(cx)
	a.focus = gk.NewFocusHandle(cx)
	switch a.mode {
	case "state":
		a.sub = a.name.OnInputEvent(cx, a.inputEvent)
	case "write-rule":
		gk.OnFrame(cx, a.writeFrame)
	case "frames":
		gk.OnFrame(cx, a.countFrame)
	case "model", "post":
		a.count.Observe(cx, func(cx *gk.Cx, v int) { a.seen = v })
	case "rows":
		a.src = gk.NewRowSource(cx, 10_000, a.itemRow, gk.TestSourceEvents(a.sourceEvent), gk.Initial(20))
	case "handler-memory":
		gk.OnFrame(cx, a.memoryFrame)
	case "list":
		src := gk.NewListSource(cx, func() int { return 10_000 }, a.listRow, gk.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gk.NewListState(cx, src).Build()
		gk.SetDelegateData(cx, a.list, gk.DelegateData{Threshold: 20, PlaceholderHeight: 32})
	case "dialog":
		a.dlg = gk.NewBuilt(cx, a.confirmDialog)
		a.dialogSteps(cx)
	}
}

func cell(child gk.Element) gk.Element {
	return gk.Div().W(gk.Px(100)).H(gk.Px(40)).Child(child)
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	a.views++
	row := gk.Div(s.Flex, s.FlexRow)
	first := gk.Button("first").Label("go")
	switch a.mode {
	case "handlers":
		first.OnClick(a.clicked)
	case "model":
		first.OnClick(a.bump)
	default:
		first.OnClick(a.boom)
	}
	if a.mode == "stale-element" {
		if a.keptEl != nil {
			row.Child(a.keptEl) // an element of an earlier build: stops
		}
		a.keptEl = gk.Label("kept")
	}
	row.Child(cell(first))
	row.Child(cell(gk.Switch("sw").Checked(a.checked).OnClick(a.toggled)))
	row.Child(cell(gk.Div().ID("hover").W(gk.Px(80)).H(gk.Px(30)).Bg(gk.HSLA(0.3, 0.5, 0.4, 1)).
		OnHover(a.hovered).
		OnClick(a.rowClicked(RowID{5, fmt.Sprintf("row %d", 5)}))))
	row.Child(cell(gk.Div().W(gk.Px(80)).H(gk.Px(30)).Bg(gk.HSLA(0.6, 0.5, 0.4, 1)).
		TrackFocus(a.focus).
		OnMouseDown(gk.MouseButtonLeft, a.pressed).
		OnKeyDown(a.key)))
	value := ""
	switch a.mode {
	case "state", "write-rule":
		value = a.name.Value(cx) // a dependency: described again when the input changes
		fmt.Printf("proto echo view=%d frame=%d value=%q\n", a.views, a.frame, value)
	case "view-write":
		a.name.SetValue(cx, "written by a view")
	}
	col := gk.VFlex(s.SizeFull, s.P4).Bg(theme.Background).TextColor(theme.Foreground).
		Child(row).
		Child(gk.Div().W(gk.Px(200)).H(gk.Px(40)).Child(gk.Input(a.name))).
		Child(gk.Label("echo " + value))
	if a.mode == "model" || a.mode == "post" {
		col.ChildView(a.side)
	}
	return col
}

func (a *App) start(cx *gk.Cx) {
	a.side = gk.NewView(nil, a.sideView)
	switch a.mode {
	case "rows":
		a.root = gk.NewView(a.init, a.rowsView)
	case "handler-memory":
		a.root = gk.NewView(a.init, a.memoryView)
	case "list":
		a.root = gk.NewView(a.init, a.listView)
		gk.OpenWindow(cx, "proto", 480, 320, a.root)
		return
	default:
		a.root = gk.NewView(a.init, a.rootView)
	}
	gk.OpenWindow(cx, "proto", 480, 300, a.root)
}

// Positions (window coordinates): the four cells are 100 wide from x = 16, at y = 16; the input
// is below them.
const (
	first  = "click 36 32\n"
	swtch  = "click 126 26\n"
	hover  = "250 30\n"
	keys   = "click 350 30\n"
	input  = "click 60 76\n"
	rowAt0 = "click 40 68\n"
)

// version: the library's half of the version check, called directly with a wrong gk_app.
func version() {
	try := func(what string, hashXor uint64, major, minor uint16, size uint32, noCallback bool) {
		rc, msg := gk.TestRunRaw(hashXor, major, minor, size, noCallback)
		fmt.Printf("proto version %s rc=%d message=%q\n", what, rc, msg)
	}
	try("hash", 1, 0, 0, 0, false)
	try("major", 0, 1, 0, 0, false)
	try("newer-minor", 0, 0, 1, 0, false)
	try("size", 0, 0, 0, 8, false)
	try("callback", 0, 0, 0, 0, true)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO"), described: map[int]int{}}
	if kindsMain(a.mode) || runBridges(a.mode) { // kinds.go: the generator's descriptor kinds; bridges.go: the Go ports of Odin's hand-written bridges
		return
	}
	if os.Getenv("GK_PROTO_STALE_HASH") != "" {
		gk.TestExpectHash(1)
	}
	script := "size 480 300\nframe\n"
	switch a.mode {
	case "version":
		version()
		return
	case "handlers":
		script += first + swtch + swtch + "move " + hover + "move 250 200\n" + "click " + hover + keys + "type k\n" + "key shift-enter\n" + "frame\nstat\n"
	case "state":
		script += input + "type xy\n" + "key enter\n" + "type z\n" + "click 250 200\n" + "frame\nstat\n"
	case "write-rule", "frames":
		script += "frame\nframe\nframe\nstat\n"
	case "model":
		script += first + first + "frame\nstat\n"
	case "post-memory":
		script += first + "sleep 200\nframe\nsleep 200\nframe\n" + first + "sleep 200\nframe\nsleep 200\nframe\nstat\n"
	case "handler-memory":
		script += strings.Repeat("frame\n", 600) + "stat\n"
	case "rows":
		script += first + "frame\nframe\nframe\n" + rowAt0 + "frame\n" + first + "frame\nframe\n" + first + "frame\nframe\n" + first + "frame\nstat\n"
	case "list":
		// a click on row 0 (select, confirm), down, a right click, escape, a far scroll (one
		// placeholder frame), back to the top (rows kept, nothing described again).
		script = "size 480 320\nframe\nstat\nclick 60 74\nkey down\nrclick 60 140\nkey escape\nscroll 160 200 -200000\nframe\nstat\nscroll 160 200 400000\nframe\n"
	case "dialog":
		// open at 10; bumped at 30 while open; escape (on_cancel of the second description);
		// reopened at 50 on the same view; closed by the app at 70; done at 90
		step := "frame\nframe\n"
		script = "size 640 480\nframe\n" + "advance 10\n" + step + "advance 20\n" + step + "key escape\n" + step +
			"advance 20\n" + step + "advance 20\n" + step + "advance 20\n" + step + "stat\n"
	case "post":
		gk.Post(a.tick(Tick{0, "tick"})) // before the library's loop exists
		sent.Add(1)
		go a.poster()
		go a.postBatch()
		script += strings.Repeat("sleep 100\nframe\n", 8) + "sleep 300\nframe\nframe\nstat\n"
	default:
		script += first + "frame\nframe\nstat\n"
	}
	gk.RunHeadless(script, a.start)
	fmt.Printf("proto done mode=%s views=%d sides=%d clicks=%d events=%d frame=%d count=%d seen=%d ticks=%d sent=%d\n",
		a.mode, a.views, a.sides, a.clicks, a.events, a.frame, a.count.Peek(), a.seen, a.ticks, sent.Load())
	switch a.mode {
	case "post-memory":
		runtime.GC()
		a.reportMemory(fmt.Sprintf("post-memory done lines=%d", a.lines))
	case "handler-memory":
		runtime.GC()
		a.reportMemory(fmt.Sprintf("handler-memory done frames=%d", a.frame))
	case "rows", "list":
		again := 0
		for _, n := range a.described {
			if n > 1 {
				again++
			}
		}
		fmt.Printf("proto rows described=%d again=%d\n", len(a.described), again)
	}
}
