"""gk_values: the values family's generator rules (docs/PROTOCOLS.md sections 13.8 to 13.12). Called from gk_model.py, gk_rust.py and gk_go.py at small hooks, so the rules
live here and not in the shared files:

- Generic library objects instantiated for an argument list (`[values] objects` in
  api/overrides.toml, `"ScaleBand<SharedString> as ScaleBand"`): calculators an app calls between
  frames (the plot scales, the motion types). Each line is one library object type behind
  handles (section 13), every method of an impl that holds for the arguments classified as for any
  object, `Self` and the parameters substituted (`instantiate_objects`). A generic trait the type
  implements for its parameter (`Scale<T>`) gives its methods to the instance too.
- Results that travel to the app: `Result<T, E>` (`["fallible", T]`: ok, the value, the error's
  text; `Result<(), E>` is the overlays family's `["result", "unit"]`, the error's text or
  nothing), a `HashMap` (`["list", ["tuple", [K, V]], "map"]`, sorted by its bytes), `Rc<X>` and
  `Arc<X>` of a kit struct (a library object of its own, `["obj", "std::rc::Rc<X>", ..]`, cloned
  out of borrows), and a fixed array `[T; N]` both ways (`["array", T, N]`).
- `Option<&mut Window>`: the turn's window (`["ctx", "window_opt"]`).
- `impl Into<X>` where X has `impl<T: Trait> From<T> for X`: the kit's own implementors of Trait
  are the members of the Into set (`Icon` from every `IconNamed`, which is `IconName`).
- A generic trait whose parameter is the implementor itself (`StyleSized<T: Styled>`,
  implemented for every `T: Styled`): a builder trait like any other, bound `E: StyleSized<E>`.
- Static functions of a node type that return a value (`ThemeColor::light`): ambient calls
  (section 11.5).
- The public fields of a kit global (the theme): synthetic reads and writes (13.11); `Rc::new`
  for a pointer object; a text-returning closure upstream calls from an event (`[values]
  event_closures`: the clipboard's value); the out flags of records and enums no result reaches.
- Speech: the default library is built without the speech feature (DESIGN.md decision 4); the
  functions of `component::speech` that are not bound are skipped with that reason.

Standard library only."""
import json
import re

import gk_comp6  # the component rules (PROTOCOLS.md 11.17 to 11.23, 12.16)
import gk_items  # item procedures (PROTOCOLS.md 9.41)
import rdj

SPEECH = "gpui_kit::component::speech::"
SPEECH_WHY = ("feature-off: the default library is built without speech (DESIGN.md decision 4): no "
              "recognizer or audio input exists in it, so an app cannot implement or drive one")
# Descriptor kinds this module adds; a call op can carry each (gk_model.Model.call_bad).
CALL_SAFE = {"fallible", "array", "civil", "arcslice", "cell", "xform", "txform"}  # xform, txform: gpui's Transformation and TimedTransformation (13.30)  # arcslice: the lists family's Arc<[T]>, a list in a call op too
# Descriptor kinds this module adds that only an element's decoder can read (a call op cannot).
DEC_ONLY = {"viewany", "traitval", "dragview", "dragstyle"}
# jiff's calendar values (`["civil", Name]`), plain records both ways (PROTOCOLS.md 13.13): Rust's
# Wire and Unwire impls in core/src/values.rs; the Go types and their checked writers
# (`putCivilDate`: a date that does not exist stops) in go/gk/rt_gen_values.go.
# gpui types the app holds as library objects, with the kit traits whose methods they get
# (PROTOCOLS.md 13.14).
GPUI_OBJECTS = {"ScrollHandle": ("ScrollbarHandle",)}
CIVIL = {"Date": "Civil_Date", "Time": "Civil_Time", "DateTime": "Civil_Date_Time", "Weekday": "Weekday"}
OBJ_SPEC = re.compile(r"\s*(\w+)<(.+)>\s+as\s+(\w+)\s*")


def _m():
    import gk_model  # noqa: PLC0415 (gk_model imports this module)
    return gk_model


# ---------------------------------------------------------------- descriptors


def resolved(m, ctx, t, out):
    """A descriptor for a resolved path this module binds, or None (gk_model.Model.resolved)."""
    g = _m()
    import gk_app  # noqa: PLC0415
    pr = gk_app.resolved(m, ctx, t, out)
    if pr is not None:
        return pr  # image caches, scroll anchors, image sources, paths read back (PROTOCOLS.md 13.31)
    r6 = gk_comp6.resolved(m, ctx, t, out)
    if r6 is not None:
        return r6  # snapshots, Box<T> of a node, Background read back (PROTOCOLS.md 11.18, 11.22, 11.23)
    rp = t["resolved_path"]
    name, args = g.last(rp["path"]), g.path_args(rp)
    if name == "Transformation" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui" and not out:
        return ["xform"]  # an svg's scale, translation, rotation and alpha (gk.Transformation, PROTOCOLS.md 13.30)
    if name == "TimedTransformation" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui" and not out:
        return ["txform"]  # an svg's transformation over time (gk.Timed_Transformation, PROTOCOLS.md 13.30)
    if name == "Role" and not args and m.api.home(ctx.crate, rp["id"])[0] == "accesskit":
        return access_role(m)  # every accessibility role (api/accesskit-role.json, PROTOCOLS.md 13.30)
    if name in CIVIL and not args and m.api.home(ctx.crate, rp["id"])[0] == "jiff":
        return ["civil", name]  # a calendar date, a time of day or a weekday (PROTOCOLS.md 13.13)
    if out and name == "Duration" and not args and m.api.home(ctx.crate, rp["id"])[0] in g.STD:
        return ["leaf", "duration", ""]  # read back as nanoseconds, Go's time.Duration (PROTOCOLS.md 13.13)
    if out and name == "Entity" and len(args) == 1 and "resolved_path" in args[0]:
        try:
            art = m.rtype(ctx, args[0])
        except g.Unsup:
            art = None
        named = {x.split(" as ")[-1].strip() for x in m.ov.get("instantiate", {}).get("states", [])}
        if art is not None and (art in m.states and not m.states[art].get("obj") or g.last(art) in named):
            # a state type instantiated under an alias (`InputState = InputBaseState<InputMode>`, 13.21)
            return ["entity", art, g.last(art)]
        ahc, ahid, ait = m.api.item(ctx.crate, args[0]["resolved_path"]["id"])
        if ait is not None and ahc in g.KIT and m.info(ahc, ahid)["entity"] and not m.info(ahc, ahid)["generic"]:
            # kit state a read hands out: a handle to it, one per entity (PROTOCOLS.md 13.16)
            return ["entity", m.rtype(ctx, args[0]), g.last(rdj.ty(args[0]).split("<")[0])]
    if out and not args:
        rb = readback(m, ctx, rp)
        if rb:
            return rb  # a kit value read back: a copy behind a handle, with its reads (PROTOCOLS.md 13.15)
    if name == "Range" and len(args) == 1 and not out and m.api.home(ctx.crate, rp["id"])[0] in g.STD:
        d = m.desc(ctx, args[0])
        if d[0] == "leaf" and d[1] in ("px", "rems"):
            return ["rangeof", d]  # Range<Pixels>: two lengths (the lists family's descriptor, PROTOCOLS.md 13.19)
    if out and name == "FocusHandle" and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui":
        # a focus handle read back (`QuestionnaireState::focus_handle`): a handle of kind FOCUS (13.21)
        return ["handle", m.rtype(ctx, t), "FocusHandle"]
    if name in GPUI_OBJECTS and not args and m.api.home(ctx.crate, rp["id"])[0] == "gpui":
        return gpui_object(m, name)  # gpui's ScrollHandle, a library object (PROTOCOLS.md 13.14)
    if name == "Option" and len(args) == 1 and not out:
        b = args[0].get("borrowed_ref")
        if b and b["is_mutable"] and "resolved_path" in b["type"] and g.last(b["type"]["resolved_path"]["path"]) == "Window":
            return ["ctx", "window_opt"]
    if name == "Result" and out and args:
        ok = args[0]
        if "tuple" in ok and not ok["tuple"]:
            try:
                import gk_overlay  # noqa: F401, PLC0415 (the overlays family's descriptor, 11.6)
            except ImportError:
                raise g.Unsup("todo:protocol: calls (Result<(), E> is the overlays family's descriptor)", "Result<()>")
            return ["result", "unit"]  # the error's text or nothing
        return ["fallible", m.desc(ctx, ok, out)]
    if name == "HashMap" and out and len(args) == 2:
        k, v = m.desc(ctx, args[0], out), m.desc(ctx, args[1], out)
        return ["list", ["tuple", [k, v], f"({m.rtype(ctx, args[0])}, {m.rtype(ctx, args[1])})"], "map"]
    if name in ("Rc", "Arc") and len(args) == 1:
        a = args[0]
        if a.get("generic") == "Self" and ctx.self_t:
            ctx, a = ctx.at(ctx.self_t[0]), ctx.self_t[1]  # Arc<Self> of a static (ThemeColor::dark)
        if "resolved_path" in a:
            return rc_object(m, ctx, name, a)
    if args and not out:
        # A generic type given the arguments of an `[instantiate] types` line (`resize_handle<T, E>`
        # returning ResizeHandle<Drag, GkView>): that node (PROTOCOLS.md 13.20).
        try:
            rt = m.rtype(ctx, t)
        except g.Unsup:
            rt = None
        if rt is not None and rt in m.nodes and m.nodes[rt].get("inst"):
            return ["node", rt]
    if args and getattr(m, "objects", None):
        # A generic type given the arguments of a `[values] objects` line: that object.
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
    """accesskit's Role (gpui re-exports it as gpui::Role; the kit's RoleOverride::Role holds one): a
    third-party enum outside the API extract, read from api/accesskit-role.json (its 182 variants in
    declaration order, from the crate source Cargo.lock pins). An ordinary enum of the binding from then
    on, both ways, so every role() takes every role."""
    import os  # noqa: PLC0415
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
    """A kit value type the app builds (a node that is not an element: `QuestionnaireAnswer`,
    `QuestionnaireItemDefinition`) read back from the library: it has no public fields, so it
    cannot travel as a record. It travels as a copy behind a temporary handle, a library object of
    its own named `XRead` in Go, whose methods are the type's `&self` reads (`answer.Choices(cx)`,
    PROTOCOLS.md 13.15). None for any other type. Only a Clone type can be copied out of a borrow."""
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
        return None  # read back as the record of its public fields (gk_model.Model.record)
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
    """The `&self` reads of every read-back value (readback), classified as the object's reads;
    a read may hand out another read-back value (`choices()` of an item definition), so this runs
    until no new one appears. The upstream functions keep their row and status (a read of a
    builder the app is writing, skipped): these are synthetic items of the object."""
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
                    # an iterator read (`Table::rows`): an object's read writes its result by Wire,
                    # which an iterator has not; left out
                    it["status"], it["reason"] = "todo", "protocol: state (a read whose result the binding cannot carry yet)"
                    for grp in ("reads", "calls"):
                        if it in st[grp]:
                            st[grp].remove(it)
                if it["status"] in ("bound", "bridged"):
                    it["callee"] = f"<{st['path']}>::{it['name']}"
                    m.items.append(it)


def walk_tables(m, d):
    """Every part of a descriptor, following records, enums with data and Into sets into their
    fields (gk_model.walk_desc stops at their names): an object inside a state's event enum
    (`QuestionnaireEvent::Submit(QuestionnaireSubmission)`) is handed out by the event."""
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
    """A read or an event that hands out kit state (`HslaSliders::hue -> &Entity<SliderState>`) is
    bound only when the binding holds that state type (it has a bound constructor, so a handle type
    and its methods exist): the others go back to not attempted."""
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
    if not isinstance(d, list) or not d:
        return False
    if d[0] == "iter" and not top:
        return True
    return any(_inner_iter(x, False) for x in d[1:] if isinstance(x, list))


def nested_iterators(m):
    """A result that is an iterator of iterators (`editor_host::Table::rows`) is collected only at
    its outer level (gk_lists.put_ret), so it cannot be written: not attempted."""
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
    """The state types a result or an event hands out (registered so rd_value reads them)."""
    out = set()
    for it in m.all_items():
        if it["status"] in ("bound", "bridged") and it.get("ret"):
            out |= {x[1] for x in walk_tables(m, it["ret"]) if x[0] == "entity"}
    for sh in m.shapes.values():
        for d in sh["args"]:
            out |= {x[1] for x in walk_tables(m, d) if x[0] == "entity"}
    return sorted(k for k in out if "stem" in m.states.get(k, {}))


def gpui_object(m, name):
    """A gpui type of `GPUI_OBJECTS` as a library object (PROTOCOLS.md 13.14): made by a synthetic
    `new` (a kept handle; gpui's own methods are not in the coverage table), always copied (it is an
    Rc inside: a copy names the same scroll position). Taken by `&ScrollHandle` arguments and, as
    the one implementor the binding holds, by a generic `H: ScrollbarHandle` (gk_app.bounded)."""
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
    """The methods of a kit trait a gpui object implements (`ScrollbarHandle` for ScrollHandle:
    `offset`, `set_offset`, `content_size`, ..), classified as the object's own and called through
    the trait; the trait's row counts them as bridged (trait_summaries)."""
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
    """`Rc<X>` / `Arc<X>` of a kit struct that is not generic: a library object of its own (a
    clone of the pointer), named `XRc` in Go. None when X is not such a struct."""
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
    """`[T; N]` with a literal N: N values of T (`["array", T, N]`). None otherwise."""
    a = t.get("array")
    if not a or not str(a.get("len", "")).isdigit():
        return None
    n = int(a["len"])
    if not 0 < n <= 16:
        return None
    return ["array", m.desc(ctx, a["type"], out), n]


def from_implementors(m, hc, im):
    """The members `impl<T: Trait> From<T> for X` gives an Into set: every non-generic kit type
    that implements Trait (a closed set in the library; an app type cannot reach the library)."""
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
            has = bounds[0] in f["traits"] or bounds[0] == "IntoElement" and f["element"]  # blanket (gk_model)
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
    """`trait StyleSized<T: Styled>`, implemented for every `T: Styled` as `impl<T: Styled>
    StyleSized<T> for T`: its parameter is the implementor, so its builders are bound like a
    plain builder trait's. True for such a trait (one parameter, bounded by one trait)."""
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


# ---------------------------------------------------------------- argument lists, nested


def split_args(text):
    """The top-level arguments of `A<B<C>, D>, E`: ["A<B<C>, D>", "E"]."""
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
    """gk_model.Model.inst_arg for an argument that is itself generic (`SidebarGroup<SidebarMenu>`):
    (crate, type ref with its arguments, Rust source). Plain arguments go to the model's rule."""
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


# ---------------------------------------------------------------- generic objects, instantiated


def _declared(m):
    """Every `[values] objects` line's object, made before its type is classified: a method of
    one instance can hand out another (`Sequence::sample` gives a `SequenceSample`)."""
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
    """`[values] objects` lines that name this generic type: [(arguments, name)]."""
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


# Instantiated objects made by Default (their impl is generic, so the plain rule of PROTOCOLS.md 9.13
# does not see it): the TransitionState an app keeps with its data (13.30). Only these: Default of
# the other instances would make reads reachable that no codec carries yet.
DEFAULT_OBJECTS = {"gpui_kit::base::TransitionState<f32>"}


def instantiate_objects(m, row, f, c, crate, iid, specs):
    """One library object type per `Base<A, ..> as Name` line (PROTOCOLS.md 13.8). Methods of an
    inherent impl hold when the impl is generic over the parameters (`impl<T> ScaleBand<T>`);
    `Self` and the parameters are substituted. A generic trait the type implements for its own
    parameter (`impl<T> Scale<T> for ScaleBand<T>`) adds its methods as reads and calls. The
    coverage row keeps one item per upstream function, bridged when some instance binds it."""
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
                        if it["status"] in ("bound", "bridged"):  # a trait method, called through the trait
                            it["callee"] = f"<{tpath} as {tpath_t}<{targs}>>::{it['name']}"
                    shadow["items"] += shadow_t["items"]
                    continue
                m.object_method(shadow, st, fi, crate, ctx)
        if tpath in DEFAULT_OBJECTS:  # Default as the constructor (PROTOCOLS.md 9.13, 13.30)
            import gk_lists  # noqa: PLC0415
            gk_lists.default_ctor(m, st, f, crate)
        shadows.append(shadow)
    # The row: one item per upstream inherent function.
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
    """An impl of the generic type that holds for any arguments: `impl<T> Base<T>` (parameters
    only, bounds left to rustc)."""
    g = _m()
    a = g.path_args(im["for"].get("resolved_path", {}))
    return len(a) == len(f["generic"]) and all("generic" in x for x in a)


def _trait_items(m, crate, tr):
    """[(crate, item)] of the methods a trait declares (its provided and required methods)."""
    hc, hid, it = m.api.item(crate, tr["id"])
    if it is None or "trait" not in it["inner"]:
        return []
    c = m.api.crates[hc]
    return [(hc, c.item(i)) for i in it["inner"]["trait"]["items"] if c.item(i)]


def _summary(row, crate, fname, got, iid):
    best = next((it for it in got if it["status"] in ("bound", "bridged")), got[0] if got else None)
    it = {"row": row["name"], "owner": row["path"], "name": fname, "trait": None, "kind": "summary",
          "id": iid, "args": [], "crate": crate, "summary": True,
          "sig": best["sig"] if best else "", "status": best["status"] if best else "todo",
          "reason": best["reason"] if best else "generic library type (needs an instantiation)",
          "detail": best.get("detail", "") if best else "",
          "also": [x["id"] for x in got if x["status"] in ("bound", "bridged")]}
    row["items"].append(it)


def trait_summaries(m):
    """A generic trait's methods bound through instantiated objects (`Scale::tick` on the
    `ScaleBand` instance): the trait's own row counts them as bridged."""
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


# ---------------------------------------------------------------- settings: cells, fields, enum nodes (13.19)

# Kit traits whose implementors are value nodes the binding instantiates (`F: AnySettingField` is
# one of the `SettingField<T>` instances): the library's wrapper of the trait object, which upstream
# calls in place of the field (it delegates every method), made by a generated dispatcher on the
# node's type.
TRAIT_VALUES = {"AnySettingField": ("crate::values::AnyField", "Any_Setting_Field")}
# Payloads of an element closure that the closure's fragment does not receive (the app wrote them):
# a settings item's render options.
FRAGMENT_PAYLOADS = {"RenderOptions"}


# Generic wrappers a type-changing method of a kit trait returns, instantiated for one element type
# (as the overlays family's ContextMenu<Div>, PROTOCOLS.md 11.6): (wrapper's name, element path, node
# name). `div->overflow_y_scrollbar()` is a Scrollable<Div>, with its own builders and the tables of
# the traits it implements. (FocusTrapElement::focus_trap returns FocusTrapContainer<Self>, which has
# no public path: generated code cannot name it.)
VALUES_INSTANCES = [("Scrollable", "gpui_kit::Div", "ScrollableDiv")]
# Blanket traits entered into a node's tables (the extract lists a blanket impl only in the crate
# that declares the trait, or the impl for a gpui type is in the kit crate): (trait, node path).
VALUES_BLANKETS = [("ScrollableElement", "gpui_kit::Div")]


def _by_path(m, path):
    for cr, w in m.api.walks.items():
        for iid in w.types:
            if m.api.pubpath(cr, iid) == path:
                return cr, iid
    return None


def after_scan(m):
    """Enums that are value nodes (`[values] enum_nodes`: `SettingItem`, built by `new` and `render`
    and its builders): node types like a builder struct (gk_model.scan_types takes structs only).
    The wrappers of VALUES_INSTANCES, instantiated for their element type."""
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


# Upstream functions the runtime itself calls for the app, where no generated call fits: the item,
# where (counted bridged, PROTOCOLS.md 13.21).
RUNTIME_ITEMS = {
    "gpui_kit::component::TitleBar::window_options":
        "core/src/app.rs WINDOW_OPEN flag 32 (gk.Window_Config.kit_title_bar): the window opens with the kit's title bar options",
}
# Upstream functions the motion runtime calls while it draws what the app described (core/src/motion.rs,
# PROTOCOLS.md 13.30): counted bridged, each run by a gallery page compared with Rust (motion_pages).
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


# Entity types each window has one of, made by the library around the app's view: their static
# `read(window, cx) -> &Self` hands out the handle of the window's own (PROTOCOLS.md 13.21).
WINDOW_ROOTS = {"gpui_kit::base::Root": "Root"}


def runtime_items(m):
    for it in m.items:
        if it["id"] in RUNTIME_ITEMS and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail=RUNTIME_ITEMS[it["id"]])
    for it in m.items:  # the platform and motion families' (PROTOCOLS.md 11.30, 13.30)
        if it["id"] in MOTION_ITEMS and it["status"] in ("todo", "norule", "skipped"):
            it.update(kind="shim", status="bridged", reason="", detail=MOTION_ITEMS[it["id"]])
    import gk_app  # noqa: PLC0415
    gk_app.runtime_items(m)  # the platform family's PROTOCOLS.md 13.31


def window_roots(m):
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
    """Blanket trait tables (VALUES_BLANKETS), before the model settles (Model.__init__)."""
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
    """A generic builder instantiated by several `[instantiate] types` lines (`SettingField<bool>`,
    `<SharedString>`, `<f64>`): each instance is a node type of its own whose methods are those of
    the impls that hold for its arguments; the coverage row keeps one item per upstream function,
    bound when an instance binds it (as for objects, 13.8). False when the type has fewer than two."""
    g = _m()
    nodes = [m.nodes[k] for o, k in sorted(m.node_of.items(), key=lambda kv: kv[1])
             if len(o) == 3 and o[:2] == (crate, iid) and (o[2][0] == "inst" or m.nodes[k].get("values_inst"))]
    if len(nodes) < 2 and not any(n.get("values_inst") for n in nodes):
        return False  # one instance: the model's own rule (a wrapper of VALUES_INSTANCES is classified here)
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
    """gk_model.Model.closure_desc's hook (PROTOCOLS.md 13.19), None for other closures:

    - `Fn(&App) -> T`, a value upstream reads while it draws (a settings field's `value`, its
      `is_dirty`): a *value cell* the library holds, `["cell", T, wrap, ptypes, rust T]`. The app
      makes it (`gk.cell(cx, v)`), writes it between frames (`gk.cell_set`), reads it anywhere;
      the closure reads the cell's current bytes. T is a value a call op can carry.
    - an element closure whose one payload is in FRAGMENT_PAYLOADS (`Fn(&RenderOptions, &mut
      Window, &mut App) -> E`): a fragment of the view's tape; the payload is not passed on."""
    g = _m()
    c6 = gk_comp6.closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload)
    if c6 is not None:
        return c6  # optional element closures, sets of values (PROTOCOLS.md 11.19, 11.21)
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
    """gk_app.bounded's hook for this module: a generic parameter bounded by `SettingFieldElement`
    (implemented upstream for every `Fn(&RenderOptions, &mut Window, &mut App) -> E`) is that closure,
    a fragment; one bounded by a trait of TRAIT_VALUES is one of the value nodes implementing it."""
    if name == "SettingFieldElement":
        return ["fragment", ["arg", "window", "app"], "impl",
                ["&gpui_kit::component::setting::RenderOptions", "&mut gpui_kit::Window", "&mut gpui_kit::App"]]
    if name in TRAIT_VALUES:
        keys = trait_nodes(m, name)
        if keys:
            return ["traitval", name, keys]
    return None


def trait_nodes(m, name):
    return sorted(k for k, nd in m.nodes.items() if not nd["element"] and nd.get("inst")
                  and name in m.info(nd["crate"], nd["id"])["traits"])


def settle_cells(m):
    """A function taking a value cell or a trait value goes through the runtime: bridged. The rows of
    a builder instantiated several times (multi_inst) then follow their instances' final status
    (the pruning may have refused an instance's function after the row was summed up)."""
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


# ---------------------------------------------------------------- generic parameters fixed (13.20)

# The library's drag value (core/src/values.rs `Drag`, Go `gk.DragValue`): the one type the app
# drags. gpui's drag methods are generic over the dragged value's type (`on_drag<T, W>`, `on_drop<T>`,
# `drag_over<S>`, `on_drag_move<T>`); with T fixed to it, a drop is a handler whose payload is the
# value, a drag's preview is the app's view, a drag-over style is a described style.
DRAG = "crate::values::Drag"
DRAG_FNS = {"on_drag": ("T", "W"), "on_drop": ("T",), "drag_over": ("S",), "on_drag_move": ("T",),
            "group_drag_over": ("S",), "external_drag_payload": ("T",),  # the platform family, PROTOCOLS.md 13.31
            "resize_handle": ("T", "E")}  # the kit's resize handle drags a value too
DRAG_VIEW = "crate::rt::GkView"
# Bounds a parameter may have besides `Into<X>` and still be X (`L: Into<DefiniteLength> + Clone +
# Default + Debug + PartialEq` of StyledExt::margins): the derive traits every such X has.
DERIVES = {"Clone", "Copy", "Default", "Debug", "PartialEq", "Eq", "Send", "Sync", "Sized", "Hash"}
LENGTHS = {"DefiniteLength", "Length", "AbsoluteLength"}


_FN_NAMES = {}


def _fn_names(m):
    if not _FN_NAMES:
        for c in m.api.crates.values():
            for i in c.idx.values():
                if isinstance(i, dict) and "function" in i.get("inner", {}):
                    _FN_NAMES[id(i["inner"]["function"])] = i.get("name")
    return _FN_NAMES


def fixed_param(fn, pname):
    """Whether fn_ctx fixes this parameter (gk_app.bound_only: the caller does not choose it)."""
    name = _FN_NAMES.get(id(fn))
    return name in DRAG_FNS and pname in DRAG_FNS[name]


# Delegate traits the library implements once, over the app's row source (PROTOCOLS.md 9.5): a
# method generic over one is given the library's shim (the states of [instantiate] states are).
DELEGATE_SHIMS = {"ListDelegate": "crate::shim_rows::GkListDelegate",
                  "TableDelegate": "crate::shim_rows::GkTableDelegate"}


def fn_ctx(m, c, fn):
    """gk_model.Model.fn_ctx's hook: generic parameters the binding fixes, as substitutions.

    - the dragged value of gpui's drag methods (DRAG_FNS): the library's Drag, and the preview's
      view type the library's view (the app passes a gk.View);
    - a parameter bounded only by `Into<X>` and derive traits (DERIVES), X a type without
      parameters: X itself (`margins(Edges<DefiniteLength>)`);
    - a parameter bounded only by a delegate trait the library implements once (DELEGATE_SHIMS):
      that shim, as the states over it are instantiated (`Input::list<D: ListDelegate>(&Entity<
      ListState<D>>)` takes a gk.List_State)."""
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
                    # a length: pixels, which the wrappers it is given in have their From impls for
                    # (`Edges<Pixels>: From<f32>`; the model's Into sets read those for any argument)
                    cr, ref, _ = m.inst_arg(c.crate, "Pixels")
                    fix = (cr, ref)
        if fix is not None:
            subst = dict(c.subst) if subst is None else subst
            subst[pn] = fix
    if subst is None:
        return c
    return g.Ctx(c.crate, c.self_t, subst, c.bounds)


def drag_closure(m, ctx, inputs, output, wrap, params, ptypes, payload):
    """The two closures of the drag methods that are not handlers (DRAG_FNS):
    - the preview, `Fn(&Drag, Point<Pixels>, &mut Window, &mut App) -> Entity<GkView>`: the app's
      view (`["dragview", wrap, ptypes]`), handed back while the value is dragged;
    - a drag-over style, `Fn(StyleRefinement, &Drag, &mut Window, &mut App) -> StyleRefinement`: the
      described style refined over upstream's (`["dragstyle", style node, wrap, ptypes]`)."""
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


# ---------------------------------------------------------------- after classification


def field_items(m):
    """The public fields of a kit global (the theme: `colors`, `font_size`, `list`, `mode`): a read
    and a write each (PROTOCOLS.md 13.12), synthetic items not counted in the coverage table (they
    are not functions upstream) that take the values a Rust app assigns there (`ListSettings`,
    `ThemeColor`, `NotificationSettings`). A write marks every window for redraw."""
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
                            m.objects[x[2]]["reachable"] = True  # this read hands one out
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
    """`Rc<X>` / `Arc<X>` objects (13.10) the app can also make from a value X it built
    (`gk.theme_config_rc(cx, gk.theme_config())`): `Rc::new`, a kept handle."""
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
    """The Rust expression of a field read (gk_rust.Rust.obj_calls): a borrow, or a copy of an
    object (an `Rc<ThemeConfig>` moved into a new handle)."""
    if not it.get("field"):
        return None
    return f"s.{it['field']}.clone()" if it["ret"][0] == "obj" else f"&s.{it['field']}"


def field_set(it, passed):
    if not it.get("field"):
        return None
    return f"{{ s.{it['field']} = {passed}; cx.refresh_windows(); }}"


def post(m):
    """Statuses decided once every rule has run (gk_model.Model.settle, before the pruning)."""
    g = _m()
    gk_comp6.post(m)  # the component statuses (PROTOCOLS.md 11.17 to 11.23)
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
    gk_comp6.post_late(m)  # the component skips of read-back values (PROTOCOLS.md 11.28)


def out_flags(m):
    """A record or enum with data travels out (a Wire impl is generated for it) only when a bound
    function's result or a handler's payload reaches it. A result tried by a rule that then
    refused the function (an ambient call whose argument no call can carry) used to leave the flag
    set, and a Wire impl was generated for fields that cannot travel out (a node)."""
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
                t["out"] = False  # read out by nothing, and its fields could not be written out


def event_closure(m, ctx, kind, ptypes, output, params, payload, wrap):
    """A closure that upstream calls from an event, never while it draws, and that returns text
    (`Clipboard::value_fn`: called when the copy button is clicked): a handler of a shape that
    returns `str` (the app's procedure returns a string). Named by its signature in public paths
    in `[values] event_closures`; None for any other closure."""
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
    """A static function of a node type that returns a value or nothing (`ThemeColor::light()`,
    `Img::extensions()`): an ambient call (PROTOCOLS.md 11.5). True when bound."""
    import gk_app  # noqa: PLC0415
    return gk_app.ambient(m, it, crate, fn, None, self_t=self_t)


# ---------------------------------------------------------------- Rust


def rust_dec(rs, d):
    """The Rust expression that reads a value of one of this module's descriptors."""
    if d[0] == "array":
        return "[" + ", ".join(rs.dec(d[1]) for _ in range(d[2])) + "]"
    if d[0] == "civil":
        return f"<jiff::civil::{d[1]} as genapi::Unwire>::get(r)"
    if d[0] == "viewany":
        # A closure that returns a view (a tooltip): the app's view, mounted where the element is
        # decoded; none given draws an empty view (PROTOCOLS.md 13.18); or the kit's Tooltip (11.17).
        return gk_comp6.rust_viewany(rs, d)
    if d[0] == "cell":
        # A value cell (PROTOCOLS.md 13.19): the closure reads the cell's current bytes each time
        # upstream calls it. In a tape a released cell stops where it is drawn.
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
        # A drag's preview: the app's view, handed back while the value is dragged (13.20).
        ps = rs.closure_params(["arg", "arg", "window", "app"], d[2])
        return rs.wrap(d[1], "{ let v = crate::values::drag_view_of(r.u64(), dx); move |%s| -> gpui_kit::Entity<%s> "
                       "{ crate::values::drag_view(&v, cx) } }" % (ps, DRAG_VIEW))
    if d[0] == "dragstyle":
        # A style while the library's drag value is over the element: the described style (13.20).
        ps = re.sub(r"^_a0:", "v:", rs.closure_params(["arg", "arg", "window", "app"], d[3]))
        fn = rs.m.nodes[d[1]]["fn"]
        return rs.wrap(d[2], "{ let s = build_%s(dx, r.u32())?; move |%s| refined(v, &s) }" % (fn, ps))
    if d[0] == "traitval":
        # One of the value nodes implementing a kit trait: the library's wrapper of it (13.19).
        return f"crate::generated::traitval_{_snake(d[1])}(dx, r.u32())?"
    if d[0] == "xform":
        # gpui's Transformation (13.30): f32 scale x, y, translation x, y (pixels), rotation (radians),
        # alpha, read in that order (crate::motion::transformation).
        return "crate::motion::transformation(r)"
    if d[0] == "txform":
        # gpui's TimedTransformation (13.30): crate::motion::timed_transformation.
        return "crate::motion::timed_transformation(r)"
    raise KeyError(d[0])


def _snake(name):
    return re.sub(r"(?<!^)(?=[A-Z])", "_", name).lower()


def rust_field_put(rc, f):
    """A record field written out (gk_rust's record Wire impls). A colour field of a record the app
    also writes is a gk.Color in Go (theme u16, then h, s, l, a): it is written in that layout,
    theme 0 (PROTOCOLS.md 13.20: `box_shadow` read back and given to `shadow`)."""
    ip = gk_items.field_put(rc, f)
    if ip:
        return ip  # a field of an item procedure's record of reads (PROTOCOLS.md 9.41)
    sp = gk_comp6.snap_field_put(rc, f)
    if sp:
        return sp  # a snapshot record's field is its read (PROTOCOLS.md 11.18)
    if rc["in"] and f["desc"][:2] == ["leaf", "color"]:
        return f"crate::values::put_color_field(&self.{f['name']}, out);"
    return f"genapi::Wire::put(&self.{f['name']}, out);"


def ret_readable(m, ret):
    """A result whose records hold a colour can be read back: the record's Go field is a gk.Color
    when the app also writes the record (rust_field_put writes that layout then) and an Hsla when it
    only reads it (Hsla's own Wire impl), decided for both sides by the same flag."""
    for x in walk_tables(m, ret):
        if x[0] == "leaf" and x[1] not in ("px", "rems", "color"):
            return False
        if x[0] in ("node", "build", "handler", "fragment", "child", "stylefn"):
            return False
    return True


def rust_put_ret(ret, expr):
    """A call op's result of one of this module's descriptors, or None (gk_rust.Rust.put_ret).
    `Result<T, E>`: crate::values::Fallible writes u8 ok, opt<T>, str error (Display)."""
    if ret[0] == "fallible" and ret[1][0] == "obj":
        return f"genapi::Wire::put(&crate::values::Fallible(({expr}).map(genapi::Owned::new)), &mut out)"
    if ret[0] == "fallible":
        return f"genapi::Wire::put(&crate::values::Fallible({expr}), &mut out)"
    return None


# ---------------------------------------------------------------- value types


def action_nodes(m):
    """Value types whose type implements Action and that the app can make: `gk.ActionOf` takes
    them (PROTOCOLS.md 13.11)."""
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
    """`generated::value_action`: the value node `n` as a boxed action, by its constructors'
    numbers; None when its type is not an action (core/src/values.rs, VALUE_ACTION)."""
    lines = ["/// A kit action value as a boxed action (core/src/values.rs, call op VALUE_ACTION).",
             "pub fn value_action(dx: &mut Dx, n: u32) -> Option<Box<dyn gpui_kit::Action>> {",
             "    match dx.node(n).0 {"]
    for key in action_nodes(m):
        node = m.nodes[key]
        nums = sorted(i["op"] for i in node["ctors"] if i["status"] in ("bound", "bridged"))
        lines.append(f"        {' | '.join(map(str, nums))} => Some(Box::new(build_{node['fn']}(dx, n)?)),")
    lines += ["        _ => None,", "    }", "}", ""]
    return lines + rust_traitvals(m) + motion_wrappers(m) + __import__("gk_app").rust_wrappers(m)  # + text_selection_scope (13.31)


def animatable(m):
    """Element nodes an animation may wrap (PROTOCOLS.md 13.30): styled, with a constructor the
    binding makes, and a name in the package. With those that are also a parent and a gpui
    Element (focus_trap's bounds)."""
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
    """`generated::motion_wrap` and `motion_trap` (core/src/motion.rs): the type-changing wrappers
    of an element, one arm per element type, by its constructors' numbers: gpui's with_animation,
    the kit's EffectTransition::apply, motion values set while drawing, and focus_trap."""
    out, trap = animatable(m)
    lines = ["/// An element node wrapped by an animation (core/src/motion.rs, node ANIMATED).",
             "pub fn motion_wrap(dx: &mut Dx, n: u32, w: crate::motion::Wrap) -> Option<gpui_kit::AnyElement> {",
             "    let (ty, args, first) = dx.node(n);",
             "    Some(match ty {"]
    for key, nums in out:
        lines.append(f"        {' | '.join(map(str, nums))} => crate::motion::styled(make_{m.nodes[key]['fn']}(dx, ty, args, first)?, w),")
    lines += ["        _ => return None,", "    })", "}", "",
              "/// FocusTrapElement::focus_trap over an element node (core/src/motion.rs, node ANIMATED kind 4).",
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
    """`generated::traitval_<trait>`: the value node `n` as the library's wrapper of the trait, by its
    constructors' numbers (TRAIT_VALUES)."""
    lines = []
    for name, (wrapper, _) in sorted(TRAIT_VALUES.items()):
        lines += [f"/// One of the value nodes implementing {name}, as {wrapper} (PROTOCOLS.md 13.19).",
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
    """The values family's part of the interface description (api/gk.api.json.gz)."""
    return {"actions": action_nodes(m),
            "objects": sorted(k for k, st in m.objects.items() if st.get("generic") or st.get("pointer"))}


# ---------------------------------------------------------------- options only partly reachable (STATUS.md)


def _partial_walk(m, d, out, seen, path):
    """Yields (enum key, missing variant names, direction, via) for every enum inside a descriptor
    of which the binding builds (or reads) only some variants: through options, lists, records,
    the fields of other variants, the members of an Into set and a handler's parameters."""
    if not isinstance(d, (list, tuple)) or not d or not isinstance(d[0], str):
        return
    k = d[0]
    if k in ("enum", "denum"):
        e = (m.enums if k == "enum" else m.denums).get(d[1])
        if e is None or (k, d[1], out) in seen:
            return
        seen.add((k, d[1], out))
        miss = [v["name"] for v in e["variants"] if not v["ok"]]
        if miss and not out and d[1] in getattr(m, "_held_from", {}):
            miss = []  # every variant reaches it as a held value a bound function returned (PROTOCOLS.md 9.30, 9.38)
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
    if k == "handler":  # ["handler", key, params, wrap, ptypes, descs]: what the handler receives
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
    """The enum keys of the held values (`["held", key]`) inside a descriptor."""
    if not isinstance(d, (list, tuple)) or not d:
        return
    if d[0] == "held" and len(d) > 1 and isinstance(d[1], str):
        yield d[1]
        return
    for x in d[1:]:
        if isinstance(x, (list, tuple)):
            yield from _walk_held(x)


def partials(m):
    """Every bound or bridged function an option of which the binding cannot give or read: an
    argument (or a handler's payload, or a result) holding an enum of which some variants are not
    built. Listed in docs/STATUS.md's generated block (tools/status.py) by the family that owns
    the function."""
    import gkgen  # noqa: PLC0415 (the families of what remains)
    # A data enum read back as held values (gk_editor.held: BlockNode from parse_markdown) takes each
    # held value back as one more member, so a variant the app cannot build (BlockNode::Custom, which
    # wraps the parser's MarkdownNode) still reaches every function that takes the enum: counted given.
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
    """For gen/out: the instantiated objects and their methods (debugging aid)."""
    return json.dumps({k: [i["id"] for i in st["ctors"] + st["reads"] + st["calls"] + st["writes"] + st["statics"]]
                       for k, st in m.objects.items() if st.get("generic")}, indent=1)


_ = rdj
