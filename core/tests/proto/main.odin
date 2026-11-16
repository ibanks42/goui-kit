// proto: the focused tests of the protocols in docs/PROTOCOLS.md, one mode per protocol.
// Each mode runs offscreen (the diag library) under a gk-meter script and prints lines that
// core/tests/run.sh checks, together with the exit code. Nothing here is an example of
// good app code; it pokes at the edges on purpose.
//
//   GK_PROTO=handlers | shape | state-type | state | stale | stale-write | wrong-type | lease | write-rule |
//            frames | view-write | model | post | panic-odin | panic-call | panic-later | thread | version |
//            released-shown | post-memory | handler-memory | rows
package proto

import "core:fmt"
import "core:os"
import "core:strconv"
import "core:strings"
import "core:sync"
import "core:thread"
import "core:time"
import gk "gpuikit:gk"

Row_Id :: struct {
	id:   int,
	name: string,
}

App :: struct {
	mode:    string,
	root:    gk.View,
	side:    gk.View,
	name:    gk.Input_State,
	rows:    gk.Rows,
	focus:   gk.Focus_Handle,
	sub:     gk.Subscription,
	views:   int, // runs of root_view
	sides:   int, // runs of side_view
	checked: bool,
	clicks:  int,
	events:  int,
	frame:   int,
	count:   gk.Model(int),
	seen:    int, // what the model's observer saw last
	ticks:   int,
	cx:      ^gk.Cx,
	src:     gk.Row_Source,
	lines:   int, // posted lines received (post-memory)
	bytes:   int,
	rss0:    int,
	rss1:    int,
	described: map[int]int, // row descriptions per row (rows)
}

Other :: struct {
	x: int,
}

// --- handlers of every shape ------------------------------------------------------------------

clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event) {
	a.clicks += 1
	fmt.printf("proto click n=%d button=%v count=%d kind=%v x=%.0f y=%.0f\n", a.clicks, e.button, e.click_count, e.kind, e.position.x, e.position.y)
	if a.clicks == 1 {
		// A value-returning handler: the library gets true and false back.
		yes := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {return true})
		no := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {return false})
		fmt.printf("proto returns true=%d false=%d\n", gk.test_fire(cx, yes), gk.test_fire(cx, no))
		// Two payload arguments and a string coming back.
		back := gk.test_fire_value(cx, gk.on(proc(a: ^App, cx: ^gk.Cx, p: gk.Test_Payload) -> string {
			return fmt.tprintf("%s/%d", p.text, p.n)
		}), "payload", 7)
		fmt.printf("proto value back=%q\n", back)
		// The same with handler data owned by the binding.
		back = gk.test_fire_value(cx, gk.on(proc(a: ^App, cx: ^gk.Cx, p: gk.Test_Payload, d: Row_Id) -> string {
			return fmt.tprintf("%s/%d/%d/%s", p.text, p.n, d.id, d.name)
		}, Row_Id{9, fmt.tprintf("row-%d", 9)}), "with-data", 8)
		fmt.printf("proto value back=%q\n", back)
		// A handler without a return value where one is expected gives the default.
		back = gk.test_fire_value(cx, gk.on(proc(a: ^App, cx: ^gk.Cx) {}), "x", 1)
		fmt.printf("proto value default=%q\n", back)
	}
}

toggled :: proc(a: ^App, cx: ^gk.Cx, checked: bool) {
	a.checked = checked
	fmt.printf("proto toggle checked=%v\n", checked)
}

hovered :: proc(a: ^App, cx: ^gk.Cx, inside: bool) {
	fmt.printf("proto hover inside=%v views=%d\n", inside, a.views)
}

row_clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event, row: Row_Id) {
	fmt.printf("proto row id=%d name=%q button=%v\n", row.id, row.name, e.button)
}

pressed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Mouse_Event) {
	fmt.printf("proto mouse button=%v count=%d\n", e.button, e.click_count)
	gk.focus(cx, a.focus)
}

key :: proc(a: ^App, cx: ^gk.Cx, e: gk.Key_Event) {
	ch, has := e.key_char.?
	fmt.printf("proto key key=%q char=%q has_char=%v held=%v mods=%v\n", e.key, ch, has, e.is_held, e.modifiers)
}

input_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	a.events += 1
	switch v in e {
	case gk.Input_Event_Change:
		fmt.printf("proto input change value=%q\n", a.name->value(cx))
	case gk.Input_Event_Press_Enter:
		fmt.printf("proto input enter secondary=%v shift=%v\n", v.secondary, v.shift)
		if a.mode == "state" {
			// Read your own write: a read flushes the buffered write first.
			a.name->set_value(cx, "written")
			fmt.printf("proto read-after-write value=%q\n", a.name->value(cx))
			gk.release(cx, a.sub)
			fmt.println("proto unsubscribed")
		}
	case gk.Input_Event_Focus:
		fmt.println("proto input focus")
	case gk.Input_Event_Blur:
		fmt.println("proto input blur")
	}
}

bad_event :: proc(a: ^App, cx: ^gk.Cx, wrong: bool) {}
bad_state :: proc(o: ^Other, cx: ^gk.Cx) {}
boom :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "panic-odin":
		panic("the app's own panic")
	case "panic-call":
		gk.test_panic(cx, false)
	case "panic-later":
		gk.test_panic(cx, true)
	case "stale":
		gk.release(cx, a.name)
		fmt.printf("proto stale value=%q\n", a.name->value(cx))
	case "stale-write":
		gk.release(cx, a.name)
		a.name->set_value(cx, "to a released handle") // buffered: fails after this handler
		fmt.println("proto stale-write buffered")
	case "wrong-type":
		fake := gk.Input_State{{a.rows.h}, a.name.vt}
		fmt.printf("proto wrong value=%q\n", fake->value(cx))
	case "thread":
		a.cx = cx
		t := thread.create_and_start_with_poly_data(a, proc(a: ^App) {
			gk.notify(a.cx, a.root)
		})
		thread.join(t)
	case "lease":
		lease(a, cx)
	case "released-shown":
		// The view still shows the input; it is described again when this handler returns.
		gk.release(cx, a.name)
		fmt.println("proto released-shown released")
	case "post-memory":
		if a.rss0 == 0 {
			a.rss0 = rss_kb()
		}
		post_lines(a)
		report_memory(a, fmt.tprintf("post-memory queued round=%d", a.clicks + 1))
		a.clicks += 1
	}
}

// --- row sources ------------------------------------------------------------------------------

item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.described[at.row] += 1
	return gk.h_flex(.Size_Full)->child(gk.button(fmt.tprintf("row-%d", at.row))->label(fmt.tprintf("row %d", at.row))->
		on_click(gk.quiet(gk.on(row_click, at.row))))
}

row_click :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	fmt.printf("proto row clicked row=%d\n", row)
}

source_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Source_Test) {
	fmt.printf("proto source request id=%d text=%q\n", e.id, e.text)
	gk.answer(cx, a.src, e.id, fmt.tprintf("pong %s", e.text))
}

rows_step :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.clicks {
	case 1:
		gk.scroll_to(cx, a.src, 5000)
	case 2:
		gk.scroll_to(cx, a.src, 0)
	case 3:
		gk.set_count(cx, a.src, 100)
		gk.test_source_request(cx, a.src, "ping")
	case 4:
		text, ok := gk.test_source_take(cx, a.src)
		fmt.printf("proto answered ok=%v text=%q bound=%d\n", ok, text, gk.test_source_rows(a.src))
	}
	fmt.printf("proto rows step=%d bound=%d\n", a.clicks, gk.test_source_rows(a.src))
}

rows_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.views += 1
	return gk.v_flex(.Size_Full, .P_4)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(cell(gk.button("first")->label("go")->on_click(gk.on(rows_step))))->
		child(gk.source_list("rows", a.src, 24)->w(gk.px(300))->h(gk.px(240)))
}

// --- handler values of views described often --------------------------------------------------

memory_frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.frame += 1
	gk.notify(cx, a.root)
	if a.frame == 50 {
		a.rss0 = rss_kb()
	}
	if a.frame % 200 == 0 {
		report_memory(a, fmt.tprintf("handler-memory frame=%d", a.frame))
	}
}

memory_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.views += 1
	col := gk.v_flex(.Size_Full)
	for i in 0 ..< 100 {
		// A value that is new on every build, as a clock or a counter in a label would be.
		col->child(gk.div()->id(fmt.tprintf("m-%d", i))->h(gk.px(2))->
			on_click(gk.on(row_clicked, Row_Id{i, fmt.tprintf("row %d of build %d, made in a view and never clicked", i, a.views)})))
	}
	return col
}

// A handler the component calls from inside its own update: a read reports busy, a write is
// put back and lands after; a queued event runs after the update and reads freely.
lease :: proc(a: ^App, cx: ^gk.Cx) {
	inside := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {
		_, ok := a.name->try_value(cx)
		a.name->set_value(cx, "written inside the update")
		fmt.printf("proto lease busy=%v\n", !ok)
		return true
	})
	ret := gk.test_fire_in_update(cx, a.name, inside)
	fmt.printf("proto lease ret=%d value_after=%q\n", ret, a.name->value(cx))
	gk.test_fire_queued(cx, a.name, gk.on(proc(a: ^App, cx: ^gk.Cx, row: int) {
		text, ok := a.name->try_value(cx)
		fmt.printf("proto queued row=%d ok=%v value=%q\n", row, ok, text)
	}), 42)
	fmt.println("proto queued not yet")
}

// --- the per-frame handler and the write rule ---------------------------------------------------

frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.frame += 1
	a.name->set_value(cx, fmt.tprintf("frame %d", a.frame))
}

// A frame handler that touches nothing the views show: it must still run before every frame.
count_frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.frame += 1
}

// --- app state --------------------------------------------------------------------------------

bump :: proc(a: ^App, cx: ^gk.Cx) {
	gk.update(cx, &a.count, proc(v: ^int) {v^ += 1})
	fmt.printf("proto model set count=%d observer_saw=%d\n", gk.peek(&a.count), a.seen)
}

side_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.sides += 1
	fmt.printf("proto side view run=%d count=%d\n", a.sides, gk.get(&a.count))
	return gk.h_flex()->child(gk.label(fmt.tprintf("count %d", gk.get(&a.count))))
}

// --- background posts ---------------------------------------------------------------------------

POSTS :: 20_000
sent: i64

Tick :: struct {
	n:    int,
	text: string,
}

tick :: proc(a: ^App, cx: ^gk.Cx, t: Tick) {
	a.ticks += 1
	if t.text != "tick" {
		fmt.printf("proto post bad text %q\n", t.text)
	}
	gk.set(cx, &a.count, t.n)
	delete(t.text) // the message is the procedure's
}

// A thread's list of results in one post: a slice of records with strings and a dynamic array,
// copied when posted (the thread frees its own) and the procedure's when it runs.
Batch :: struct {
	items: []Tick,
	nums:  [dynamic]int,
}

batch :: proc(a: ^App, cx: ^gk.Cx, b: Batch) {
	sum := 0
	for n in b.nums {sum += n}
	fmt.printf("proto post batch items=%d first=%q last=%q nums=%d sum=%d\n", len(b.items), b.items[0].text,
		b.items[len(b.items) - 1].text, len(b.nums), sum)
	for t in b.items {delete(t.text)} // the message is the procedure's
	delete(b.items)
	delete(b.nums)
}

post_batch :: proc() {
	items := make([]Tick, 3)
	for &t, i in items {t = {i, fmt.aprintf("item %d", i)}}
	nums: [dynamic]int
	append(&nums, 1, 2, 3, 4)
	gk.post(batch, Batch{items, nums})
	for t in items {delete(t.text)} // the post copied them
	delete(items)
	delete(nums)
}

// --- memory: posted lines and handler values are freed -------------------------------------------

LINES :: 300_000

Line :: struct {
	n:    int,
	text: string,
}

line :: proc(a: ^App, cx: ^gk.Cx, l: Line) {
	a.lines += 1
	a.bytes += len(l.text)
	delete(l.text) // the procedure owns its message
	if a.lines % LINES == 0 {
		report_memory(a, fmt.tprintf("post-memory round=%d", a.lines / LINES))
	}
	// The second round reuses what the first freed: the process does not grow again.
	if a.lines == LINES {
		a.rss1 = rss_kb()
	} else if a.lines == 2 * LINES {
		fmt.printf("proto post-memory second round grew_mb=%d held_kb=%d\n", (rss_kb() - a.rss1) / 1024, gk.test_owned_bytes() / 1024)
	}
}

// rss_kb is this process's resident memory (Linux; the runtime's tests are Linux-only).
rss_kb :: proc() -> int {
	data, err := os.read_entire_file("/proc/self/statm", context.temp_allocator)
	if err != nil {
		return -1
	}
	fields := strings.fields(string(data), context.temp_allocator)
	pages, _ := strconv.parse_int(fields[1])
	return pages * 4
}

report_memory :: proc(a: ^App, what: string) {
	held := gk.test_owned_bytes()
	fmt.printf("proto %s rss_mb=%d growth_mb=%d held_kb=%d\n", what, rss_kb() / 1024, (rss_kb() - a.rss0) / 1024, held / 1024)
}

// post_lines posts LINES distinct lines of about 100 bytes from another thread while the UI
// thread waits for it, so all of them are queued at once (and the binding warns past 16 MB).
// round makes the lines of each round distinct from the last's.
round: int

post_lines :: proc(a: ^App) {
	round += 1
	t := thread.create_and_start(proc() {
		for i in 0 ..< LINES {
			gk.post(line, Line{i, fmt.tprintf("round %d, line %d of a log that a worker thread streams to the UI, padded to a hundred bytes", round, i)})
			free_all(context.temp_allocator)
		}
	})
	thread.join(t)
}

poster :: proc() {
	for i in 0 ..< POSTS {
		gk.post(tick, Tick{i + 1, fmt.tprintf("ti%s", "ck")})
		sync.atomic_add(&sent, 1)
		if i % 1000 == 999 {
			time.sleep(25 * time.Millisecond) // about half a second in all, across several frames
		}
	}
}

// --- the view -----------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->default_value("abc")->build()
	a.rows = gk.rows(cx)
	a.focus = gk.focus_handle(cx)
	switch a.mode {
	case "state":
		a.sub = a.name->on_input_event(cx, gk.on(input_event))
	case "write-rule":
		gk.on_frame(cx, frame)
	case "frames":
		gk.on_frame(cx, count_frame)
	case "model", "post":
		gk.observe(cx, &a.count, a, proc(a: ^App, cx: ^gk.Cx, v: int) {a.seen = v})
	case "rows":
		a.src = gk.row_source(cx, a, 10_000, item_row, gk.on(source_event), gk.SHAPE_SOURCE_TEST, initial = 20)
	case "handler-memory":
		gk.on_frame(cx, memory_frame)
	}
}

cell :: proc(child: gk.Element) -> gk.Element {
	return gk.div()->w(gk.px(100))->h(gk.px(40))->child(child)
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.views += 1
	row := gk.div(.Flex, .Flex_Row)
	first := gk.button("first")->label("go")
	switch a.mode {
	case "shape":
		first->on_click(gk.on(bad_event))
	case "state-type":
		first->on_click(gk.on(bad_state))
	case "handlers":
		first->on_click(gk.on(clicked))
	case "model":
		first->on_click(gk.quiet(gk.on(bump)))
	case:
		first->on_click(gk.on(boom))
	}
	row->child(cell(first))
	row->child(cell(gk.switch_("sw")->checked(a.checked)->on_click(gk.on(toggled))))
	row->child(cell(gk.div()->id("hover")->w(gk.px(80))->h(gk.px(30))->bg(gk.hsla(0.3, 0.5, 0.4, 1))->
		on_hover(gk.quiet(gk.on(hovered)))->
		on_click(gk.on(row_clicked, Row_Id{5, fmt.tprintf("row %d", 5)}))))
	row->child(cell(gk.div()->w(gk.px(80))->h(gk.px(30))->bg(gk.hsla(0.6, 0.5, 0.4, 1))->
		track_focus(a.focus)->
		on_mouse_down(gk.Mouse_Button_Left{}, gk.on(pressed))->
		on_key_down(gk.on(key))))
	value: gk.Temp_String
	switch a.mode {
	case "state", "write-rule":
		value = a.name->value(cx) // a dependency: described again when the input changes
		fmt.printf("proto echo view=%d frame=%d value=%q\n", a.views, a.frame, value)
	case "view-write":
		a.name->set_value(cx, "written by a view")
	}
	col := gk.v_flex(.Size_Full, .P_4)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(row)->
		child(gk.div()->w(gk.px(200))->h(gk.px(40))->child(gk.input(a.name)))->
		child(gk.label(fmt.tprintf("echo %s", value)))
	if a.mode == "model" || a.mode == "post" {
		col->child_view(a.side)
	}
	return col
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.side = gk.view(a, nil, side_view)
	switch a.mode {
	case "rows":
		a.root = gk.view(a, init, rows_view)
	case "handler-memory":
		a.root = gk.view(a, init, memory_view)
	case:
		a.root = gk.view(a, init, root_view)
	}
	gk.open_window(cx, "proto", {480, 300}, a.root)
}

// Positions (window coordinates): the four cells are 100 wide from x = 16, at y = 16; the
// input is below them.
FIRST :: "click 36 32\n"
SWITCH :: "click 126 26\n"
HOVER :: "250 30\n"
KEYS :: "click 350 30\n"
INPUT :: "click 60 76\n"
ROW :: "click 40 68\n" // the first row of the list in mode rows

// version: the library's half of the version check, called directly with a wrong gk_app.
version :: proc() {
	nothing :: proc "c" (user: rawptr, cb: ^gk.Gk_Cb, kind: u32, req: [^]u8, n: uint) -> i32 {return 0}
	try :: proc(what: string, app: gk.Gk_App) {
		app := app
		rc := gk.gk_run(&app)
		fmt.printf("proto version %s rc=%d message=%q\n", what, rc, gk.last_error())
	}
	good := gk.Gk_App{size_of(gk.Gk_App), gk.ABI_MAJOR, gk.ABI_MINOR, gk.API_HASH, nothing, nil, 0}
	bad := good
	bad.api_hash ~= 1
	try("hash", bad)
	bad = good
	bad.abi_major += 1
	try("major", bad)
	bad = good
	bad.abi_minor += 1
	try("newer-minor", bad)
	bad = good
	bad.size -= 8
	try("size", bad)
	bad = good
	bad.callback = nil
	try("callback", bad)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 480 300\nframe\n"
	switch app.mode {
	case "version":
		version()
		return
	case "handlers":
		script = fmt.aprint(
			script, FIRST, SWITCH, SWITCH, "move ", HOVER, "move 250 200\n", "click ", HOVER, KEYS, "type k\n", "key shift-enter\n", "frame\nstat\n",
			sep = "",
		)
	case "state":
		script = fmt.aprint(script, INPUT, "type xy\n", "key enter\n", "type z\n", "click 250 200\n", "frame\nstat\n", sep = "")
	case "write-rule", "frames":
		script = fmt.aprint(script, "frame\nframe\nframe\nstat\n", sep = "")
	case "model":
		script = fmt.aprint(script, FIRST, FIRST, "frame\nstat\n", sep = "")
	case "post-memory":
		script = fmt.aprint(script, FIRST, "sleep 200\nframe\nsleep 200\nframe\n", FIRST, "sleep 200\nframe\nsleep 200\nframe\nstat\n", sep = "")
	case "handler-memory":
		for _ in 0 ..< 600 {
			script = fmt.aprint(script, "frame\n", sep = "")
		}
		script = fmt.aprint(script, "stat\n", sep = "")
	case "rows":
		// frame 1: rows 0.. from the source's first rows; a jump to 5000 shows placeholders for
		// one frame; a click on the first row; back to 0 (kept); count 100 and a request; the answer.
		script = fmt.aprint(script, FIRST, "frame\nframe\nframe\n", ROW, "frame\n", FIRST, "frame\nframe\n", FIRST, "frame\nframe\n", FIRST, "frame\nstat\n", sep = "")
	case "post":
		gk.post(tick, Tick{0, "tick"}) // before the library's loop exists
		sync.atomic_add(&sent, 1)
		thread.create_and_start(poster)
		thread.create_and_start(post_batch)
		for _ in 0 ..< 8 {
			script = fmt.aprint(script, "sleep 100\nframe\n", sep = "")
		}
		script = fmt.aprint(script, "sleep 300\nframe\nframe\nstat\n", sep = "")
	case:
		script = fmt.aprint(script, FIRST, "frame\nframe\nstat\n", sep = "")
	}
	gk.run_headless(&app, script, start)
	fmt.printf(
		"proto done mode=%s views=%d sides=%d clicks=%d events=%d frame=%d count=%d seen=%d ticks=%d sent=%d\n",
		app.mode, app.views, app.sides, app.clicks, app.events, app.frame, gk.peek(&app.count), app.seen, app.ticks, sync.atomic_load(&sent),
	)
	switch app.mode {
	case "post-memory":
		report_memory(&app, fmt.tprintf("post-memory done lines=%d", app.lines))
	case "handler-memory":
		report_memory(&app, fmt.tprintf("handler-memory done frames=%d", app.frame))
	case "rows":
		again := 0
		for _, n in app.described {
			if n > 1 {
				again += 1
			}
		}
		fmt.printf("proto rows described=%d again=%d\n", len(app.described), again)
	}
}
