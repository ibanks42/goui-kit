"""Value types, generic object instantiations, motion/action/traitval dispatchers, and partial-enum audits.

This module is the main extension hook plugged into `gen/gk_model.py` (during type
classification and post-processing) and `gen/gk_rust.py` (during Rust decoder and
`mod.rs` emission). It owns six areas that do not fit the core single-struct/enum rules:

1. **Extended `resolved_path` and array descriptors (`resolved`, `array`, `access_role`)**:
   - GPUI motion transforms (`Transformation`, `TimedTransformation`), `accesskit::Role`
     (loaded from `api/accesskit-role.json`), `jiff::civil` date/time/weekday values,
     `Range<Pixels|Rems>`, outbound `Duration`, `FocusHandle`, `ScrollHandle`,
     `Option<&mut Window>`, outbound `Result<T, E>` (`fallible` or unit `result`),
     outbound `HashMap<K, V>` (wired as a list of `(K, V)` pairs), `Rc<T>` / `Arc<T>`
     shared pointer objects, fixed-size arrays `[T; 1..=16]`, and concrete instantiated
     generic nodes/objects.
   - `CALL_SAFE` vs `DEC_ONLY` tells `gk_model.py` which descriptors can cross synchronous
     call ops (no `Dx` tape builder) vs which require an active view-tape decoder.
2. **Readback handles (`readback`, `readback_reads`, `handed_states`, `nested_iterators`)**:
   - When an upstream method returns a `Clone`, private-field builder/value struct by
     value, `readback()` synthesizes a `read:<path>` object handle (`<Name>Read`) and
     `readback_reads()` binds its `&self` getters so Go can inspect it.
   - `handed_states()` downgrades methods returning `Entity<T>` whose state type has no
     bound constructor or runtime hand-out, preventing dangling Go handle types.
   - `nested_iterators()` downgrades return types with nested `iter` descriptors (direct
     iterator-of-iterators is later rescued by `gk_editor.after_values()`).
3. **Generic instantiations (`instantiate_objects`, `multi_inst`, `after_scan`, `trait_summaries`)**:
   - Instantiates generic library objects listed under `[values].objects` in
     `api/overrides.toml` (e.g. `ScaleLinear<f64> as ScaleLinear`), generic element
     wrappers in `VALUES_INSTANCES` (`Scrollable<Div>`), and `[values].enum_nodes`.
   - Emits per-generic summary rows aggregating bound/bridged status across concrete
     instantiations so coverage accounting reflects what is actually usable.
4. **Closures, reactive cells, drag payloads, and trait values (`closure`, `bounded`, `fn_ctx`, `drag_closure`)**:
   - Lifts `Fn(&App) -> T` getters into `["cell", ...]` descriptors backed by
     `crate::values::cell` handles.
   - Substitutes generic drag type parameters (`DRAG_FNS`) to `crate::values::Drag` and
     `crate::rt::GkView`, and `ListDelegate`/`TableDelegate` parameters to our Rust shims.
   - Maps type-erased traits (`TRAIT_VALUES`, such as `AnySettingField`) to `traitval`
     dispatchers over all concrete implementing value nodes.
5. **Global field accessors, pointer constructors, and Rust dispatchers (`field_items`, `pointer_ctors`, `rust_dec`, `rust_mod`)**:
   - Synthesizes getters/setters (with `cx.refresh_windows()`) for public fields of
     reachable `Global` structs (like `Theme`) and `Rc::new`/`Arc::new` constructors for
     pointer-wrapped value nodes.
   - Emits `value_action`, `traitval_*`, `motion_wrap`, and `motion_trap` into `mod.rs`.
6. **Partial-enum coverage audit (`partials`, `_partial_walk`)**:
   - Reports every bound/bridged function whose argument or return enum omits one or more
     upstream variants, feeding the partial-coverage table in `docs/STATUS.md`.
"""
import json
import re

import gk_comp6
import gk_items
import rdj

SPEECH = "gpui_kit::component::speech::"
SPEECH_WHY = ("feature-off: the default library is built without speech (DESIGN.md decision 4): no "
              "recognizer or audio input exists in it, so an app cannot implement or drive one")
# Descriptor tags handled by gk_values that are legal inside call-op arguments (no Dx tape needed).
CALL_SAFE = {"fallible", "array", "civil", "arcslice", "cell", "xform", "txform"}
# Descriptor tags decoded only during view-tape building (require Dx or an active view context).
DEC_ONLY = {"viewany", "traitval", "dragview", "dragstyle"}
# GPUI core objects exposed as library handles along with the kit traits whose methods are bound onto them.
GPUI_OBJECTS = {"ScrollHandle": ("ScrollbarHandle",)}
# `jiff::civil` calendar/time types wired through hand-written Unwire/Wire impls in core/src/values.rs.
CIVIL = {"Date": "Civil_Date", "Time": "Civil_Time", "DateTime": "Civil_Date_Time", "Weekday": "Weekday"}
# Syntax for `[values].objects` entries in api/overrides.toml: `Generic<Arg1, Arg2> as GoName`.
OBJ_SPEC = re.compile(r"\s*(\w+)<(.+)>\s+as\s+(\w+)\s*")


def _m():
    import gk_model
    return gk_model


def resolved(m, ctx, t, out):
    """Match GPUI value types, AccessKit Role, jiff civil types, Result, HashMap, Rc/Arc, and instantiated objects.

    Invoked from `Model.desc()` for `resolved_path` types before standard kit struct/enum
    classification. Order matters:
    1. `gk_app.resolved` and `gk_comp6.resolved` take precedence for platform and comp6 types.
    2. Leaf GPUI motion transforms (`xform`, `txform`), `accesskit::Role`, `jiff::civil`,
       and outbound `std::time::Duration`.
    3. Outbound `Entity<State>` handles (only when `State` is a known non-object state,
       listed in `[instantiate].states`, or a concrete kit entity).
    4. Outbound `readback()` value structs (synthesizes `read:<path>` handles).
    5. `Range<Pixels|Rems>`, `FocusHandle`, `GPUI_OBJECTS` (`ScrollHandle`), `Option<&mut Window>`.
    6. Outbound `Result<(), E>` (overlay unit result) vs `Result<T, E>` (`fallible`),
       outbound `HashMap<K, V>` (wired as `Vec<(K, V)>`), and `Rc<T>` / `Arc<T>` objects.
    7. Concrete instantiated generic nodes (`m.nodes[rt]["inst"]`) and objects (`[values].objects`).
    """
    g = _m()
    import gk_app
    pr = gk_app.resolved(m, ctx, t, out)
    if pr is not None:
        return pr
    r6 = gk_comp6.resolved(m, ctx, t, out)
    if r6 is not None:
        return r6
    rp = t["resolved_path"]
    name, args = g.last(rp["path"]), g.path_args(rp)
    # GPUI motion transforms (input only, decoded by core/src/motion.rs).
    if name == "Transformation" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui" and not out:
        return ["xform"]
    if name == "TimedTransformation" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui" and not out:
        return ["txform"]
    # AccessKit accessibility role enum loaded from api/accesskit-role.json.
    if name == "Role" and not args and m.api.home(ctx.crate, rp["id"])[0] == "accesskit":
        return access_role(m)
    # jiff::civil date/time/weekday value types wired through core/src/values.rs.
    if name in CIVIL and not args and m.api.home(ctx.crate, rp["id"])[0] == "jiff":
        return ["civil", name]
    if out and name == "Duration" and not args and m.api.home(ctx.crate, rp["id"])[0] in g.STD:
        return ["leaf", "duration", ""]
    # Outbound Entity<State> handles: check already-registered states, [instantiate].states, or non-generic kit entities.
    if out and name == "Entity" and len(args) == 1 and "resolved_path" in args[0]:
        try:
            art = m.rtype(ctx, args[0])
        except g.Unsup:
            art = None
        named = {x.split(" as ")[-1].strip() for x in m.ov.get("instantiate", {}).get("states", [])}
        if art is not None and (art in m.states and not m.states[art].get("obj") or g.last(art) in named):
            return ["entity", art, g.last(art)]
        ahc, ahid, ait = m.api.item(ctx.crate, args[0]["resolved_path"]["id"])
        if ait is not None and ahc in g.KIT and m.info(ahc, ahid)["entity"] and not m.info(ahc, ahid)["generic"]:
            return ["entity", m.rtype(ctx, args[0]), g.last(rdj.ty(args[0]).split("<")[0])]
    # Cloneable value nodes returned from read methods become synthesized `read:<path>` object handles.
    if out and not args:
        rb = readback(m, ctx, rp)
        if rb:
            return rb
    if name == "Range" and len(args) == 1 and not out and m.api.home(ctx.crate, rp["id"])[0] in g.STD:
        d = m.desc(ctx, args[0])
        if d[0] == "leaf" and d[1] in ("px", "rems"):
            return ["rangeof", d]
    if out and name == "FocusHandle" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui":
        return ["handle", m.rtype(ctx, t), "FocusHandle"]
    if name in GPUI_OBJECTS and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui":
        return gpui_object(m, name)
    # Optional `&mut Window` parameter (`Option<&mut Window>`) passed as `Some(window)` on the Rust side.
    if name == "Option" and len(args) == 1 and not out:
        b = args[0].get("borrowed_ref")
        if b and b["is_mutable"] and "resolved_path" in b["type"] and g.last(b["type"]["resolved_path"]["path"]) == "Window":
            return ["ctx", "window_opt"]
    # Outbound Result<(), E> uses gk_overlay's `result` unit outcome; Result<T, E> uses `fallible`.
    if name == "Result" and out and args:
        ok = args[0]
        if "tuple" in ok and not ok["tuple"]:
            try:
                import gk_overlay
            except ImportError:
                raise g.Unsup("todo:protocol: calls (Result<(), E> is the overlays family's descriptor)", "Result<()>")
            return ["result", "unit"]
        return ["fallible", m.desc(ctx, ok, out)]
    # Outbound HashMap<K, V> serializes on the wire as a list of `(K, V)` pairs.
    if name == "HashMap" and out and len(args) == 2:
        k, v = m.desc(ctx, args[0], out), m.desc(ctx, args[1], out)
        return ["list", ["tuple", [k, v], f"({m.rtype(ctx, args[0])}, {m.rtype(ctx, args[1])})"], "map"]
    if name in ("Rc", "Arc") and len(args) == 1:
        a = args[0]
        if a.get("generic") == "Self" and ctx.self_t:
            ctx, a = ctx.at(ctx.self_t[0]), ctx.self_t[1]
        if "resolved_path" in a:
            return rc_object(m, ctx, name, a)
    # Concrete instantiated generic nodes (e.g. SidebarGroup<SidebarMenu>) and objects (e.g. ScaleLinear<f64>).
    if args and not out:
        try:
            rt = m.rtype(ctx, t)
        except g.Unsup:
            rt = None
        if rt is not None and rt in m.nodes and m.nodes[rt].get("inst"):
            return ["node", rt]
    if args and getattr(m, "objects", None):
        try:
            rt = m.rtype(ctx, t)
        except g.Unsup:
            return None
        st = m.objects.get(rt) or _declared(m).get(rt)
        if st is not None and st.get("generic"):
            return ["obj", rt, rt, st["clone"]]
    return None


ROLE_FILE = "accesskit-role.json"


def access_role(m):
    """Register gpui_kit::Role from api/accesskit-role.json as a unit enum descriptor."""
    import os
    rt = "gpui_kit::Role"
    if rt not in m.enums:
        path = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "api", ROLE_FILE)
        src = json.load(open(path))
        if src["path"] != rt:
            raise SystemExit(f"gk_values: {path} names {src['path']}, not {rt}")
        m.enums[rt] = {"kind": "enum", "rust": rt, "path": rt, "name": "Role", "crate": "gpui",
                       "variants": [{"name": v, "n": n, "shape": "unit", "fields": [], "ok": True}
                                    for n, v in enumerate(src["variants"])],
                       "in": True, "out": True, "constructible": True}
    return ["enum", rt]


def readback(m, ctx, rp):
    """Synthesize a `read:<path>` object handle for a Clone builder/value struct returned from a call.

    Used when a struct is normally built as a value node on the tape (private fields,
    `&self` getters) and an upstream read method returns an instance of it.
    """
    g = _m()
    hc, hid, it = m.api.item(ctx.crate, rp["id"])
    if it is None or hc not in g.KIT or (hc, hid) not in m.node_of:
        return None
    f = m.info(hc, hid)
    if f["kind"] != "struct" or f["generic"] or f["record"] or f["element"] or f["state"] or not f["clone"]:
        return None
    if m.is_obj(hc, hid) or not f["refs"]:
        return None
    sk = f["item"]["inner"]["struct"]["kind"]
    if isinstance(sk, dict) and "plain" in sk and any(
            (m.api.crates[hc].item(x) or {}).get("visibility") == "public" for x in sk["plain"]["fields"]):
        return None
    xp = m.api.pubpath(hc, hid)
    if xp is None:
        return None
    key = "read:" + xp
    if key not in m.objects:
        st = {"key": key, "path": xp, "name": f["name"] + "Read", "crate": hc, "ctors": [], "builders": [],
              "reads": [], "writes": [], "calls": [], "events": [], "globals": [], "statics": [], "obj": True,
              "glob": False, "clone": True, "readback": (hc, hid)}
        m.states[key] = st
        m.objects[key] = st
    return ["obj", xp, key, True]


def readback_reads(m):
    """Populate `&self` getter methods on every synthesized `read:*` readback object.

    Runs to a fixed point because binding a getter on one readback object can return
    another private-field value struct, registering a new `read:*` object in `m.objects`.
    Any getter returning an `iter` descriptor is downgraded to `todo` because readback
    objects are only queried through single-value call ops.
    """
    g = _m()
    done = set()
    while True:
        todo = [(k, st) for k, st in sorted(m.objects.items()) if st.get("readback") and k not in done]
        if not todo:
            return
        for key, st in todo:
            done.add(key)
            st["reachable"] = True
            hc, hid = st["readback"]
            f = m.info(hc, hid)
            c = m.api.crates[hc]
            ctx = g.Ctx(hc, (hc, {"resolved_path": {"path": f["name"], "id": int(hid), "args": None}}))
            row = {"name": st["name"], "path": key, "group": "state", "crate": hc, "items": []}
            for im in f["impls"]:
                if im.get("trait") is not None:
                    continue
                for mid in im["items"]:
                    fi = c.item(mid)
                    if not fi or "function" not in fi["inner"] or fi["visibility"] != "public":
                        continue
                    ins = fi["inner"]["function"]["sig"]["inputs"]
                    if not ins or ins[0][0] != "self" or rdj.ty(ins[0][1]) != "&Self":
                        continue
                    m.object_method(row, st, fi, hc, ctx)
            for it in row["items"]:
                it.update(synthetic=True, id=f"{key}::{it['name']}", row=None)
                if it["status"] in ("bound", "bridged") and any(x[0] == "iter" for x in g.walk_desc(it.get("ret") or ["unit"])):
                    it["status"], it["reason"] = "todo", "protocol: state (a read whose result the binding cannot carry yet)"
                    for grp in ("reads", "calls"):
                        if it in st[grp]:
                            st[grp].remove(it)
                if it["status"] in ("bound", "bridged"):
                    it["callee"] = f"<{st['path']}>::{it['name']}"
                    m.items.append(it)


def walk_tables(m, d):
    """Yield every descriptor reachable from `d`, following rec, denum, and into table definitions.

    Tracks visited `(tag, key)` pairs so recursive records or data enums terminate cleanly.
    Used by reachability checks (`handed_states`, `handed_state_types`, `ret_readable`).
    """
    g = _m()
    tables = {"rec": m.records, "denum": m.denums, "into": m.intos}
    seen, todo = set(), [d]
    while todo:
        for x in g.walk_desc(todo.pop()):
            yield x
            if x[0] in tables and len(x) > 1 and (x[0], x[1]) not in seen and x[1] in tables[x[0]]:
                seen.add((x[0], x[1]))
                t = tables[x[0]][x[1]]
                todo += [f["desc"] for f in t.get("fields", []) if isinstance(f, dict)]
                todo += [f["desc"] for v in t.get("variants", []) if isinstance(v, dict)
                         for f in v.get("fields", []) if isinstance(f, dict)]
                todo += [mem["desc"] for mem in t.get("members", []) if isinstance(mem, dict)]


def handed_states(m):
    """Downgrade return items that yield an `Entity<T>` whose state type has no Go handle.

    A state type `T` is considered held (`held`) only if it has at least one bound/bridged
    constructor or is explicitly marked `handed=True` (for example `Root` or dock panels
    created by the library). Any other `Entity<T>` would have no Go struct generated.
    """
    g = _m()
    why = "protocol: state (a read whose result the binding cannot carry yet)"
    held = {k for k, st in m.states.items() if not st.get("obj") and (st.get("handed") or any(
        i["status"] in ("bound", "bridged") for i in st["ctors"]))}
    for it in m.all_items():
        if it["status"] not in ("bound", "bridged") or not it.get("ret"):
            continue
        for x in walk_tables(m, it["ret"]):
            if x[0] == "entity" and x[1] not in held:
                it["status"], it["reason"], it["detail"] = "todo", why, f"Entity<{x[2]}>: no constructor of it is bound"
                for grp in ("reads", "calls", "statics"):
                    st = m.states.get(it.get("state", ""), {})
                    if it in st.get(grp, []):
                        st[grp].remove(it)
                break


def _inner_iter(d, top):
    """Return True if an `iter` descriptor appears nested beneath the top-level return descriptor."""
    if not isinstance(d, list) or not d:
        return False
    if d[0] == "iter" and not top:
        return True
    return any(_inner_iter(x, False) for x in d[1:] if isinstance(x, list))


def nested_iterators(m):
    """Downgrade return items whose descriptor contains a nested `iter` inside another descriptor.

    Wire serialization collects a top-level iterator into a length-prefixed list, which
    cannot nest arbitrarily without a dedicated codec. Direct `Iterator<Item = Iterator<...>>`
    is later revived by `gk_editor.after_values()`, while iterators nested inside records or
    options stay unbound.
    """
    g = _m()
    for it in m.all_items():
        r = it.get("ret")
        if it["status"] in ("bound", "bridged") and r and _inner_iter(r, True):
            it["status"], it["reason"] = "todo", "protocol: state (a read whose result the binding cannot carry yet)"
            it["detail"] = "an iterator of iterators"
            st = m.states.get(it.get("state", ""), {})
            for grp in ("reads", "calls", "statics"):
                if it in st.get(grp, []):
                    st[grp].remove(it)


def handed_state_types(m):
    """Return sorted keys of state types returned as `Entity<T>` or passed into event handler shapes.

    Used by Rust/Go emitters to ensure `Entity<T>` handle wrappers and read/write codecs are
    generated even for states constructed purely inside the library rather than by user code.
    """
    out = set()
    for it in m.all_items():
        if it["status"] in ("bound", "bridged") and it.get("ret"):
            out |= {x[1] for x in walk_tables(m, it["ret"]) if x[0] == "entity"}
    for sh in m.shapes.values():
        for d in sh["args"]:
            out |= {x[1] for x in walk_tables(m, d) if x[0] == "entity"}
    return sorted(k for k in out if "stem" in m.states.get(k, {}))


def gpui_object(m, name):
    """Register a core GPUI handle type (such as `ScrollHandle`) as a library object with a `new()` ctor.

    Because core GPUI types live outside `gpui_base`/`gpui_component`, they are not
    discovered by the kit crate walk; registering them on demand here lets any kit builder
    accept or return them and lets `gpui_object_traits()` attach kit extension traits.
    """
    path = f"gpui_kit::{name}"
    if path not in m.objects:
        st = {"key": path, "path": path, "name": name, "crate": "gpui", "ctors": [], "builders": [],
              "reads": [], "writes": [], "calls": [], "events": [], "globals": [], "statics": [], "obj": True,
              "glob": False, "clone": True, "gpui": name}
        it = {"row": None, "owner": path, "trait": None, "crate": "gpui", "synthetic": True, "name": "new",
              "id": f"{path}::new", "kind": "obj-ctor", "status": "bridged", "reason": "", "state": path,
              "args": [], "callee": f"{path}::new", "sig": f"fn new() -> {name}"}
        st["ctors"].append(it)
        m.items.append(it)
        m.states[path] = st
        m.objects[path] = st
    return ["obj", path, path, True]


def gpui_object_traits(m):
    """Bind kit trait methods (like `ScrollbarHandle`) onto registered GPUI objects.

    Constructs fully qualified `<gpui_kit::ScrollHandle as gpui_kit::base::ScrollbarHandle>::method`
    callees and records the resulting items in `m.inst_trait_items` so `trait_summaries()`
    credits the upstream trait row.
    """
    g = _m()
    for path, st in sorted(m.objects.items()):
        name = st.get("gpui")
        if not name or not st.get("reachable", True):
            continue
        for crate in g.KIT:
            for tid, (_, tr) in sorted(m.api.walks[crate].traits.items()):
                if tr["name"] not in GPUI_OBJECTS[name]:
                    continue
                c = m.api.crates[crate]
                ref = {"resolved_path": {"path": name, "id": 0, "args": None}, "verbatim": path}
                ctx = g.Ctx(crate, (crate, {"verbatim": path}))
                row = {"name": f"{name} as {tr['name']}", "path": path, "group": "state", "crate": crate, "items": []}
                for mid in tr["inner"]["trait"]["items"]:
                    fi = c.item(mid)
                    if fi and "function" in fi["inner"]:
                        m.object_method(row, st, fi, crate, ctx)
                tpath = m.api.pubpath(crate, tid)
                for it in row["items"]:
                    it["trait_of"] = tr["name"]
                    if it["status"] in ("bound", "bridged"):
                        it["callee"] = f"<{path} as {tpath}>::{it['name']}"
                        m.items.append(it)
                m.inst_trait_items = getattr(m, "inst_trait_items", []) + row["items"]
                _ = ref


def rc_object(m, ctx, wrap, inner):
    """Register `Rc<T>` or `Arc<T>` around a concrete kit struct as a Clone library object handle.

    Tags the object entry with `pointer=xp` so `pointer_ctors()` can later synthesize
    `Rc::new(value)` / `Arc::new(value)` when `T` is a constructible value node.
    """
    g = _m()
    hc, hid, it = m.api.item(ctx.crate, inner["resolved_path"]["id"])
    if it is None or hc not in g.KIT or g.path_args(inner["resolved_path"]):
        return None
    f = m.info(hc, hid)
    if f["kind"] != "struct" or f["generic"] or f["entity"]:
        return None
    xp = m.api.pubpath(hc, hid)
    if xp is None:
        return None
    rt = f"std::{'rc' if wrap == 'Rc' else 'sync'}::{wrap}<{xp}>"
    if rt not in m.objects:
        st = {"key": rt, "path": rt, "name": f["name"] + wrap, "crate": hc, "ctors": [], "builders": [],
              "reads": [], "writes": [], "calls": [], "events": [], "globals": [], "statics": [], "obj": True,
              "glob": False, "clone": True, "pointer": xp}
        m.states[rt] = st
        m.objects[rt] = st
    return ["obj", rt, rt, True]


def array(m, ctx, t, out):
    """Classify small fixed-size Rust arrays `[T; N]` (1 <= N <= 16) as `["array", elem_desc, N]`.

    Only constant integer lengths up to 16 are unrolled inline on the wire; larger or
    const-generic arrays are left unhandled.
    """
    a = t.get("array")
    if not a or not str(a.get("len", "")).isdigit():
        return None
    n = int(a["len"])
    if not 0 < n <= 16:
        return None
    return ["array", m.desc(ctx, a["type"], out), n]


def from_implementors(m, hc, im):
    """Expand a blanket `impl<T: Bound> From<T> for Target` across all concrete kit implementors.

    When an upstream `From` impl is generic over a single bounded type parameter (for example
    `impl<T: IntoElement> From<T> for ...`), enumerates every concrete non-generic kit type
    satisfying `Bound` so `gk_model.py` can build a closed Go union (`into` set) for `Target`.
    """
    g = _m()
    ps = [p for p in im["generics"]["params"] if "type" in p["kind"]]
    src = g.path_args(im["trait"])
    if len(ps) != 1 or len(src) != 1 or src[0].get("generic") != ps[0]["name"]:
        return []
    bounds = [n for n, _ in g.bound_parts(ps[0]["kind"]["type"]["bounds"])]
    if len(bounds) != 1:
        return []
    out = []
    for cr in g.KIT:
        for iid, (_, ty) in sorted(m.api.walks[cr].types.items()):
            f = m.info(cr, iid)
            has = bounds[0] in f["traits"] or bounds[0] == "IntoElement" and f["element"]
            if not has or f["generic"]:
                continue
            ref = {"resolved_path": {"path": ty["name"], "id": int(iid), "args": None}}
            try:
                sctx = g.Ctx(cr)
                out.append((m.desc(sctx, ref), m.rtype(sctx, ref)))
            except g.Unsup:
                continue
    return out


def self_generic_trait(m, crate, tr):
    """Return True if `tr` is a single-parameter trait blanket-implemented with `Trait<T> for T`.

    Used by `gk_model.py` to recognize self-parameterized traits (such as `StyleSized<T>` or
    similar extension traits) where the trait's type parameter equals `Self`.
    """
    g = _m()
    ps = [p for p in tr["generics"]["params"] if "type" in p["kind"]]
    if len(ps) != 1 or len(g.bound_parts(ps[0]["kind"]["type"]["bounds"])) != 1:
        return False
    p = ps[0]["name"]
    c = m.api.crates[crate]
    for i in tr.get("implementations", []):
        ii = c.item(i)
        im = ii and ii["inner"].get("impl")
        if not im:
            continue
        args = g.path_args(im["trait"]) if im.get("trait") else []
        if im["for"].get("generic") and len(args) == 1 and args[0].get("generic") == im["for"]["generic"]:
            return True
    return p == "T" and not tr.get("implementations")


def split_args(text):
    """Split a comma-separated generic argument list at top-level angle-bracket depth."""
    out, depth, cur = [], 0, ""
    for ch in text:
        if ch == "<":
            depth += 1
        elif ch == ">":
            depth -= 1
        if ch == "," and depth == 0:
            out.append(cur.strip())
            cur = ""
        else:
            cur += ch
    if cur.strip():
        out.append(cur.strip())
    return out


def inst_arg(m, crate, a):
    """Resolve a possibly nested generic type argument string from `[values].objects` into `(crate, rdoc_ty, rust_path)`.

    Extends `Model.inst_arg()` to handle nested angle-bracket arguments like `Option<Pixels>`
    or `ScaleLinear<f64>` recursively.
    """
    g = _m()
    mm = re.fullmatch(r"(\w+)<(.+)>", a.strip())
    if mm is None:
        return m.inst_arg(crate, a)
    found = [(cr, i) for cr in g.CRATES for i, (_, it) in m.api.walks[cr].types.items()
             if it["name"] == mm.group(1) and len(m.info(cr, i)["generic"]) == len(split_args(mm.group(2)))]
    if len(found) != 1:
        raise SystemExit(f"gkgen: overrides [instantiate] types: {mm.group(1)} names {len(found)} generic types")
    cr, iid = found[0]
    subs = [inst_arg(m, cr, x) for x in split_args(mm.group(2))]
    ref = {"resolved_path": {"path": mm.group(1), "id": int(iid),
                             "args": {"angle_bracketed": {"args": [{"type": r} for _, r, _ in subs], "constraints": []}}}}
    return cr, ref, f"{m.api.pubpath(cr, iid)}<{', '.join(rs for _, _, rs in subs)}>"


assert split_args("A<B<C>, D>, E") == ["A<B<C>, D>", "E"]


def _declared(m):
    """Lazily parse `[values].objects` into pre-declared object stubs so forward references resolve during `desc()`.

    Needed when one object's method signature mentions another instantiated generic object
    before `instantiate_objects()` has visited that second generic type.
    """
    if getattr(m, "values_declared", None) is None:
        g = _m()
        m.values_declared = {}
        for spec in m.ov.get("values", {}).get("objects", []):
            mm = OBJ_SPEC.fullmatch(spec)
            if mm is None:
                continue
            found = [(cr, i) for cr in g.KIT for i, (_, it) in m.api.walks[cr].types.items()
                     if it["name"] == mm.group(1) and m.info(cr, i)["generic"]]
            if len(found) != 1:
                continue
            cr, iid = found[0]
            f = m.info(cr, iid)
            rust = [inst_arg(m, cr, a)[2] for a in split_args(mm.group(2))]
            tpath = f"{m.api.pubpath(cr, iid)}<{', '.join(rust)}>"
            m.values_declared[tpath] = {"key": tpath, "path": tpath, "name": mm.group(3), "crate": cr, "ctors": [],
                                        "builders": [], "reads": [], "writes": [], "calls": [], "events": [],
                                        "globals": [], "statics": [], "obj": True, "glob": False,
                                        "clone": f["clone"], "generic": m.api.pubpath(cr, iid)}
    return m.values_declared


def object_specs(m, crate, iid):
    """Return `[(arg_strs, go_name), ...]` for every `[values].objects` override matching type `(crate, iid)`."""
    name = m.api.crates[crate].item(iid)["name"]
    out = []
    for spec in m.ov.get("values", {}).get("objects", []):
        mm = OBJ_SPEC.fullmatch(spec)
        if mm is None:
            raise SystemExit(f"gkgen: overrides [values] objects: not `Generic<Args> as Name`: {spec}")
        if mm.group(1) == name:
            m.ov_used.add("values.objects." + spec)
            out.append((split_args(mm.group(2)), mm.group(3)))
    return out


# Instantiated generic objects that lack an explicit `new()` constructor and need a synthesized `Default::default()` ctor.
DEFAULT_OBJECTS = {"gpui_kit::base::TransitionState<f32>"}


def instantiate_objects(m, row, f, c, crate, iid, specs):
    """Instantiate generic library object types listed under `[values].objects` and summarize methods.

    For each `(args, name)` spec:
    - Substitutes the generic parameters into `Ctx` and registers the concrete object in
      `m.states` and `m.objects`.
    - Walks inherent impls (filtered by `_holds` to match unconstrained generic impls) and
      parameterized non-Rust-only trait impls (recording `trait_of` and `<T as Trait<Args>>::m`
      callees for `trait_summaries()`).
    - Adds a synthetic `Default::default()` constructor if the path is in `DEFAULT_OBJECTS`.
    - Appends one summary item per inherent method name to `row` so `docs/STATUS.md` tracks
      the generic type once rather than once per instantiation.
    """
    g = _m()
    base_path = m.api.pubpath(crate, iid)
    shadows = []
    for args, name in specs:
        if len(args) != len(f["generic"]):
            raise SystemExit(f"gkgen: overrides [values] objects: {f['name']} takes {len(f['generic'])} arguments")
        subst, rust = {}, []
        for p, a in zip(f["generic"], args):
            cr, ref, rs = inst_arg(m, crate, a)
            subst[p] = (cr, ref)
            rust.append(rs)
        tpath = f"{base_path}<{', '.join(rust)}>"
        st = _declared(m).get(tpath)
        self_t = (crate, {"resolved_path": {"path": f["name"], "id": int(iid), "args": {
            "angle_bracketed": {"args": [{"type": subst[p][1]} for p in f["generic"]], "constraints": []}}}})
        ctx = g.Ctx(crate, self_t, subst)
        shadow = {"name": row["name"], "path": tpath, "group": "state", "crate": crate, "items": []}
        st = st or {"key": tpath, "path": tpath, "name": name, "crate": crate, "ctors": [], "builders": [],
                    "reads": [], "writes": [], "calls": [], "events": [], "globals": [], "statics": [], "obj": True,
                    "glob": False, "clone": f["clone"], "generic": base_path}
        m.states[tpath] = st
        m.objects[tpath] = st
        seen = set()
        for im in f["impls"]:
            tr = im.get("trait")
            if not _holds(im, f):
                continue
            items = im["items"]
            if tr is not None:
                if g.last(tr["path"]) in g.RUST_ONLY_TRAITS or not g.path_args(tr):
                    continue
                items = _trait_items(m, crate, tr)
            for mid in items:
                fi = c.item(mid) if not isinstance(mid, tuple) else mid[1]
                cr2 = crate if not isinstance(mid, tuple) else mid[0]
                if not fi or "function" not in fi["inner"] or fi["name"] in seen:
                    continue
                if tr is None and fi["visibility"] != "public":
                    continue
                seen.add(fi["name"])
                if tr is not None:
                    shadow_t = {"name": row["name"], "path": tpath, "group": "state", "crate": crate, "items": []}
                    m.object_method(shadow_t, st, fi, cr2, ctx)
                    tpath_t = m.api.pubpath(crate, tr["id"])
                    targs = ", ".join(m.rtype(ctx, a) for a in g.path_args(tr))
                    for it in shadow_t["items"]:
                        it["trait_of"] = g.last(tr["path"])
                        if it["status"] in ("bound", "bridged"):
                            it["callee"] = f"<{tpath} as {tpath_t}<{targs}>>::{it['name']}"
                    shadow["items"] += shadow_t["items"]
                    continue
                m.object_method(shadow, st, fi, crate, ctx)
        if tpath in DEFAULT_OBJECTS:
            import gk_lists
            gk_lists.default_ctor(m, st, f, crate)
        shadows.append(shadow)
    names = []
    for im in f["impls"]:
        if im.get("trait") is None:
            for mid in im["items"]:
                fi = c.item(mid)
                if fi and "function" in fi["inner"] and fi["visibility"] == "public" and fi["name"] not in names:
                    names.append(fi["name"])
    for fname in names:
        got = [it for sh in shadows for it in sh["items"] if it["name"] == fname and not it.get("trait_of")]
        _summary(row, crate, fname, got, f"{row['path']}::{fname}")
    m.inst_trait_items = getattr(m, "inst_trait_items", []) + [it for sh in shadows for it in sh["items"] if it.get("trait_of")]


def _holds(im, f):
    """Return True if impl block `im` applies to the unconstrained generic type `f` (all type args are generic params)."""
    g = _m()
    a = g.path_args(im["for"].get("resolved_path", {}))
    return len(a) == len(f["generic"]) and all("generic" in x for x in a)


def _trait_items(m, crate, tr):
    """Resolve the list of `(defining_crate, item)` entries declared on trait path `tr`."""
    hc, hid, it = m.api.item(crate, tr["id"])
    if it is None or "trait" not in it["inner"]:
        return []
    c = m.api.crates[hc]
    return [(hc, c.item(i)) for i in it["inner"]["trait"]["items"] if c.item(i)]


def _summary(row, crate, fname, got, iid):
    """Append a synthetic summary item to `row` picking the best bound/bridged status across concrete instantiations."""
    best = next((it for it in got if it["status"] in ("bound", "bridged")), got[0] if got else None)
    it = {"row": row["name"], "owner": row["path"], "name": fname, "trait": None, "kind": "summary",
          "id": iid, "args": [], "crate": crate, "summary": True,
          "sig": best["sig"] if best else "", "status": best["status"] if best else "todo",
          "reason": best["reason"] if best else "generic library type (needs an instantiation)",
          "detail": best.get("detail", "") if best else "",
          "also": [x["id"] for x in got if x["status"] in ("bound", "bridged")]}
    row["items"].append(it)


def trait_summaries(m):
    """Mark trait-table methods as `bridged` summary items when bound on instantiated generic objects."""
    got = getattr(m, "inst_trait_items", [])
    if not got:
        return
    by = {}
    for it in got:
        by.setdefault((it["trait_of"], it["name"]), []).append(it)
    for r in m.owners:
        if r["group"] != "trait":
            continue
        for it in r["items"]:
            if it["status"] in ("bound", "bridged"):
                continue
            hits = [x for x in by.get((r["path"].split("::")[-1], it["name"]), []) if x["status"] in ("bound", "bridged")]
            if hits:
                it.update(status="bridged", reason="", kind="summary",
                          detail="bound on the instantiated objects: " + ", ".join(sorted({x["state"] for x in hits})),
                          also=[x["id"] for x in hits])


# Type-erased value traits mapped to `(rust_wrapper_type, go_union_stem)`.
TRAIT_VALUES = {"AnySettingField": ("crate::values::AnyField", "Any_Setting_Field")}
# Closure payload structs ignored when lifting a fragment closure into a `fragment` descriptor
# (for example `SettingFieldElement` passes `&RenderOptions` alongside `(&mut Window, &mut App)`).
FRAGMENT_PAYLOADS = {"RenderOptions"}

# Generic element wrappers and blanket traits added by gk_values during scan/classify.
# `Scrollable<Div>` is instantiated as a concrete node (`ScrollableDiv`), and the blanket
# `ScrollableElement` trait is attached to `Div` so `Div.Scrollable(...)` is bound.
VALUES_INSTANCES = [("Scrollable", "gpui_kit::Div", "ScrollableDiv")]
VALUES_BLANKETS = [("ScrollableElement", "gpui_kit::Div")]


def _by_path(m, path):
    """Look up `(crate, item_id)` for a type by its exact public `gpui_kit::...` path."""
    for cr, w in m.api.walks.items():
        for iid in w.types:
            if m.api.pubpath(cr, iid) == path:
                return cr, iid
    return None


def after_scan(m):
    """Instantiate `VALUES_INSTANCES` wrappers (`Scrollable<Div>`) and `[values].enum_nodes` right after scanning."""
    g = _m()
    for wname, arg_path, name in VALUES_INSTANCES:
        found = [(cr, i) for cr in g.KIT for i, (_, it) in m.api.walks[cr].types.items()
                 if it["name"] == wname and len(m.info(cr, i)["generic"]) == 1]
        arg = _by_path(m, arg_path)
        if len(found) != 1 or arg is None:
            raise SystemExit(f"gk_values: VALUES_INSTANCES: {wname}<{arg_path}> names {len(found)} types")
        cr, iid = found[0]
        f = m.info(cr, iid)
        m.add_node(cr, iid, f, f"{m.api.pubpath(cr, iid)}<{arg_path}>", name,
                   {f["generic"][0]: (arg[0], {"resolved_path": {"path": arg_path.rsplit("::", 1)[-1], "id": int(arg[1]),
                                                                 "args": None}})}, inst=arg)
        m.nodes[f"{m.api.pubpath(cr, iid)}<{arg_path}>"]["values_inst"] = True
    for name in m.ov.get("values", {}).get("enum_nodes", []):
        found = [(cr, i) for cr in g.KIT for i, (_, it) in m.api.walks[cr].types.items()
                 if it["name"] == name and "enum" in it["inner"]]
        if len(found) != 1:
            raise SystemExit(f"gkgen: overrides [values] enum_nodes: {name} names {len(found)} enums")
        cr, iid = found[0]
        m.add_node(cr, iid, m.info(cr, iid), m.api.pubpath(cr, iid), name, {})


# Upstream items implemented via dedicated hand-written runtime shims rather than generated ops.
# `runtime_items()` marks these `bridged` (kind="shim") with their implementation note so coverage
# accounting knows where each lives.
RUNTIME_ITEMS = {
    "gpui_kit::component::TitleBar::window_options":
        "core/src/app.rs WINDOW_OPEN flag 32 (gk.Window_Config.kit_title_bar): the window opens with the kit's title bar options",
}
_M = "core/src/motion.rs (node ANIMATED, rt_motion.go): "
MOTION_ITEMS = {
    "gpui_kit::base::transition": _M + "gk.motion(.., gk.to_transition(..)): the property's value this frame",
    "gpui_kit::base::spring": _M + "gk.motion(.., gk.to_spring(..))",
    "gpui_kit::base::animate_keyframes": _M + "gk.motion(.., gk.at_keyframes(..))",
    "gpui_kit::base::Presence::sample": _M + "gk.motion(.., gk.at_presence(..)): its progress between two values",
    "gpui_kit::base::TransitionState::sample": _M + "gk.motion(.., gk.to_state(..))",
    "gpui_kit::base::animation::EffectTransition::apply": _M + "gk.effect(el, effect, id)",
    "gpui_kit::base::animation::cubic_bezier": _M + "gk.Easing{kind = .Cubic_Bezier, ..} of an animation",
    "gpui_kit::base::FocusTrapElement::focus_trap": _M + "gk.focus_trap(el, id, focus): Div and the other parent elements",
}


# Window-owned root state types constructed by the library rather than by app code.
WINDOW_ROOTS = {"gpui_kit::base::Root": "Root"}


def runtime_items(m):
    """Mark `RUNTIME_ITEMS`, `MOTION_ITEMS`, and `gk_app.runtime_items` as `bridged` shims."""
    for it in m.items:
        if it["id"] in RUNTIME_ITEMS and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail=RUNTIME_ITEMS[it["id"]])
    for it in m.items:
        if it["id"] in MOTION_ITEMS and it["status"] in ("todo", "norule", "skipped"):
            it.update(kind="shim", status="bridged", reason="", detail=MOTION_ITEMS[it["id"]])
    import gk_app
    gk_app.runtime_items(m)


def window_roots(m):
    """Mark `Root` as runtime-handed and bind `Root::read` as an ambient call returning `Option<Entity<Root>>`.

    Routes through `crate::values::window_root::<Root>` so Go apps can obtain the current
    window's `Root` entity handle without a user-callable `Root::new`.
    """
    for path, name in WINDOW_ROOTS.items():
        st = m.states.get(path)
        if st is None:
            raise SystemExit(f"gk_values: WINDOW_ROOTS: no state type {path}")
        st["handed"] = True
        for it in m.items:
            if it["id"] != f"{path}::read" or it["status"] not in ("todo", "norule"):
                continue
            it.update(kind="ambient", status="bound", reason="", detail="the window's own, made by the library",
                      stem_base=f"{_snake(name)}_read",
                      callee=f"crate::values::window_root::<{path}>", crate=st["crate"], borrowed_ret=False,
                      args=[{"name": "window", "desc": ["ctx", "window_ref"]}, {"name": "cx", "desc": ["ctx", "app_ref"]}],
                      ret=["opt", ["entity", path, name]])
            if not hasattr(m, "ambient"):
                m.ambient = []
            m.ambient.append(it)


def after_classify(m):
    """Run late classification hooks: window roots, runtime/motion shims, and `ScrollableElement` on `Div`."""
    window_roots(m)
    runtime_items(m)
    for tname, npath in VALUES_BLANKETS:
        node = m.nodes.get(npath)
        hit = [(cr, tid) for cr, w in m.api.walks.items() for tid, (_, t) in w.traits.items() if t["name"] == tname]
        if node is None or len(hit) != 1:
            raise SystemExit(f"gk_values: blanket table {tname} for {npath}: not found")
        if not any((tc, tid) == hit[0] for tc, tid, _ in node["traits"]):
            node["traits"].append((hit[0][0], hit[0][1], tname))


def multi_inst(m, crate, iid, f, c, rowname, tpath):
    """Classify a generic node type instantiated at multiple concrete type arguments (e.g. `SettingField<T>`).

    Each concrete instantiation gets its own shadow pass over matching `impl` blocks, and
    the generic owner row records summary items aggregating status across all instances.
    """
    g = _m()
    nodes = [m.nodes[k] for o, k in sorted(m.node_of.items(), key=lambda kv: kv[1])
             if len(o) == 3 and o[:2] == (crate, iid) and (o[2][0] == "inst" or m.nodes[k].get("values_inst"))]
    if len(nodes) < 2 and not any(n.get("values_inst") for n in nodes):
        return False
    row = m.row(rowname, tpath, "component" if nodes[0]["element"] else "builder", crate)
    row["multi"] = True
    shadows = []
    for node in nodes:
        shadow = {"name": row["name"], "path": node["path"], "group": row["group"], "crate": crate, "items": []}
        f2 = dict(f)
        f2["impls"] = [im for im in f["impls"] if m.impl_matches(im, node)]
        m.type_impls(shadow, node, f2, c, crate, g.Ctx(crate, m.inst_self(crate, iid, f, node), node["subst"]), "node")
        shadows.append(shadow)
    names = []
    for im in f["impls"]:
        if im.get("trait") is None:
            for mid in im["items"]:
                fi = c.item(mid)
                if fi and "function" in fi["inner"] and fi["visibility"] == "public" and fi["name"] not in names:
                    names.append(fi["name"])
    for fname in names:
        got = [it for sh in shadows for it in sh["items"] if it["name"] == fname]
        _summary(row, crate, fname, got, f"{tpath}::{fname}")
    return True


def _is_el(m, ctx, output):
    """Return True if closure return type `output` is `AnyElement`, `impl IntoElement`, or a generic bounded by `IntoElement`."""
    g = _m()
    if output is None:
        return False
    out_s = rdj.ty(output)
    if "impl_trait" in output and "IntoElement" in out_s or g.last(out_s) == "AnyElement":
        return True
    if "generic" in output and output["generic"] in ctx.bounds:
        return "IntoElement" in [n for n, _ in g.bound_parts(ctx.bounds[output["generic"]])]
    return False


def closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """Match comp6, drag, reactive `Cell` (`&App -> T`), and `RenderOptions` fragment closures.

    For `Cell` closures, rejects any return type containing handles, nodes, mutable refs,
    or actions because `crate::values::cell_read` evaluates the closure on demand from an
    immutable byte buffer without an active `Dx` view decoder.
    """
    g = _m()
    c6 = gk_comp6.closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload)
    if c6 is not None:
        return c6
    dc = drag_closure(m, ctx, inputs, output, wrap, params, ptypes, payload)
    if dc is not None:
        return dc
    if params == ["app_ref"] and output is not None and rdj.ty(output) != "()":
        try:
            d = m.desc(ctx, output)
            rt = m.rtype(ctx, output)
        except g.Unsup:
            return None
        if m.call_bad(d, False) is not None or any(x[0] in ("obj", "refmut", "entity", "handle", "node", "action")
                                                   for x in g.walk_desc(d)):
            return None
        return ["cell", d, wrap, ptypes, rt]
    if len(payload) == 1 and "window" in params and "app" in params and _is_el(m, ctx, output):
        b = payload[0].get("borrowed_ref")
        inner = b and b["type"].get("resolved_path")
        if inner and g.last(inner["path"]) in FRAGMENT_PAYLOADS and not b["is_mutable"]:
            return ["fragment", params, wrap, ptypes]
    return None


def bounded(m, ctx, name):
    """Match trait-bounded generic parameters handled by `gk_values` (`SettingFieldElement` and `TRAIT_VALUES`)."""
    if name == "SettingFieldElement":
        return ["fragment", ["arg", "window", "app"], "impl",
                ["&gpui_kit::component::setting::RenderOptions", "&mut gpui_kit::Window", "&mut gpui_kit::App"]]
    if name in TRAIT_VALUES:
        keys = trait_nodes(m, name)
        if keys:
            return ["traitval", name, keys]
    return None


def trait_nodes(m, name):
    """Return sorted keys of instantiated non-element value nodes implementing trait `name`."""
    return sorted(k for k, nd in m.nodes.items() if not nd["element"] and nd.get("inst")
                  and name in m.info(nd["crate"], nd["id"])["traits"])


def settle_cells(m):
    """Promote items taking `cell`/`traitval` descriptors to `bridged` and refresh multi-inst summaries.

    Runs after reachability/classification so `multi` summary rows reflect any late status
    changes (either promoted to `bridged` or downgraded to `todo` if all concrete instances
    became unreachable).
    """
    g = _m()
    for it in m.all_items():
        if it["status"] == "bound" and any(x[0] in ("cell", "traitval") for a in it.get("args", [])
                                           for x in g.walk_desc(a["desc"])):
            it["status"] = "bridged"
    by_id = {}
    for it in m.all_items():
        by_id.setdefault(it["id"], []).append(it)
    for r in m.owners:
        if not r.get("multi"):
            continue
        for it in r["items"]:
            live = [x for i in it.get("also", []) for x in by_id.get(i, []) if x["status"] in ("bound", "bridged")]
            if live:
                it["status"] = "bridged" if any(x["status"] == "bridged" for x in live) else "bound"
            elif it["status"] in ("bound", "bridged"):
                dead = [x for i in it.get("also", []) for x in by_id.get(i, [])]
                it["status"], it["reason"] = "todo", dead[0]["reason"] if dead else "opaque library object without a constructor the binding can drive"


# Drag-and-drop substitution table: maps generic drag methods to the type parameter names
# pinned to `crate::values::Drag` (payload) and `crate::rt::GkView` (drag preview view).
DRAG = "crate::values::Drag"
DRAG_FNS = {"on_drag": ("T", "W"), "on_drop": ("T",), "drag_over": ("S",), "on_drag_move": ("T",),
            "group_drag_over": ("S",), "external_drag_payload": ("T",),
            "resize_handle": ("T", "E")}
DRAG_VIEW = "crate::rt::GkView"
# Standard marker/derive traits ignored when checking whether a generic parameter is a pure `Into<T>` bound.
DERIVES = {"Clone", "Copy", "Default", "Debug", "PartialEq", "Eq", "Send", "Sync", "Sized", "Hash"}
# GPUI length traits whose `Into<Length>` generic bounds specialize cleanly to `Pixels`.
LENGTHS = {"DefiniteLength", "Length", "AbsoluteLength"}


_FN_NAMES = {}


def _fn_names(m):
    """Build and cache a reverse map from `id(fn_inner_dict)` -> function name across all loaded crates.

    Used by `fn_ctx()` and `fixed_param()` because `Model` passes the inner function dict
    rather than the outer rustdoc item wrapper.
    """
    if not _FN_NAMES:
        for c in m.api.crates.values():
            for i in c.idx.values():
                if isinstance(i, dict) and "function" in i.get("inner", {}):
                    _FN_NAMES[id(i["inner"]["function"])] = i.get("name")
    return _FN_NAMES


def fixed_param(fn, pname):
    """Return True if type parameter `pname` on `fn` is fixed by `DRAG_FNS` (suppresses generic-fn skip)."""
    name = _FN_NAMES.get(id(fn))
    return name in DRAG_FNS and pname in DRAG_FNS[name]


# Concrete Rust shim types substituted for upstream delegate trait parameters.
DELEGATE_SHIMS = {"ListDelegate": "crate::shim_rows::GkListDelegate",
                  "TableDelegate": "crate::shim_rows::GkTableDelegate"}


def fn_ctx(m, c, fn):
    """Bind known generic method type parameters (drag payloads, delegates, and simple `Into<T>` bounds).

    Extends context `c` with concrete substitutions before argument classification:
    - `DRAG_FNS` parameters: `Render`-bounded params bind to `GkView`, others to `Drag`.
    - `DELEGATE_SHIMS` parameters: bind to `GkListDelegate` / `GkTableDelegate`.
    - Single `Into<T>` (+ optional `DERIVES`) non-synthetic parameters: bind to `T`
      (or `Pixels` when `T` is one of `LENGTHS`), letting callers pass concrete values
      without needing generic monomorphization on the Rust side.
    """
    g = _m()
    name = _fn_names(m).get(id(fn))
    subst = None
    for p in fn["generics"]["params"]:
        if "type" not in p["kind"] or p["name"] in c.subst:
            continue
        pn = p["name"]
        parts = g.bound_parts(c.bounds.get(pn, []))
        names = [n for n, _ in parts]
        fix = None
        if name in DRAG_FNS and pn in DRAG_FNS[name] and not (set(names) - {"Render"}):
            fix = (c.crate, {"verbatim": DRAG_VIEW if "Render" in names else DRAG})
        elif len(names) == 1 and names[0] in DELEGATE_SHIMS and not p["kind"]["type"].get("is_synthetic"):
            fix = (c.crate, {"verbatim": DELEGATE_SHIMS[names[0]]})
        elif names.count("Into") == 1 and set(names) - {"Into"} <= DERIVES and not p["kind"]["type"].get("is_synthetic"):
            tr = next(t for n, t in parts if n == "Into")
            ta = g.path_args(tr)
            if len(ta) == 1 and "resolved_path" in ta[0] and not g.path_args(ta[0]["resolved_path"]):
                fix = (c.crate, ta[0])
                if g.last(ta[0]["resolved_path"]["path"]) in LENGTHS:
                    cr, ref, _ = m.inst_arg(c.crate, "Pixels")
                    fix = (cr, ref)
        if fix is not None:
            subst = dict(c.subst) if subst is None else subst
            subst[pn] = fix
    if subst is None:
        return c
    return g.Ctx(c.crate, c.self_t, subst, c.bounds)


def drag_closure(m, ctx, inputs, output, wrap, params, ptypes, payload):
    """Match drag preview (`-> Entity<GkView>`) and drag-over style (`(StyleRefinement, &Drag, ...) -> StyleRefinement`) closures."""
    g = _m()
    if output is None:
        return None
    try:
        rt = m.rtype(ctx, output)
    except g.Unsup:
        return None
    if rt == f"gpui_kit::Entity<{DRAG_VIEW}>" and "window" in params and "app" in params:
        return ["dragview", wrap, ptypes]
    if g.last(rt) == "StyleRefinement" and len(payload) == 2 and ptypes[1] == f"&{DRAG}" and \
            rdj.ty(payload[0]).endswith("StyleRefinement"):
        d = m.desc(ctx, payload[0])
        if d[0] == "node":
            return ["dragstyle", d[1], wrap, ptypes]
    return None


def field_items(m):
    """Synthesize getter and setter call ops for public fields on reachable `Global` structs (like `Theme`)."""
    g = _m()
    for key, st in sorted(m.objects.items()):
        if not st.get("glob") or not st.get("reachable"):
            continue
        found = [(cr, i) for cr in g.KIT for i, (_, it) in m.api.walks[cr].types.items()
                 if m.api.pubpath(cr, i) == st["path"]]
        if len(found) != 1:
            continue
        hc, hid = found[0]
        f = m.info(hc, hid)
        sk = f["item"]["inner"]["struct"]["kind"]
        if not isinstance(sk, dict) or "plain" not in sk:
            continue
        c = m.api.crates[hc]
        taken = {i["name"] for grp in ("reads", "writes", "calls", "globals", "statics", "ctors") for i in st.get(grp, [])}
        ctx = g.Ctx(hc, (hc, {"resolved_path": {"path": f["name"], "id": int(hid), "args": None}}))
        for fid in sk["plain"]["fields"]:
            fi = c.item(fid)
            if fi is None or fi.get("visibility") != "public":
                continue
            ty, name = fi["inner"]["struct_field"], fi["name"]
            base = {"row": None, "owner": st["path"], "trait": None, "crate": hc, "synthetic": True, "field": name,
                    "state": key, "status": "bridged", "reason": "", "sig": f"pub {name}: {rdj.ty(ty)}"}
            if name not in taken:
                try:
                    ret = m.desc(ctx, ty, out=True)
                    m.check_obj_ret(ret, rdj.ty(ty))
                    for x in g.walk_desc(ret):
                        if x[0] == "obj":
                            m.objects[x[2]]["reachable"] = True
                    it = dict(base, name=name, id=f"{st['path']}::{name}", kind="obj-read", args=[], ret=ret,
                              callee=f"<{st['path']}>::{name}")
                    st["reads"].append(it)
                    m.items.append(it)
                except g.Unsup:
                    pass
            if "set_" + name not in taken:
                try:
                    d = m.call_desc(ctx, ty)
                    if m.call_bad(d, False) is None and not any(
                            x[0] == "refmut" or x[0] == "obj" and not m.objects[x[2]].get("reachable") for x in g.walk_desc(d)):
                        it = dict(base, name="set_" + name, id=f"{st['path']}::set_{name}", kind="obj-write",
                                  args=[{"name": "value", "desc": d}], callee=f"<{st['path']}>::set_{name}", mut=True)
                        st["writes"].append(it)
                        m.items.append(it)
                except g.Unsup:
                    pass


def pointer_ctors(m):
    """Synthesize `Rc::new` / `Arc::new` constructors for pointer objects wrapping constructible value nodes."""
    g = _m()
    for key, st in sorted(m.objects.items()):
        xp = st.get("pointer")
        if not xp or st["ctors"]:
            continue
        found = [(cr, i) for cr in g.KIT for i, (_, it) in m.api.walks[cr].types.items() if m.api.pubpath(cr, i) == xp]
        if len(found) != 1:
            continue
        cr, iid = found[0]
        ref = {"resolved_path": {"path": xp.split("::")[-1], "id": int(iid), "args": None}}
        try:
            d = m.call_desc(g.Ctx(cr), ref)
        except g.Unsup:
            continue
        if d[0] != "node" or m.call_bad(d, False) is not None:
            continue
        if not any(i["status"] in ("bound", "bridged") for i in m.nodes[d[1]]["ctors"]):
            continue
        wrap = key.split("::")[2].split("<")[0]
        it = {"row": None, "owner": key, "trait": None, "crate": st["crate"], "synthetic": True, "name": "new",
              "id": f"{key}::new", "kind": "obj-ctor", "status": "bridged", "reason": "", "state": key,
              "args": [{"name": "value", "desc": d}], "callee": f"std::{'rc' if wrap == 'Rc' else 'sync'}::{wrap}::new",
              "sig": f"fn new(value: {xp}) -> {wrap}<{xp}>"}
        st["ctors"].append(it)
        st["reachable"] = True
        m.items.append(it)


def field_expr(it):
    """Return the Rust field-read expression for a synthesized global field getter (`s.field.clone()` or `&s.field`)."""
    if not it.get("field"):
        return None
    return f"s.{it['field']}.clone()" if it["ret"][0] == "obj" else f"&s.{it['field']}"


def field_set(it, passed):
    """Return the Rust field-write block for a synthesized global field setter, refreshing all windows."""
    if not it.get("field"):
        return None
    return f"{{ s.{it['field']} = {passed}; cx.refresh_windows(); }}"


def post(m):
    """Run late value passes in strict dependency order before Rust/Go emission.

    Order invariants:
    1. `gk_comp6.post` creates comp6 objects/snaps first so later passes see them.
    2. `field_items` (on globals) and `pointer_ctors` (on `Rc`/`Arc` objects) may mark
       additional nested objects reachable.
    3. `gpui_object_traits` and `readback_reads` attach methods to reachable GPUI and
       readback objects.
    4. `handed_states` and `nested_iterators` prune invalid return items before
       `trait_summaries`, `settle_cells`, and `out_flags` freeze final reachability.
    5. Unused `[values]` override entries fail generation loudly so stale config is caught.
    """
    g = _m()
    gk_comp6.post(m)
    field_items(m)
    pointer_ctors(m)
    gpui_object_traits(m)
    readback_reads(m)
    handed_states(m)
    nested_iterators(m)
    for it in m.all_items():
        if it["id"].startswith(SPEECH) and it["status"] in ("todo", "norule"):
            m.set(it, g.Unsup("skip:feature-off", SPEECH_WHY))
    for r in m.owners:
        if r["path"].startswith(SPEECH):
            for it in r["items"]:
                if it["status"] in ("todo", "norule"):
                    m.set(it, g.Unsup("skip:feature-off", SPEECH_WHY))
    trait_summaries(m)
    settle_cells(m)
    out_flags(m)
    for sig in m.ov.get("values", {}).get("event_closures", []):
        if "values.event_closures." + sig not in m.ov_used:
            raise SystemExit(f"gkgen: overrides [values] event_closures names no closure: {sig}")
    for spec in m.ov.get("values", {}).get("objects", []):
        if "values.objects." + spec not in m.ov_used:
            raise SystemExit(f"gkgen: overrides [values] objects names no generic type: {spec}")
    gk_comp6.post_late(m)


def out_flags(m):
    """Clear `out=True` on records and data enums that are never read out and contain non-outbound fields.

    During classification, speculative `desc(..., out=True)` probes can set `out=True` on a
    record or data enum whose fields include tape-only values (nodes, children, handlers,
    closures). Generating a Rust `Wire::put` impl for such a type would fail to compile, so
    this pass walks every actually-bound return type and handler payload and clears `out`
    on any unreached type that contains a non-serializable field.
    """
    g = _m()
    tables = {"rec": m.records, "denum": m.denums}
    live = [it for it in m.all_items() if it["status"] in ("bound", "bridged")]
    todo = [it.get("ret") for it in live if it.get("ret")] + [d for sh in m.shapes.values() for d in sh["args"]]
    seen = set()
    while todo:
        d = todo.pop()
        if not d or not isinstance(d, list):
            continue
        for x in g.walk_desc(d):
            if x[0] in tables and len(x) > 1 and (x[0], x[1]) not in seen and x[1] in tables[x[0]]:
                seen.add((x[0], x[1]))
                t = tables[x[0]][x[1]]
                todo += [f["desc"] for f in t.get("fields", [])]
                todo += [f["desc"] for v in t.get("variants", []) for f in v["fields"]]
            if x[0] == "into" and x[1] in m.intos:
                todo += [mem["desc"] for mem in m.intos[x[1]]["members"]]
    cannot = {"node", "child", "fragment", "build", "stylefn", "handler", "data", "field", "format", "shim", "rows"}
    for kind, table in tables.items():
        for rt, t in table.items():
            descs = [f["desc"] for f in t.get("fields", [])] + [f["desc"] for v in t.get("variants", []) if v.get("ok")
                                                                for f in v["fields"]]
            bad = any(x[0] in cannot for d in descs for x in g.walk_desc(d))
            if t.get("out") and (kind, rt) not in seen and bad:
                t["out"] = False


def event_closure(m, ctx, kind, ptypes, output, params, payload, wrap):
    """Match zero-payload string-returning closures declared under `[values].event_closures`.

    Registers a `[[], "str"]` handler shape so Rust can invoke the Go callback synchronously
    and receive a `SharedString` reply back over the event channel.
    """
    g = _m()
    if output is None or payload:
        return None
    try:
        sig = f"{kind}({', '.join(ptypes)}) -> {m.rtype(ctx, output)}"
    except g.Unsup:
        return None
    if sig not in m.ov.get("values", {}).get("event_closures", []):
        return None
    m.ov_used.add("values.event_closures." + sig)
    if m.rtype(ctx, output) != "gpui_kit::SharedString" or "window" not in params:
        raise SystemExit(f"gkgen: overrides [values] event_closures: only Fn(&mut Window, &mut App) -> SharedString: {sig}")
    key = json.dumps([[], "str"])
    m.shapes.setdefault(key, {"key": key, "args": [], "ret": "str", "rust": [], "builtin": None})
    return ["handler", key, params, wrap, ptypes, []]


def static_call(m, it, crate, fn, self_t=None):
    """Delegate a static associated function with no `Self` return to `gk_app.ambient()`."""
    import gk_app
    return gk_app.ambient(m, it, crate, fn, None, self_t=self_t)


def rust_dec(rs, d):
    """Emit Rust decoder expression for array, civil, viewany, cell, drag, traitval, and motion descriptors.

    Note on `cell` handle lookup modes (`rs.call`):
    - Inside a `builder` or `codec` (`Option`-returning): `cell(id).ok()?`.
    - Inside a synchronous call op (`Result`-returning): `cell(id)?`.
    - Inside a view-tape decoder (where the frame keeps cells pinned): `cell_shown(id)`.
    """
    if d[0] == "array":
        return "[" + ", ".join(rs.dec(d[1]) for _ in range(d[2])) + "]"
    if d[0] == "civil":
        return f"<jiff::civil::{d[1]} as genapi::Unwire>::get(r)"
    if d[0] == "viewany":
        return gk_comp6.rust_viewany(rs, d)
    if d[0] == "cell":
        if rs.call in ("builder", "codec"):
            get = "crate::values::cell(r.u64()).ok()?"
        elif rs.call:
            get = "crate::values::cell(r.u64())?"
        else:
            get = "crate::values::cell_shown(r.u64())"
        ps = rs.closure_params(["arg"], d[3])
        return rs.wrap(d[2], "{ let cell = %s; move |%s| -> %s { crate::values::cell_read(&cell, |r| %s) } }"
                       % (get, ps, d[4], rs.dec(d[1])))
    if d[0] == "dragview":
        ps = rs.closure_params(["arg", "arg", "window", "app"], d[2])
        return rs.wrap(d[1], "{ let v = crate::values::drag_view_of(r.u64(), dx); move |%s| -> gpui_kit::Entity<%s> "
                       "{ crate::values::drag_view(&v, cx) } }" % (ps, DRAG_VIEW))
    if d[0] == "dragstyle":
        ps = re.sub(r"^_a0:", "v:", rs.closure_params(["arg", "arg", "window", "app"], d[3]))
        fn = rs.m.nodes[d[1]]["fn"]
        return rs.wrap(d[2], "{ let s = build_%s(dx, r.u32())?; move |%s| refined(v, &s) }" % (fn, ps))
    if d[0] == "traitval":
        return f"crate::generated::traitval_{_snake(d[1])}(dx, r.u32())?"
    if d[0] == "xform":
        return "crate::motion::transformation(r)"
    if d[0] == "txform":
        return "crate::motion::timed_transformation(r)"
    raise KeyError(d[0])


def _snake(name):
    return re.sub(r"(?<!^)(?=[A-Z])", "_", name).lower()


def rust_field_put(rc, f):
    """Emit the Rust `Wire::put` statement for record field `f`, handling `items`, `snap`, and bidirectional color fields."""
    ip = gk_items.field_put(rc, f)
    if ip:
        return ip
    sp = gk_comp6.snap_field_put(rc, f)
    if sp:
        return sp
    if rc["in"] and f["desc"][:2] == ["leaf", "color"]:
        # Bidirectional records use `Color` in Go rather than raw `Hsla`, so tag the color on write.
        return f"crate::values::put_color_field(&self.{f['name']}, out);"
    return f"genapi::Wire::put(&self.{f['name']}, out);"


def ret_readable(m, ret):
    """Return True if return descriptor `ret` contains only readable leaf/value types (no tape-only nodes/closures)."""
    for x in walk_tables(m, ret):
        if x[0] == "leaf" and x[1] not in ("px", "rems", "color"):
            return False
        if x[0] in ("node", "build", "handler", "fragment", "child", "stylefn"):
            return False
    return True


def rust_put_ret(ret, expr):
    """Emit Rust wire serialization for `fallible` (`Result<T, E>`) return descriptors."""
    if ret[0] == "fallible" and ret[1][0] == "obj":
        return f"genapi::Wire::put(&crate::values::Fallible(({expr}).map(genapi::Owned::new)), &mut out)"
    if ret[0] == "fallible":
        return f"genapi::Wire::put(&crate::values::Fallible({expr}), &mut out)"
    return None


def action_nodes(m):
    """Return sorted keys of non-element, non-instantiated value nodes implementing `gpui::Action`."""
    out = []
    for key in sorted(m.nodes):
        n = m.nodes[key]
        if n["element"] or n.get("inst"):
            continue
        if "Action" not in m.info(n["crate"], n["id"])["traits"]:
            continue
        if any(i["status"] in ("bound", "bridged") for i in n["ctors"]):
            out.append(key)
    return out


def rust_mod(rs, m):
    """Emit value_action, traitval_*, motion_wrap, motion_trap, and motion_scope for core/src/gen/mod.rs."""
    lines = ["/// Build a generated action value node into a boxed `gpui_kit::Action` (VALUE_ACTION).",
             "pub fn value_action(dx: &mut Dx, n: u32) -> Option<Box<dyn gpui_kit::Action>> {",
             "    match dx.node(n).0 {"]
    for key in action_nodes(m):
        node = m.nodes[key]
        nums = sorted(i["op"] for i in node["ctors"] if i["status"] in ("bound", "bridged"))
        lines.append(f"        {' | '.join(map(str, nums))} => Some(Box::new(build_{node['fn']}(dx, n)?)),")
    lines += ["        _ => None,", "    }", "}", ""]
    return lines + rust_traitvals(m) + motion_wrappers(m) + __import__("gk_app").rust_wrappers(m)


def animatable(m):
    """Return (styled_elements, focus_trap_elements) eligible for animation and trap wrappers."""
    out, trap = [], []
    for key in sorted(m.nodes):
        n = m.nodes[key]
        nums = sorted(i["op"] for i in n["ctors"] if i["status"] in ("bound", "bridged") and i.get("op") is not None)
        if not n["element"] or not n["styled"] or not nums or "fn" not in n:
            continue
        out.append((key, nums))
        traits = m.info(n["crate"], n["id"])["traits"] if n.get("inst") is None else ()
        if key == "gpui_kit::Div" or {"ParentElement", "Element"} <= set(traits):
            trap.append((key, nums))
    return out, trap


def motion_wrappers(m):
    """Emit motion_wrap and motion_trap dispatchers for animated and focus-trapped element nodes."""
    out, trap = animatable(m)
    lines = ["/// Wrap a styled generated element node with a motion or effect transition (ANIMATED).",
             "pub fn motion_wrap(dx: &mut Dx, n: u32, w: crate::motion::Wrap) -> Option<gpui_kit::AnyElement> {",
             "    let (ty, args, first) = dx.node(n);",
             "    Some(match ty {"]
    for key, nums in out:
        lines.append(f"        {' | '.join(map(str, nums))} => crate::motion::styled(make_{m.nodes[key]['fn']}(dx, ty, args, first)?, w),")
    lines += ["        _ => return None,", "    })", "}", "",
              "/// Wrap a generated parent element node with `FocusTrapElement::focus_trap` (ANIMATED kind 4).",
              "pub fn motion_trap(dx: &mut Dx, n: u32, id: gpui_kit::SharedString, focus: &gpui_kit::FocusHandle) -> Option<gpui_kit::AnyElement> {",
              "    use gpui_kit::base::FocusTrapElement as _;",
              "    use gpui_kit::IntoElement as _;",
              "    let (ty, args, first) = dx.node(n);",
              "    Some(match ty {"]
    for key, nums in trap:
        lines.append(f"        {' | '.join(map(str, nums))} => make_{m.nodes[key]['fn']}(dx, ty, args, first)?.focus_trap(id, focus).into_any_element(),")
    lines += ["        _ => return None,", "    })", "}", ""]
    return lines


def rust_traitvals(m):
    """Emit type-erased trait-value decoders (such as traitval_any_setting_field)."""
    lines = []
    for name, (wrapper, _) in sorted(TRAIT_VALUES.items()):
        lines += [f"/// Build a generated value node implementing `{name}` and wrap it in `{wrapper}`.",
                  f"pub fn traitval_{_snake(name)}(dx: &mut Dx, n: u32) -> Option<{wrapper}> {{",
                  "    match dx.node(n).0 {"]
        for key in trait_nodes(m, name):
            node = m.nodes[key]
            nums = sorted(i["op"] for i in node["ctors"] if i["status"] in ("bound", "bridged"))
            if nums:
                lines.append(f"        {' | '.join(map(str, nums))} => Some({wrapper}::new(build_{node['fn']}(dx, n)?)),")
        lines += ["        _ => None,", "    }", "}", ""]
    return lines


def describe(m):
    return {"actions": action_nodes(m),
            "objects": sorted(k for k, st in m.objects.items() if st.get("generic") or st.get("pointer"))}


def _partial_walk(m, d, out, seen, path):
    """Walk descriptor `d` and yield `(enum_path, missing_variants, direction, into_notes)` for partial enums."""
    if not isinstance(d, (list, tuple)) or not d or not isinstance(d[0], str):
        return
    k = d[0]
    if k in ("enum", "denum"):
        e = (m.enums if k == "enum" else m.denums).get(d[1])
        if e is None or (k, d[1], out) in seen:
            return
        seen.add((k, d[1], out))
        miss = [v["name"] for v in e["variants"] if not v["ok"]]
        # Held inbound enums (like BlockNode) can still pass any variant back by handle, so don't flag them.
        if miss and not out and d[1] in getattr(m, "_held_from", {}):
            miss = []
        if miss:
            yield d[1], miss, "out" if out else "in", path
        for v in e["variants"]:
            for f in v["fields"] if v["ok"] else ():
                yield from _partial_walk(m, f["desc"], out, seen, path)
        return
    if k == "rec":
        r = m.records.get(d[1])
        if r is None or ("rec", d[1], out) in seen:
            return
        seen.add(("rec", d[1], out))
        for f in r["fields"]:
            yield from _partial_walk(m, f["desc"], out, seen, path)
        return
    if k == "into":
        s = m.intos.get(d[1])
        if s is None:
            return
        others = [x.get("from") or "" for x in s["members"] if x["desc"][0] not in ("enum", "denum") or x["desc"][1] != d[1]]
        for x in s["members"]:
            yield from _partial_walk(m, x["desc"], out, seen, path + ([f"Into set, also: {', '.join(o for o in others if o)}"]
                                                                      if others and x["desc"][1:2] == [d[1]] else []))
        return
    if k == "handler":
        for x in (d[5] if len(d) > 5 and isinstance(d[5], list) else []):
            yield from _partial_walk(m, x, True, seen, path)
        return
    for x in d[1:]:
        if isinstance(x, list):
            if x and isinstance(x[0], str):
                yield from _partial_walk(m, x, out, seen, path)
            else:
                for y in x:
                    yield from _partial_walk(m, y, out, seen, path)


def _walk_held(d):
    if not isinstance(d, (list, tuple)) or not d:
        return
    if d[0] == "held" and len(d) > 1 and isinstance(d[1], str):
        yield d[1]
        return
    for x in d[1:]:
        if isinstance(x, (list, tuple)):
            yield from _walk_held(x)


def partials(m):
    """Collect bound/bridged items whose argument or return enums omit one or more upstream variants."""
    import gkgen
    m._held_from = {}
    for it in m.all_items():
        if it["status"] in ("bound", "bridged"):
            for x in _walk_held(it.get("ret")):
                if m.denums.get(x, {}).get("held"):
                    m._held_from.setdefault(x, []).append(it["id"])
    res = []
    for it in m.all_items():
        if it["status"] not in ("bound", "bridged"):
            continue
        got = {}
        for a in it.get("args") or []:
            for key, miss, way, via in _partial_walk(m, a.get("desc"), False, set(), []):
                got.setdefault((key, way), (miss, via))
        if it.get("ret"):
            for key, miss, way, via in _partial_walk(m, it["ret"], True, set(), []):
                got.setdefault((key, way), (miss, via))
        for (key, way), (miss, via) in sorted(got.items()):
            res.append({"id": it["id"], "family": gkgen.family(it.get("owner") or it["id"]), "enum": key,
                        "direction": way, "missing": miss, "note": "; ".join(via)})
    return res


def dump(m):
    return json.dumps({k: [i["id"] for i in st["ctors"] + st["reads"] + st["calls"] + st["writes"] + st["statics"]]
                       for k, st in m.objects.items() if st.get("generic")}, indent=1)


_ = rdj
