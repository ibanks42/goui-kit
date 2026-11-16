// attack: what another binding (or a broken one) can send through the C interface of
// docs/DESIGN.md section 5, without the Odin package's checks in between. This program is its
// own raw client: its own callback, its own requests and replies, the package used only for
// the numbers. Each attack must end in an error code or a stop with a message, never in a
// crash, a hang or memory written where it must not be (the second review, 11 November 2026).
// core/tests/run-attack.sh checks the lines and the exit code.
//
//   GK_PROTO=truncated     a request cut short in its arguments: the library stops (poisoned,
//                          exit 1) with the message; later calls are refused, nothing crashes
//   GK_PROTO=unknown-op    an op number no library has, an empty and a null request: refused
//                          with GK_ERR_BAD_OP, and the library goes on working
//   GK_PROTO=forged        a made-up handle, a handle of another kind, one released twice, a
//                          call from another thread: each refused with its code
//   GK_PROTO=stale-cb      gk_reply with the gk_cb of a callback that has returned: refused
//                          (it pointed into a stack frame that is gone)
//   GK_PROTO=bad-reply     a reply whose counts run past its bytes: the library stops with the
//                          message (exit 1), it does not read past the reply
package attack

import "base:runtime"
import "core:c"
import "core:fmt"
import "core:os"
import "core:thread"
import gk "gpuikit:gk"

mode: string
stale_cb: ^gk.Gk_Cb
views_seen: int
first_view: u64

put_u16 :: proc(b: ^[dynamic]u8, v: u16) {append(b, u8(v), u8(v >> 8))}
put_u32 :: proc(b: ^[dynamic]u8, v: u32) {for i in 0 ..< 4 {append(b, u8(v >> (8 * u32(i))))}}
put_u64 :: proc(b: ^[dynamic]u8, v: u64) {for i in 0 ..< 8 {append(b, u8(v >> (8 * u64(i))))}}
put_f32 :: proc(b: ^[dynamic]u8, v: f32) {put_u32(b, transmute(u32)v)}

// call sends one request; ret is the first u64 of the result, if any.
call :: proc(req: []u8) -> (rc: i32, ret: u64) {
	p: [^]u8
	n: c.size_t
	rc = gk.gk_call(raw_data(req), len(req), &p, &n)
	if rc == 0 && n >= 8 {
		for i in 0 ..< 8 {
			ret |= u64(p[i]) << (8 * u64(i))
		}
	}
	return
}

last :: proc() -> string {
	n: c.size_t
	p := gk.gk_last_error(&n)
	return string(p[:n])
}

op :: proc(code: u16, args: ..u64) -> []u8 {
	b := make([dynamic]u8, context.temp_allocator)
	put_u16(&b, code)
	for a in args {
		put_u64(&b, a)
	}
	return b[:]
}

from_thread :: proc(t: ^thread.Thread) {
	req := op(gk.C_VIEW_NEW)
	rc, _ := call(req)
	(cast(^i32)t.data)^ = rc
}

on_init :: proc() {
	_, view := call(op(gk.C_VIEW_NEW))
	first_view = view
	w := make([dynamic]u8, context.temp_allocator)
	put_u16(&w, gk.C_OPEN_WINDOW)
	put_f32(&w, 200)
	put_f32(&w, 100)
	put_u64(&w, view)
	rc, _ := call(w[:])
	fmt.printf("attack init view=%#x window rc=%d msg=%q\n", view, rc, last() if rc != 0 else "")
	switch mode {
	case "truncated":
		// SOURCE_SET_COUNT takes u64 source, u32 section, u64 count: 3 bytes are sent.
		req := [?]u8{u8(gk.C_SOURCE_SET_COUNT), 0, 1, 2, 3}
		rc, _ = call(req[:])
		fmt.printf("attack truncated rc=%d msg=%q\n", rc, last())
		rc2, _ := call(op(gk.C_VIEW_NEW))
		fmt.printf("attack truncated after rc=%d\n", rc2)
		os.exit(1)
	case "unknown-op":
		rc, _ = call(op(0xfff0))
		fmt.printf("attack unknown-op rc=%d msg=%q\n", rc, last())
		buf := [1]u8{0}
		rc, _ = call(buf[:0]) // an empty request, not a null one
		fmt.printf("attack empty rc=%d msg=%q\n", rc, last())
		p: [^]u8
		n: c.size_t
		rc = gk.gk_call(nil, 0, &p, &n)
		fmt.printf("attack null rc=%d msg=%q\n", rc, last())
		rc, view = call(op(gk.C_VIEW_NEW))
		fmt.printf("attack unknown-op after rc=%d view=%#x\n", rc, view)
	case "forged":
		forged := u64(gk.HK_SOURCE) << 56 | 0x0000_0001_0000_0099
		req := make([dynamic]u8, context.temp_allocator)
		put_u16(&req, gk.C_SOURCE_SET_COUNT)
		put_u64(&req, forged)
		put_u32(&req, 0)
		put_u64(&req, 5)
		rc, _ = call(req[:])
		fmt.printf("attack forged source rc=%d msg=%q\n", rc, last())
		clear(&req)
		put_u16(&req, gk.C_SOURCE_SET_COUNT)
		put_u64(&req, view) // a view, not a source
		put_u32(&req, 0)
		put_u64(&req, 5)
		rc, _ = call(req[:])
		fmt.printf("attack wrong-kind rc=%d msg=%q\n", rc, last())
		_, v2 := call(op(gk.C_VIEW_NEW))
		rc1, _ := call(op(gk.C_RELEASE, v2))
		rc2, _ := call(op(gk.C_RELEASE, v2))
		fmt.printf("attack release-twice first=%d second=%d msg=%q\n", rc1, rc2, last())
		got: i32 = -1
		t := thread.create(from_thread)
		t.data = &got
		thread.start(t)
		thread.join(t)
		thread.destroy(t)
		fmt.printf("attack thread rc=%d\n", got)
	}
}

callback :: proc "c" (user: rawptr, cb: ^gk.Gk_Cb, kind: u32, req: [^]u8, n: c.size_t) -> i32 {
	context = runtime.default_context()
	switch kind {
	case gk.K_PANIC:
		fmt.printf("attack library-panic msg=%q\n", string(req[4:n]))
		os.exit(1)
	case gk.K_INIT:
		stale_cb = cb
		on_init()
	case gk.K_VIEWS:
		views_seen += 1
		if views_seen == 1 && mode == "stale-cb" {
			reply := [?]u8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
			rc := gk.gk_reply(stale_cb, raw_data(reply[:]), len(reply))
			fmt.printf("attack stale-cb rc=%d msg=%q\n", rc, last())
		}
		if views_seen == 1 && mode == "bad-reply" {
			// ret of 0 bytes, then 4,294,967,295 dirty views and no bytes for them.
			reply := [?]u8{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
			rc := gk.gk_reply(cb, raw_data(reply[:]), len(reply))
			fmt.printf("attack bad-reply sent rc=%d\n", rc)
		}
	}
	return 0
}

main :: proc() {
	mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	app := gk.Gk_App{size_of(gk.Gk_App), gk.ABI_MAJOR, gk.ABI_MINOR, gk.API_HASH, callback, nil, 0}
	script := "size 200 100\nframe\nframe\n"
	rc := gk.gk_headless_run(&app, raw_data(script), len(script))
	fmt.printf("attack done mode=%s rc=%d views=%d msg=%q\n", mode, rc, views_seen, last() if rc != 0 else "")
	os.exit(1 if rc != 0 else 0)
}
