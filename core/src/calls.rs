//! Top-level dispatcher for synchronous (`gk_call`) and deferred wire call opcodes.
//!
//! Core lifecycle, focus, handle release, and protocol-test opcodes are handled here; all other
//! opcodes are routed through the domain subsystems and the generated state/call dispatcher.

use crate::genapi::{self, Call, HandlerRef};
use crate::ops::*;
use crate::rt::{self, GkResult, Item, err};
use crate::wire::{Put, R};
use gpui_kit::{App, Window};

/// Decodes the leading `u16` opcode of `req`, executes it, and writes any reply bytes into `out`.
pub fn call(
    req: &[u8],
    window: Option<&mut Window>,
    cx: &mut App,
    out: &mut Vec<u8>,
    deferred: bool,
) -> GkResult<()> {
    if req.len() < 2 {
        return err(ERR_BAD_OP, format!("a request of {} bytes has no op number", req.len()));
    }
    let mut r = R::new(req);
    let op = r.u16();
    let mut c = Call::new(cx, window, out, deferred, req);
    match op {
        C_VIEW_NEW => {
            c.out.put_u64(rt::new_view(c.cx));
            Ok(())
        }
        C_OPEN_WINDOW => {
            let (w, h) = (r.f32(), r.f32());
            let root = r.u64();
            c.out.put_u64(rt::open_window(w, h, root, c.cx)?);
            Ok(())
        }
        C_ON_FRAME => {
            let wh = r.u64();
            let on = r.u8() != 0;
            rt::with(|s| s.window(wh))?;
            rt::with(|s| {
                if on {
                    s.frame_handler.insert(wh);
                } else {
                    s.frame_handler.remove(&wh);
                }
            });
            if on {
                let window = c.window_opt();
                rt::ensure_hook(wh, window, c.cx);
            }
            Ok(())
        }
        C_QUIT => {
            #[cfg(feature = "diag")]
            gk_meter::headless::stop();
            c.cx.quit();
            Ok(())
        }
        C_RELEASE => {
            // Remove from the handle table before dropping so any Drop impl can safely borrow rt.
            let item = rt::with(|s| s.release(r.u64()))?;
            if let Some(Item::Source(source)) = &item {
                // Cascade release to child row views owned by a released row source.
                for v in source.views() {
                    let row = rt::with(|s| s.release(v)).ok();
                    drop(row);
                }
            }
            crate::platform::released(&item, &mut c);
            drop(item);
            Ok(())
        }
        C_FOCUS_HANDLE_NEW => {
            let focus = c.cx.focus_handle();
            let window = c.window_handle();
            c.out.put_u64(rt::with(|s| s.insert(HK_FOCUS, Item::Focus(focus, window))));
            Ok(())
        }
        C_FOCUS => {
            let (focus, window) = rt::with(|s| s.focus(r.u64()))?;
            let Some(any) = window.or_else(|| c.window_handle()) else {
                return err(ERR_NO_CONTEXT, "focus needs a window");
            };
            let window = c.window_opt();
            rt::with_window(any, window, c.cx, |window, cx| window.focus(&focus, cx))
        }
        C_TEST_FIRE => {
            // Protocol test hook: fires a `() -> bool` handler synchronously.
            let h = HandlerRef {
                view: r.u64(),
                generation: r.u32(),
                handler: r.u32(),
            };
            let window = c.window()?;
            let ret: bool = genapi::fire(h, SHAPE_UNIT_BOOL, &(), window, c.cx);
            c.out.put_i32(ret as i32);
            Ok(())
        }
        C_TEST_FIRE_VALUE => {
            // Protocol test hook: fires a `(str, u32) -> String` handler synchronously.
            let h = HandlerRef {
                view: r.u64(),
                generation: r.u32(),
                handler: r.u32(),
            };
            let text = r.str();
            let n = r.u32();
            let window = c.window()?;
            let ret: String = genapi::fire(h, SHAPE_TEST, &(text, n), window, c.cx);
            c.out.put_str(&ret);
            Ok(())
        }
        C_TEST_FIRE_SHAPE => {
            // Protocol test hook: fires any wire handler shape through generated or raw dispatch.
            let h = HandlerRef {
                view: r.u64(),
                generation: r.u32(),
                handler: r.u32(),
            };
            let shape = r.u16();
            let n = r.u32() as usize;
            let payload = r.bytes(n);
            let window = c.window()?;
            let (typed, ret) = match crate::generated::test_shape(shape, h, &mut R::new(payload), window, c.cx) {
                Some(ret) => (1, ret),
                None => (0, genapi::fire_raw(h, shape, payload, window, c.cx)),
            };
            c.out.put_u8(typed);
            c.out.put_u32(ret.len() as u32);
            c.out.extend_from_slice(&ret);
            Ok(())
        }
        C_TEST_PANIC => panic!("test panic asked for by the app (call op TEST_PANIC)"),
        C_ROWS_NEW | C_ROWS_PUSH => match crate::hand::call(op, &mut r, &mut c) {
            Some(done) => done,
            None => err(ERR_BAD_OP, format!("unknown call op {op}")),
        },
        _ => {
            if let Some(done) = crate::app::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::platform::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::objects::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::values::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::source::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::shim_input::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::shim_rows::call(op, &mut r, &mut c) {
                return done;
            }
            if let Some(done) = crate::generated::call(op, &mut r, &mut c) {
                return done;
            }
            match crate::hand::call(op, &mut r, &mut c) {
                Some(done) => done,
                None => err(ERR_BAD_OP, format!("unknown call op {op}")),
            }
        }
    }
}
