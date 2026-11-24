//! Platform dialogs, application menus, URL opening, drag-and-drop predicates, image buffers,
//! accessibility subtrees, and scripted-dialog test hooks.
//!
//! # Async Completion and Cancellation Protocol
//!
//! - **Dual OS / scripted mode**: `C_PROMPT_PATHS`, `C_PROMPT_NEW_PATH`, and `C_WINDOW_PROMPT`
//!   immediately return an `HK_TASK` handle backed by a [`Completion`] drop guard and insert the
//!   handle into thread-local `LIVE`. When `SCRIPTED` is false, [`platform_request`] spawns a
//!   GPUI task awaiting the OS dialog future and defers [`deliver`] onto a clean UI turn. When
//!   `SCRIPTED` is true (headless runs or `GK_SCRIPTED_PLATFORM=1`), [`script_request`] pairs
//!   requests and `C_PLATFORM_ANSWER` / headless `answer` commands FIFO by dialog kind (`K_PATHS`,
//!   `K_NEW_PATH`, `K_PROMPT`), supporting both early answers (`ANSWERS`) and waiting requests (`PENDING`).
//! - **Cancellation safety**: releasing the `HK_TASK` handle via `C_RELEASE` drops [`Completion`],
//!   which cancels any background `Task`, removes the handle from `LIVE` and `PENDING`, and
//!   ensures [`deliver`] discards any already-queued completion (incrementing `COUNTS.2`) rather
//!   than invoking a released host callback.
//! - **Application menus (`C_SET_MENUS`, `C_MENUS_GET`)**: `gpui_kit::Menu` is not `Clone`, so
//!   `C_SET_MENUS` decodes each menu twice from the wire buffer (capped at 16 levels deep): once
//!   for OS `cx.set_menus` and once as `OwnedMenu` stored in `GlobalState` for `AppMenuBar` and
//!   readback.
//! - **Pixel images and GPU texture cleanup**: `C_RENDER_IMAGE_NEW` converts RGBA8 wire pixels
//!   into BGRA8 (`px.swap(0, 2)`) for GPUI's `RenderImage`. When a `RenderImage` handle is later
//!   released via `C_RELEASE`, [`released`] calls `window.drop_image` across every open window so
//!   GPU atlas textures are freed promptly.

use crate::genapi::{Call, HandlerRef};
use crate::ops::*;
use crate::rt::{self, GkResult, Item, err};
use crate::wire::{Put, R};
use gpui_kit::{
    AnyWindowHandle, App, Menu, MenuItem, OsAction, OwnedMenu, OwnedMenuItem, PathPromptOptions, PromptButton,
    PromptLevel, SystemMenuType, Window,
};
use std::cell::{Cell, RefCell};
use std::collections::{HashSet, VecDeque};
use std::path::PathBuf;

/// Wire completion status: the user confirmed the dialog and provided an answer.
pub const ANSWERED: u8 = 0;

/// Wire completion status: the user dismissed or canceled the dialog.
pub const CANCELLED: u8 = 1;

/// Wire completion status: the OS dialog failed with an error string.
pub const FAILED: u8 = 2;

const K_PATHS: u8 = 1;
const K_NEW_PATH: u8 = 2;
const K_PROMPT: u8 = 3;

/// Scripted dialog request waiting for a matching [`scripted_answer`] call.
struct Pending {
    handle: u64,
    kind: u8,
    h: HandlerRef,
    window: Option<AnyWindowHandle>,
}

thread_local! {
    /// When `true`, file dialogs, prompts, and URL opens are intercepted in-process.
    static SCRIPTED: Cell<bool> = Cell::new(std::env::var("GK_SCRIPTED_PLATFORM").is_ok_and(|v| v == "1"));
    /// FIFO queue of scripted requests awaiting an answer.
    static PENDING: RefCell<VecDeque<Pending>> = const { RefCell::new(VecDeque::new()) };
    /// FIFO queue of pre-supplied scripted answers pushed before the matching request arrived.
    static ANSWERS: RefCell<VecDeque<(u8, Vec<u8>)>> = const { RefCell::new(VecDeque::new()) };
    /// URLs recorded by `C_OPEN_URL` while in scripted mode.
    static OPENED: RefCell<Vec<String>> = const { RefCell::new(Vec::new()) };
    /// Active completion handles that have not yet been delivered or canceled via `C_RELEASE`.
    static LIVE: RefCell<HashSet<u64>> = RefCell::new(HashSet::new());
    /// Diagnostic tuple `(requests_created, completions_delivered, completions_discarded)`.
    static COUNTS: Cell<(u64, u64, u64)> = const { Cell::new((0, 0, 0)) };
}

/// Switches platform dialogs and `open_url` between live OS calls and scripted test queues.
pub fn set_scripted(on: bool) {
    SCRIPTED.with(|s| s.set(on));
}

fn scripted() -> bool {
    SCRIPTED.with(|s| s.get())
}

fn bump(f: impl FnOnce(&mut (u64, u64, u64))) {
    COUNTS.with(|c| {
        let mut v = c.get();
        f(&mut v);
        c.set(v);
    });
}

/// Guard stored under `HK_TASK`; dropping it cancels the background task and suppresses delivery.
struct Completion {
    handle: u64,
    _task: Option<gpui_kit::Task<()>>,
}

impl Drop for Completion {
    fn drop(&mut self) {
        let h = self.handle;
        let _ = LIVE.try_with(|l| {
            if let Ok(mut l) = l.try_borrow_mut() {
                l.remove(&h);
            }
        });
        let _ = PENDING.try_with(|p| {
            if let Ok(mut p) = p.try_borrow_mut() {
                p.retain(|x| x.handle != h);
            }
        });
    }
}

fn new_completion(task: Option<gpui_kit::Task<()>>, handle: u64) {
    LIVE.with(|l| l.borrow_mut().insert(handle));
    rt::with(|s| s.put(handle, Item::Guard(Box::new(Completion { handle, _task: task }))));
}

fn kept(r: &mut R) -> HandlerRef {
    let handler = r.u32();
    let generation = r.u32();
    HandlerRef { view: 0, generation, handler }
}

fn opt_str<'a>(r: &mut R<'a>) -> Option<&'a str> {
    if r.u8() != 0 { Some(r.str()) } else { None }
}

/// Delivers a `K_DONE` completion turn to the host if `handle` is still live (not canceled).
fn deliver(handle: u64, h: HandlerRef, shape: u16, payload: &[u8], window: Option<AnyWindowHandle>, cx: &mut App) {
    if !LIVE.with(|l| l.borrow_mut().remove(&handle)) {
        bump(|c| c.2 += 1);
        return;
    }
    bump(|c| c.1 += 1);
    rt::trace(|| format!("deliver completion={handle:#x} window={window:?}"));
    let mut req = Vec::with_capacity(24 + payload.len());
    req.put_u64(h.view);
    req.put_u32(h.generation);
    req.put_u32(h.handler);
    req.put_u16(shape);
    req.extend_from_slice(payload);
    let run = |window: Option<&mut Window>, cx: &mut App| {
        rt::guard("delivering a completion", || {
            let mut window = window;
            let mut reply = rt::turn(K_DONE, &req, window.as_deref_mut(), cx);
            rt::process_reply(&mut reply, K_DONE, window, cx);
        });
    };
    let done = match window {
        Some(any) => any.update(cx, |_, window, cx| run(Some(window), cx)).is_ok(),
        None => false,
    };
    if !done {
        run(None, cx);
    }
}

fn shape_of(kind: u8) -> u16 {
    if kind == K_PROMPT { SHAPE_PROMPT } else { SHAPE_PATHS }
}

fn request_window(c: &Call) -> Option<AnyWindowHandle> {
    c.window_handle().or_else(|| c.cx.active_window()).or_else(|| c.cx.windows().first().copied())
}

/// Enqueues a scripted dialog request: if a matching pre-supplied answer is already queued in
/// `ANSWERS`, delivers it on `cx.defer`; otherwise pushes onto `PENDING` until [`scripted_answer`] arrives.
fn script_request(kind: u8, h: HandlerRef, c: &mut Call) -> u64 {
    let handle = rt::with(|s| s.reserve(HK_TASK));
    let window = request_window(c);
    let early = ANSWERS.with(|a| {
        let mut a = a.borrow_mut();
        a.iter().position(|(k, _)| *k == kind).and_then(|i| a.remove(i))
    });
    new_completion(None, handle);
    match early {
        Some((_, payload)) => c.cx.defer(move |cx| deliver(handle, h, shape_of(kind), &payload, window, cx)),
        None => PENDING.with(|p| p.borrow_mut().push_back(Pending { handle, kind, h, window })),
    }
    handle
}

/// Spawns an async task awaiting OS dialog future `rx`, returning an `HK_TASK` cancellation handle.
fn platform_request<F>(rx: F, kind: u8, h: HandlerRef, c: &mut Call) -> u64
where
    F: std::future::Future<Output = Vec<u8>> + 'static,
{
    let handle = rt::with(|s| s.reserve(HK_TASK));
    let window = request_window(c);
    let task = c.cx.spawn(async move |cx| {
        let payload = rx.await;
        // Defer delivery so completion runs on a clean turn outside the async poll context.
        cx.update(|cx| cx.defer(move |cx| deliver(handle, h, shape_of(kind), &payload, window, cx)));
    });
    new_completion(Some(task), handle);
    handle
}

/// Encodes a path-dialog completion payload: `[status: u8][count: u32][path: str]*[error: str]`.
fn paths_payload(status: u8, paths: &[PathBuf], error: &str) -> Vec<u8> {
    let mut out = Vec::new();
    out.put_u8(status);
    out.put_u32(paths.len() as u32);
    for p in paths {
        out.put_str(&p.to_string_lossy());
    }
    out.put_str(error);
    out
}

/// Encodes a prompt-dialog completion payload: `[status: u8][button_index: u32]`.
fn prompt_payload(status: u8, answer: u32) -> Vec<u8> {
    let mut out = Vec::new();
    out.put_u8(status);
    out.put_u32(answer);
    out
}

fn os_action(n: u8) -> GkResult<Option<OsAction>> {
    Ok(Some(match n {
        0 => return Ok(None),
        1 => OsAction::Cut,
        2 => OsAction::Copy,
        3 => OsAction::Paste,
        4 => OsAction::SelectAll,
        5 => OsAction::Undo,
        6 => OsAction::Redo,
        n => return err(ERR_BAD_ENUM, format!("menu item: OS action {n}: 0 none, 1 cut, 2 copy, 3 paste, 4 select all, 5 undo, 6 redo")),
    }))
}

/// Recursively decodes a wire `Menu` tree, capping nesting at 16 levels.
fn menu(r: &mut R, depth: u32) -> GkResult<Menu> {
    if depth > 16 {
        return err(ERR_BAD_OP, "menus nested more than 16 deep");
    }
    let name = r.str().to_string();
    let disabled = r.u8() != 0;
    let n = r.u32();
    let mut items = Vec::with_capacity(n.min(256) as usize);
    for _ in 0..n {
        items.push(match r.u8() {
            0 => MenuItem::Separator,
            1 => {
                let name = r.str().to_string();
                let action = crate::app::boxed(r.u32())?;
                let os_action = os_action(r.u8())?;
                let checked = r.u8() != 0;
                let disabled = r.u8() != 0;
                MenuItem::Action { name: name.into(), action, os_action, checked, disabled }
            }
            2 => MenuItem::Submenu(menu(r, depth + 1)?),
            3 => MenuItem::os_submenu(r.str().to_string(), SystemMenuType::Services),
            k => return err(ERR_BAD_ENUM, format!("menu item kind {k}: 0 separator, 1 action, 2 submenu, 3 system")),
        });
    }
    Ok(Menu { name: name.into(), items, disabled })
}

/// Serializes an `OwnedMenu` back into the same wire format accepted by [`menu`].
fn put_owned(out: &mut Vec<u8>, m: &OwnedMenu) {
    out.put_str(&m.name);
    out.put_u8(m.disabled as u8);
    out.put_u32(m.items.len() as u32);
    for it in &m.items {
        match it {
            OwnedMenuItem::Separator => out.put_u8(0),
            OwnedMenuItem::Action { name, action, os_action, checked, disabled } => {
                out.put_u8(1);
                out.put_str(name);
                out.put_u32(crate::app::id_of(action.as_ref()));
                out.put_u8(match os_action {
                    None => 0,
                    Some(OsAction::Cut) => 1,
                    Some(OsAction::Copy) => 2,
                    Some(OsAction::Paste) => 3,
                    Some(OsAction::SelectAll) => 4,
                    Some(OsAction::Undo) => 5,
                    Some(OsAction::Redo) => 6,
                });
                out.put_u8(*checked as u8);
                out.put_u8(*disabled as u8);
            }
            OwnedMenuItem::Submenu(m) => {
                out.put_u8(2);
                put_owned(out, m);
            }
            OwnedMenuItem::SystemMenu(m) => {
                out.put_u8(3);
                out.put_str(&m.name);
            }
        }
    }
}

/// Dispatches platform dialogs, menus, URL opening, image caches/buffers, scroll anchors,
/// text-selection scopes, and platform protocol test opcodes.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_PROMPT_PATHS => (|| {
            let files = r.u8() != 0;
            let directories = r.u8() != 0;
            let multiple = r.u8() != 0;
            let prompt = opt_str(r).map(|s| s.to_string().into());
            let h = kept(r);
            if !files && !directories {
                return err(ERR_BAD_OP, "a file dialog that may choose neither files nor directories");
            }
            bump(|c| c.0 += 1);
            let handle = if scripted() {
                script_request(K_PATHS, h, c)
            } else {
                let rx = c.cx.prompt_for_paths(PathPromptOptions { files, directories, multiple, prompt });
                platform_request(
                    async move {
                        match rx.await {
                            Ok(Ok(Some(paths))) => paths_payload(ANSWERED, &paths, ""),
                            Ok(Ok(None)) | Err(_) => paths_payload(CANCELLED, &[], ""),
                            Ok(Err(e)) => paths_payload(FAILED, &[], &format!("{e:#}")),
                        }
                    },
                    K_PATHS,
                    h,
                    c,
                )
            };
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_PROMPT_NEW_PATH => (|| {
            let directory = PathBuf::from(r.str());
            let suggested = opt_str(r).map(|s| s.to_string());
            let h = kept(r);
            bump(|c| c.0 += 1);
            let handle = if scripted() {
                script_request(K_NEW_PATH, h, c)
            } else {
                let rx = c.cx.prompt_for_new_path(&directory, suggested.as_deref());
                platform_request(
                    async move {
                        match rx.await {
                            Ok(Ok(Some(path))) => paths_payload(ANSWERED, &[path], ""),
                            Ok(Ok(None)) | Err(_) => paths_payload(CANCELLED, &[], ""),
                            Ok(Err(e)) => paths_payload(FAILED, &[], &format!("{e:#}")),
                        }
                    },
                    K_NEW_PATH,
                    h,
                    c,
                )
            };
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_WINDOW_PROMPT => (|| {
            let wh = r.u64();
            let level = match r.u8() {
                0 => PromptLevel::Info,
                1 => PromptLevel::Warning,
                2 => PromptLevel::Critical,
                l => return err(ERR_BAD_ENUM, format!("prompt level {l}: 0 info, 1 warning, 2 critical")),
            };
            let message = r.str().to_string();
            let detail = opt_str(r).map(|s| s.to_string());
            let n = r.u32();
            let mut answers = Vec::with_capacity(n.min(64) as usize);
            for _ in 0..n {
                let kind = r.u8();
                let label = r.str().to_string();
                answers.push(match kind {
                    0 => PromptButton::new(label),
                    1 => PromptButton::ok(label),
                    2 => PromptButton::cancel(label),
                    k => return err(ERR_BAD_ENUM, format!("prompt answer kind {k}: 0 other, 1 ok, 2 cancel")),
                });
            }
            let h = kept(r);
            if answers.is_empty() {
                return err(ERR_BAD_OP, "a prompt needs at least one answer");
            }
            bump(|c| c.0 += 1);
            let handle = if scripted() {
                if wh != 0 {
                    rt::with(|s| s.window(wh))?;
                } else if request_window(c).is_none() {
                    return err(ERR_NO_CONTEXT, "a prompt needs a window: the app has none open");
                }
                script_request(K_PROMPT, h, c)
            } else {
                let any = match wh {
                    0 => match request_window(c) {
                        Some(w) => w,
                        None => return err(ERR_NO_CONTEXT, "a prompt needs a window: the app has none open"),
                    },
                    wh => rt::with(|s| s.window(wh))?,
                };
                if c.window_handle() == Some(any) {
                    let window = c.window_opt();
                    let rx = rt::with_window(any, window, c.cx, |window, cx| {
                        window.prompt(level, &message, detail.as_deref(), &answers, cx)
                    })?;
                    platform_request(
                        async move {
                            match rx.await {
                                Ok(i) => prompt_payload(ANSWERED, i as u32),
                                Err(_) => prompt_payload(CANCELLED, 0),
                            }
                        },
                        K_PROMPT,
                        h,
                        c,
                    )
                } else {
                    // Opening a prompt on a non-current window must be deferred to avoid nested window borrows.
                    let (tx, shown) = futures::channel::oneshot::channel();
                    c.cx.defer(move |cx| {
                        if let Ok(rx) = any.update(cx, |_, window, cx| window.prompt(level, &message, detail.as_deref(), &answers, cx)) {
                            let _ = tx.send(rx);
                        }
                    });
                    platform_request(
                        async move {
                            match shown.await {
                                Ok(rx) => match rx.await {
                                    Ok(i) => prompt_payload(ANSWERED, i as u32),
                                    Err(_) => prompt_payload(CANCELLED, 0),
                                },
                                Err(_) => prompt_payload(FAILED, 0),
                            }
                        },
                        K_PROMPT,
                        h,
                        c,
                    )
                }
            };
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_OPEN_URL => {
            let url = r.str();
            if scripted() {
                OPENED.with(|o| o.borrow_mut().push(url.to_string()));
            } else {
                c.cx.open_url(url);
            }
            Ok(())
        }
        C_SET_MENUS => (|| {
            let n = r.u32();
            let mut menus = Vec::with_capacity(n.min(64) as usize);
            let mut owned = Vec::with_capacity(n.min(64) as usize);
            for _ in 0..n {
                // `Menu` is not `Clone`, so decode each menu twice: once for GPUI OS menus
                // and once as `OwnedMenu` for `GlobalState` (used by `AppMenuBar` and `C_MENUS_GET`).
                let mut again = R::at(r.b, r.i);
                menus.push(menu(r, 0)?);
                owned.push(menu(&mut again, 0)?.owned());
            }
            c.cx.set_menus(menus);
            gpui_kit::base::GlobalState::global_mut(c.cx).set_app_menus(owned);
            Ok(())
        })(),
        C_MENUS_GET => {
            let menus: Vec<OwnedMenu> = gpui_kit::base::GlobalState::global(c.cx).app_menus().to_vec();
            c.out.put_u32(menus.len() as u32);
            for m in &menus {
                put_owned(c.out, m);
            }
            Ok(())
        }
        C_PLATFORM_ANSWER => (|| {
            if !scripted() {
                return err(
                    ERR_UNSUPPORTED,
                    "scripted answers need the offscreen driver or GK_SCRIPTED_PLATFORM=1: the platform answers this app's dialogs",
                );
            }
            let kind = r.u8();
            let status = r.u8();
            let payload = match kind {
                K_PATHS | K_NEW_PATH => {
                    let n = r.u32();
                    let paths: Vec<PathBuf> = (0..n).map(|_| PathBuf::from(r.str())).collect();
                    let error = r.str().to_string();
                    paths_payload(status, &paths, &error)
                }
                K_PROMPT => prompt_payload(status, r.u32()),
                k => return err(ERR_BAD_ENUM, format!("answer kind {k}: 1 paths, 2 new path, 3 prompt")),
            };
            if status > FAILED {
                return err(ERR_BAD_ENUM, format!("answer status {status}: 0 answered, 1 cancelled, 2 failed"));
            }
            c.out.put_u8(scripted_answer(kind, payload, c.cx) as u8);
            Ok(())
        })(),
        C_PLATFORM_LOG => {
            let opened = OPENED.with(|o| o.borrow().clone());
            c.out.put_u32(opened.len() as u32);
            for u in &opened {
                c.out.put_str(u);
            }
            let (made, delivered, discarded) = COUNTS.with(|c| c.get());
            c.out.put_u64(made);
            c.out.put_u64(delivered);
            c.out.put_u64(discarded);
            c.out.put_u32(PENDING.with(|p| p.borrow().len()) as u32);
            c.out.put_u8(scripted() as u8);
            Ok(())
        }
        C_IMAGE_CACHE_NEW => {
            let cache = gpui_kit::RetainAllImageCache::new(c.cx);
            c.out.put_u64(crate::genapi::obj_new(cache));
            Ok(())
        }
        C_RENDER_IMAGE_NEW => {
            let img = render_image_new(r);
            c.out.put_u64(crate::genapi::obj_new(img));
            Ok(())
        }
        C_SCROLL_ANCHOR_NEW => (|| {
            let handle = crate::genapi::obj_clone::<gpui_kit::ScrollHandle>(r.u64())?;
            c.out.put_u64(crate::genapi::obj_new(gpui_kit::ScrollAnchor::for_handle(handle)));
            Ok(())
        })(),
        C_SCROLL_ANCHOR_TO => (|| {
            let anchor = crate::genapi::obj_clone::<gpui_kit::ScrollAnchor>(r.u64())?;
            let window = c.window()?;
            anchor.scroll_to(window, c.cx);
            Ok(())
        })(),
        C_TEXT_SCOPE_NEW => {
            c.out.put_u64(crate::genapi::obj_new(gpui_kit::base::TextSelectionScopeId::new()));
            Ok(())
        }
        C_PLATFORM_TEST => (|| test_call(r, c))(),
        _ => return None,
    })
}

/// Matches a scripted answer against the oldest waiting [`Pending`] request of the same `kind`,
/// returning `true` if delivered immediately or `false` if buffered in `ANSWERS` for a future request.
fn scripted_answer(kind: u8, payload: Vec<u8>, cx: &mut App) -> bool {
    let waiting = PENDING.with(|p| {
        let mut p = p.borrow_mut();
        p.iter().position(|x| x.kind == kind).and_then(|i| p.remove(i))
    });
    match waiting {
        Some(Pending { handle, h, window, .. }) => {
            cx.defer(move |cx| deliver(handle, h, shape_of(kind), &payload, window, cx));
            true
        }
        None => {
            ANSWERS.with(|a| a.borrow_mut().push_back((kind, payload)));
            false
        }
    }
}

/// Parses a headless script `answer <cmd>` line and routes it to [`scripted_answer`].
pub fn answer_line(line: &str, cx: &mut App) -> anyhow::Result<()> {
    let mut words = line.split_whitespace();
    let what = words.next().unwrap_or("");
    let rest: Vec<&str> = words.collect();
    let (kind, payload) = match what {
        "paths" | "new-path" => {
            let paths: Vec<PathBuf> = rest.iter().map(PathBuf::from).collect();
            (if what == "paths" { K_PATHS } else { K_NEW_PATH }, paths_payload(ANSWERED, &paths, ""))
        }
        "prompt" => {
            let n: u32 = rest.first().and_then(|v| v.parse().ok()).ok_or_else(|| anyhow::anyhow!("answer prompt N: {line}"))?;
            (K_PROMPT, prompt_payload(ANSWERED, n))
        }
        "cancel" => match rest.first().copied() {
            Some("paths") => (K_PATHS, paths_payload(CANCELLED, &[], "")),
            Some("new-path") => (K_NEW_PATH, paths_payload(CANCELLED, &[], "")),
            Some("prompt") => (K_PROMPT, prompt_payload(CANCELLED, 0)),
            _ => anyhow::bail!("answer cancel paths|new-path|prompt: {line}"),
        },
        _ => anyhow::bail!("answer paths P.. | new-path P | prompt N | cancel KIND: {line}"),
    };
    let took = scripted_answer(kind, payload, cx);
    println!("gk-headless: answer {line}: {}", if took { "taken" } else { "kept for the next request" });
    Ok(())
}

/// Builds a `can_drop` predicate closure from a wire [`DropAccept`] descriptor.
pub fn drop_accept(r: &mut R) -> impl Fn(&dyn std::any::Any, &mut Window, &mut App) -> bool + 'static {
    let accept = DropAccept::read(r);
    move |v, _, _| accept.accepts(v)
}

/// Wire filter specifying whether a drop target accepts internal [`crate::values::Drag`] values
/// (optionally restricted to specific IDs) and/or OS `ExternalPaths`.
#[derive(Clone, Debug, Default)]
pub struct DropAccept {
    values: bool,
    ids: Vec<u64>,
    paths: bool,
}

impl DropAccept {
    pub fn read(r: &mut R) -> Self {
        let values = r.u8() != 0;
        let n = r.u32();
        let ids = (0..n).map(|_| r.u64()).collect();
        let paths = r.u8() != 0;
        DropAccept { values, ids, paths }
    }

    pub fn accepts(&self, v: &dyn std::any::Any) -> bool {
        if let Some(d) = v.downcast_ref::<crate::values::Drag>() {
            return self.values && (self.ids.is_empty() || self.ids.contains(&d.id));
        }
        v.downcast_ref::<gpui_kit::ExternalPaths>().is_some() && self.paths
    }
}

fn files(r: &mut R) -> Vec<(PathBuf, bool)> {
    let n = r.u32();
    (0..n)
        .map(|_| {
            let p = PathBuf::from(r.str());
            (p, r.u8() != 0)
        })
        .collect()
}

/// Decodes a list of `(path, is_dir)` entries into an `ExternalDragPayload` callback.
pub fn drag_payload(
    r: &mut R,
) -> impl Fn(&crate::values::Drag, &mut Window, &mut App) -> Option<gpui_kit::ExternalDragPayload> + 'static {
    let files = files(r);
    move |_, _, _| {
        if files.is_empty() {
            None
        } else {
            Some(gpui_kit::ExternalDragPayload::Files(gpui_kit::FileDragPaths::new(files.clone())))
        }
    }
}

/// Reads a `u32`-prefixed list of `u64` indices as `Vec<usize>`.
pub fn usizes(r: &mut R) -> Vec<usize> {
    let n = r.u32();
    (0..n).map(|_| r.u64() as usize).collect()
}

/// Builds a closure that emits synthetic AccessKit child nodes `(key, role, label)`.
pub fn a11y_children(
    nodes: Vec<(String, gpui_kit::Role, String)>,
) -> impl FnOnce(&mut gpui_kit::A11ySubtreeBuilder<'_>) + 'static {
    move |b| {
        for (key, role, label) in &nodes {
            let id = b.synthetic_node_id(key);
            let mut node = accesskit::Node::new(*role);
            if !label.is_empty() {
                node.set_label(label.clone());
            }
            b.push_child(id, node);
        }
    }
}

/// Polymorphic `ImageCacheProvider` selecting either a host-retained cache handle or a keyed
/// `RetainAllImageCacheProvider`.
pub enum CacheProvider {
    Held(gpui_kit::Entity<gpui_kit::RetainAllImageCache>),
    RetainAll(gpui_kit::RetainAllImageCacheProvider),
}

impl gpui_kit::ImageCacheProvider for CacheProvider {
    fn provide(&mut self, window: &mut Window, cx: &mut App) -> gpui_kit::AnyImageCache {
        match self {
            CacheProvider::Held(e) => e.provide(window, cx),
            CacheProvider::RetainAll(p) => p.provide(window, cx),
        }
    }
}

/// Decodes a [`CacheProvider`] from the wire (`0` = handle, nonzero = keyed retain-all).
pub fn cache_provider(r: &mut R) -> CacheProvider {
    match r.u8() {
        0 => CacheProvider::Held(image_cache_shown(r.u64())),
        _ => CacheProvider::RetainAll(gpui_kit::retain_all(gpui_kit::SharedString::from(r.str().to_string()))),
    }
}

/// Resolves an `HK_OBJ` `RetainAllImageCache` entity handle during tape decoding.
pub fn image_cache_shown(h: u64) -> gpui_kit::Entity<gpui_kit::RetainAllImageCache> {
    crate::genapi::obj_shown::<gpui_kit::Entity<gpui_kit::RetainAllImageCache>>(h)
}

/// Resolves an `HK_OBJ` `ScrollAnchor` handle during tape decoding.
pub fn anchor_shown(h: u64) -> gpui_kit::ScrollAnchor {
    crate::genapi::obj_shown::<gpui_kit::ScrollAnchor>(h)
}

/// Resolves an `HK_OBJ` `TextSelectionScopeId` handle during tape decoding.
pub fn scope_shown(h: u64) -> gpui_kit::base::TextSelectionScopeId {
    crate::genapi::obj_shown::<gpui_kit::base::TextSelectionScopeId>(h)
}

/// Decodes RGBA8 pixel bytes into a GPUI `RenderImage`, swizzling RGBA -> BGRA as GPUI expects.
fn render_image_new(r: &mut R) -> std::sync::Arc<gpui_kit::RenderImage> {
    let (w, h) = (r.u32(), r.u32());
    let n = r.u32() as usize;
    let mut bytes = r.bytes(n).to_vec();
    let want = w as usize * h as usize * 4;
    bytes.resize(want, 0);
    for px in bytes.chunks_exact_mut(4) {
        px.swap(0, 2);
    }
    let buf = image::RgbaImage::from_raw(w, h, bytes).unwrap_or_else(|| image::RgbaImage::new(w.max(1), h.max(1)));
    std::sync::Arc::new(gpui_kit::RenderImage::new(vec![image::Frame::new(buf)]))
}

/// Resolves a `RenderImage` handle referenced by an element and tracks usage under `diag`.
pub fn pixel_image(r: &mut R) -> std::sync::Arc<gpui_kit::RenderImage> {
    let img = crate::genapi::obj_shown::<std::sync::Arc<gpui_kit::RenderImage>>(r.u64());
    #[cfg(feature = "diag")]
    IMAGES_SHOWN.with(|s| {
        let mut s = s.borrow_mut();
        s.0 += 1;
        s.1.insert(img.id.0);
    });
    img
}

#[cfg(feature = "diag")]
thread_local! {
    /// Counts `(times_shown, distinct_image_ids)` for image-retention tests.
    static IMAGES_SHOWN: RefCell<(u64, HashSet<usize>)> = RefCell::new((0, HashSet::new()));
    /// Counts `window.drop_image` calls made when `RenderImage` handles are released.
    static IMAGES_DROPPED: Cell<u64> = const { Cell::new(0) };
}

/// Releases GPU texture cache entries across all open windows when a `RenderImage` handle is freed.
pub fn released(item: &Option<Item>, c: &mut Call) {
    let Some(Item::Obj(crate::objects::ObjItem::Owned { cell, .. })) = item else {
        return;
    };
    let Some(img) = cell.downcast_ref::<RefCell<std::sync::Arc<gpui_kit::RenderImage>>>() else {
        return;
    };
    let img = img.borrow().clone();
    for any in c.cx.windows() {
        let window = c.window_opt();
        let _ = rt::with_window(any, window, c.cx, |window, _| {
            let _ = window.drop_image(img.clone());
            #[cfg(feature = "diag")]
            IMAGES_DROPPED.with(|d| d.set(d.get() + 1));
        });
    }
}

/// Wraps a resolved `RenderImage` in the closure signature expected by `ImageSource::Custom`.
pub fn custom_image(
    r: &mut R,
) -> std::sync::Arc<dyn Fn(&mut Window, &mut App) -> Option<Result<std::sync::Arc<gpui_kit::RenderImage>, gpui_kit::ImageCacheError>>> {
    let img = pixel_image(r);
    std::sync::Arc::new(move |_: &mut Window, _: &mut App| Some(Ok(img.clone())))
}

impl crate::genapi::Wire for gpui_kit::ExternalPaths {
    fn put(&self, out: &mut Vec<u8>) {
        let paths = self.paths();
        out.put_u32(paths.len() as u32);
        for p in paths {
            out.put_str(&p.to_string_lossy());
        }
    }
}

/// Snapshot of a `DragMoveEvent<Drag>` that copies the dragged value before crossing the wire.
pub struct DragMove {
    event: gpui_kit::MouseMoveEvent,
    bounds: gpui_kit::Bounds<gpui_kit::Pixels>,
    value: crate::values::Drag,
}

impl DragMove {
    pub fn of(e: &gpui_kit::DragMoveEvent<crate::values::Drag>, cx: &App) -> Self {
        DragMove { event: e.event.clone(), bounds: e.bounds, value: e.drag(cx).clone() }
    }
}

impl crate::genapi::Wire for DragMove {
    fn put(&self, out: &mut Vec<u8>) {
        crate::genapi::Wire::put(&self.event, out);
        crate::genapi::Wire::put(&self.bounds, out);
        crate::genapi::Wire::put(&self.value, out);
    }
}

/// Protocol test sub-dispatcher (`C_PLATFORM_TEST`) invoked by Go platform integration tests:
/// - `1`: evaluates [`DropAccept`] against a synthetic `Drag{id}` and an `ExternalPaths`
/// - `2`: runs [`drag_payload`] and echoes back the decoded `(path, is_dir)` entries
/// - `3`: round-trips [`usizes`] dynamic prepaint order lists
/// - `4`: resolves a `RenderImage` (direct or via [`custom_image`]) and returns `(w, h, first_px_le, id)`
/// - `5`: encodes a synthetic `FileDropEvent::Entered` with two paths to verify wire decoding
/// - `6`: invokes `app::ask_should_close` on window `wh` and returns the veto boolean
/// - `7` (`diag` only): returns `(times_shown, distinct_ids, gpu_drops)` from image tracking counters
fn test_call(r: &mut R, c: &mut Call) -> GkResult<()> {
    match r.u8() {
        1 => {
            let a = DropAccept::read(r);
            let id = r.u64();
            let d = crate::values::Drag { id, text: String::new() };
            c.out.put_u8(a.accepts(&d) as u8);
            let paths = gpui_kit::ExternalPaths::default();
            c.out.put_u8(a.accepts(&paths) as u8);
        }
        2 => {
            let f = drag_payload(r);
            let window = c.window()?;
            let got = f(&crate::values::Drag::default(), window, c.cx);
            match got {
                Some(gpui_kit::ExternalDragPayload::Files(p)) => {
                    let v: Vec<(PathBuf, bool)> = p.entries().to_vec();
                    c.out.put_u32(v.len() as u32);
                    for (p, d) in v {
                        c.out.put_str(&p.to_string_lossy());
                        c.out.put_u8(d as u8);
                    }
                }
                _ => c.out.put_u32(0),
            }
        }
        3 => {
            let v = usizes(r);
            c.out.put_u32(v.len() as u32);
            for x in v {
                c.out.put_u64(x as u64);
            }
        }
        4 => {
            let custom = r.u8() != 0;
            let img = if !custom {
                pixel_image(r)
            } else {
                let f = custom_image(r);
                let window = c.window()?;
                match f(window, c.cx) {
                    Some(Ok(img)) => img,
                    _ => return err(ERR_BAD_OP, "a custom image answered nothing"),
                }
            };
            let size = img.size(0);
            c.out.put_u32(size.width.0 as u32);
            c.out.put_u32(size.height.0 as u32);
            let first = img.as_bytes(0).map(|b| u32::from_le_bytes([b[0], b[1], b[2], b[3]])).unwrap_or(0);
            c.out.put_u32(first);
            c.out.put_u64(img.id.0 as u64);
        }
        5 => {
            let paths = gpui_kit::ExternalPaths(vec![PathBuf::from("/tmp/a.txt"), PathBuf::from("/tmp/b dir")].into());
            let ev = gpui_kit::FileDropEvent::Entered { position: gpui_kit::point(gpui_kit::px(3.), gpui_kit::px(4.)), paths };
            crate::genapi::Wire::put(&ev, c.out);
        }
        6 => {
            let wh = r.u64();
            let window = c.window()?;
            let yes = crate::app::ask_should_close(wh, window, c.cx);
            c.out.put_u8(yes as u8);
        }
        #[cfg(feature = "diag")]
        7 => {
            let (times, ids) = IMAGES_SHOWN.with(|s| {
                let s = s.borrow();
                (s.0, s.1.len())
            });
            c.out.put_u64(times);
            c.out.put_u32(ids as u32);
            c.out.put_u64(IMAGES_DROPPED.with(|d| d.get()));
        }
        #[cfg(not(feature = "diag"))]
        7 => return err(ERR_UNSUPPORTED, "platform test 7 (the images shown) needs the diagnostic library"),
        k => return err(ERR_BAD_ENUM, format!("platform test {k}: 1 drop accept, 2 drag payload, 3 prepaint order, 4 image, 5 file drop, 6 should close, 7 images shown")),
    }
    Ok(())
}
