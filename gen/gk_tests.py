"""The generated protocol tests (core/tests/gen, docs/PROTOCOLS.md section 8): one case per
handler shape and one per generated state type, written by `make gen` from the same model the
decoders and the package are written from, so a new shape or state type is tested the day the
generator first emits it.

Shapes. Every shape the package registers is fired once through the library (call op
TEST_FIRE_SHAPE) with a payload whose bytes are written here from the wire model, the model
the Rust `Wire` impls are emitted from. When the generated codecs can build that payload as
the Rust types a real listener hands over, the library decodes it into those types and fires
through their own `Wire` impls (typed); otherwise it fires the bytes as they are (raw). The
Odin handler, registered with `gk.on` for the payload type the package registered, encodes
what it received again: the test passes when those bytes are the bytes sent, the return value
arrives, and the typed flag is the one predicted here.

States. Every generated state type with a constructor this file can call: made (through its
builder chain, every builder once), every read, write and call once with fixed arguments,
every event subscribed; read again in a later turn, after the writes were applied; released.
"""
import struct

from gk_model import INT_WIRE

INT_FMT = {"u8": "B", "u16": "H", "u32": "I", "u64": "Q", "i8": "b", "i16": "h", "i32": "i", "i64": "q"}
# The built-in shapes' payload types as rt_handler.odin registers them (builtin_shapes).
BUILTIN_ODIN = {1: "gk.Click_Event", 2: None, 3: None, 4: "bool", 5: "int", 6: "gk.Temp_String",
                7: "gk.Key_Event", 8: "gk.Click_Event", 9: "gk.Mouse_Event", 10: "f64",
                20: "gk.Action"}  # 20: the app family's ACTION (rt_app.odin)


def decodable(m, d):
    """True when the generated codecs (the `_c` decoders of call ops) read this descriptor into
    the Rust type with the same bytes its `Wire` impl writes."""
    k = d[0]
    if k in ("bool", "f32", "f64"):
        return True
    if k == "int":
        return d[1] in INT_WIRE
    if k == "str":
        return len(d) < 2 or d[1].lstrip("&") in ("str", "asref", "String", "SharedString", "into")
    if k == "leaf":
        return d[1] == "px"  # a colour goes in as a Color (literal or theme) and out as Hsla
    if k == "enum":
        e = m.enums[d[1]]
        return bool(e["in"] and e["variants"])
    if k == "denum":
        e = m.denums[d[1]]
        v = pick_variant(m, e)
        return bool(e["in"] and e.get("callable") and v and v.get("in", True)
                     and all(decodable(m, f["desc"]) for f in v["fields"]))
    if k == "rec":
        rc = m.records[d[1]]
        return bool(rc["in"] and rc.get("callable") and all(decodable(m, f["desc"]) for f in rc["fields"]))
    if k in ("opt", "ref", "list"):
        return decodable(m, d[1])
    return False


def pick_variant(m, e):
    """The variant a sample takes: the last one the binding carries with data (so a tag that is
    off by one, or a field read in the wrong order, shows), else the last unit variant."""
    ok = [v for v in e["variants"] if v["ok"]]
    data = [v for v in ok if v["fields"]]
    typed = [v for v in data if v.get("in", True) and all(decodable(m, f["desc"]) for f in v["fields"])]
    if e["in"] and e.get("callable") and typed:
        return typed[-1]
    return (data or ok or [None])[-1]


class Bytes:
    """Sample payload bytes, value by value, as the Rust `Wire` impls write them."""

    def __init__(self, m):
        self.m, self.n = m, 0

    def next(self):
        self.n += 1
        return self.n

    def of(self, d, depth=0):
        m, k = self.m, d[0]
        if depth > 6:
            raise ValueError("too deep")
        if k == "bool":
            return b"\x01" if self.next() % 2 else b"\x00"
        if k == "int":
            w = INT_WIRE.get(d[1], d[1])
            return struct.pack("<" + INT_FMT[w], self.next() + (100 if w not in ("u8", "i8") else 0))
        if k == "f32":
            return struct.pack("<f", self.next() + 0.5)
        if k == "f64":
            return struct.pack("<d", self.next() + 0.25)
        if k == "str":
            s = f"s{self.next()}".encode()
            return struct.pack("<I", len(s)) + s
        if k == "leaf":
            n = self.next()
            if d[1] in ("px", "rems"):
                return struct.pack("<f", n + 0.5)
            if d[1] == "color":
                return struct.pack("<4f", 0.25, 0.5, 0.75, 1.0)
            if d[1] == "click_event":  # position, button, modifiers, click count, kind
                return struct.pack("<2fBBIB", 12.5, 7.25, 1, 5, 2, 0)
            if d[1] == "mouse_event":  # position, button, modifiers, click count
                return struct.pack("<2fBBI", 3.5, 9.0, 2, 1, 1)
            if d[1] == "key_event":  # key, key_char (an Option), modifiers, held
                return struct.pack("<I", 1) + b"k" + b"\x01" + struct.pack("<I", 1) + b"K" + b"\x04\x01"
            raise ValueError(f"leaf {d[1]}")
        if k == "enum":
            vs = m.enums[d[1]]["variants"]
            return struct.pack("<H", vs[-1]["num"])
        if k == "denum":
            v = pick_variant(m, m.denums[d[1]])
            if v is None:
                raise ValueError("no variant")
            return struct.pack("<H", v["num"]) + b"".join(self.of(f["desc"], depth + 1) for f in v["fields"])
        if k == "rec":
            return b"".join(self.of(f["desc"], depth + 1) for f in m.records[d[1]]["fields"])
        if k == "opt":
            return b"\x01" + self.of(d[1], depth + 1)
        if k in ("ref", "from"):
            return self.of(d[1], depth)
        if k == "list":
            return struct.pack("<I", 2) + self.of(d[1], depth + 1) + self.of(d[1], depth + 1)
        if k == "tuple":
            return b"".join(self.of(x, depth + 1) for x in d[1])
        if k == "action":  # an action's id (the app family's shape ACTION)
            return struct.pack("<I", 7)
        raise ValueError(k)


def shapes(m, od):
    """Every shape the package registers: number, Odin payload type (None: no payload), return,
    the Rust payload as written, sample bytes, and whether the library can fire it typed."""
    out = []
    for key in sorted(m.shapes, key=lambda k: m.shapes[k]["num"]):
        sh = m.shapes[key]
        num = sh["num"]
        if sh["builtin"]:
            T = BUILTIN_ODIN[num]
        elif len(sh["args"]) == 1:
            t = od.otype(sh["args"][0], True)
            T = "[]" + qualify(t[2:]) if t.startswith("[]") else qualify(t)
        else:
            T = f"gk.Event_{num}"
        try:
            bs = Bytes(m)  # one counter over all arguments, so two arguments of one type differ
            payload = b"".join(bs.of(a) for a in sh["args"])
        except ValueError as e:
            out.append({"num": num, "skip": str(e), "rust": sh["rust"]})
            continue
        out.append({"num": num, "odin": T, "ret": sh["ret"], "rust": sh["rust"], "args": sh["args"],
                    "bytes": payload, "typed": bool(sh["args"]) and all(typed(m, a) for a in sh["args"])})
    return out


def qualify(t):
    """`Bounds` -> `gk.Bounds`; built-in Odin types stay as they are."""
    return t if t in ("bool", "int", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "string") \
        else "gk." + t


class TDec:
    """Decoders the test alone needs: the payload types that only ever travel out (events), built
    from the same bytes their Wire impls write. Generated into the diagnostic library only."""

    def __init__(self, rust, m):
        self.rust, self.m, self.fns, self.busy = rust, m, {}, set()

    def ok(self, d):
        m, k = self.m, d[0]
        if decodable(m, d):
            return True
        if k == "leaf":
            return d[1] == "color" and (len(d) < 3 or not d[2])
        if k == "enum":
            return bool(m.enums[d[1]]["variants"])
        if k == "rec":
            rc = m.records[d[1]]
            if not rc["complete"] or d[1] in self.busy:
                return False
            self.busy.add(d[1])
            try:
                return all(self.ok(f["desc"]) for f in rc["fields"])
            finally:
                self.busy.discard(d[1])
        if k == "denum":
            v = pick_variant(m, m.denums[d[1]])
            if v is None or d[1] in self.busy:
                return False
            self.busy.add(d[1])
            try:
                return all(self.ok(f["desc"]) for f in v["fields"])
            finally:
                self.busy.discard(d[1])
        if k in ("opt", "list", "ref"):
            return self.ok(d[1])
        if k == "from":
            return self.ok(d[1])
        return False

    def expr(self, d):
        m, k = self.m, d[0]
        if d == ["str"]:
            return "r.str().to_string()"
        if decodable(m, d):
            return self.rust.dec(d)
        if k == "leaf":  # an Hsla, as it travels out
            return "gpui_kit::Hsla { h: r.f32(), s: r.f32(), l: r.f32(), a: r.f32() }"
        if k in ("enum", "rec", "denum"):
            t = {"enum": m.enums, "rec": m.records, "denum": m.denums}[k][d[1]]
            name = f"t_{t['fn']}"
            if name not in self.fns:
                self.fns[name] = None
                self.fns[name] = self.fn(k, d[1], t, name)
            return f"{name}(r)?"
        if k == "opt":
            return f"if r.u8() != 0 {{ Some({self.expr(d[1])}) }} else {{ None }}"
        if k == "list":
            return ("{ let n = r.u32() as usize; let mut v = Vec::with_capacity(n.min(4096)); "
                    f"for _ in 0..n {{ v.push({self.expr(d[1])}); }} v }}")
        if k == "ref":
            return self.expr(d[1])
        if k == "from":
            return f"<{d[2]} as From<{d[3]}>>::from({self.expr(d[1])})"
        raise ValueError(k)

    def fn(self, k, rt, t, name):
        if k == "enum":
            arms = " ".join(f"{v['num']} => {t['path']}::{v['name']}," for v in t["variants"])
            return f"fn {name}(r: &mut R) -> Option<{rt}> {{ Some(match r.u16() {{ {arms} _ => return None }}) }}"
        if k == "rec":
            if t["tuple"]:
                body = f"{t['path']}(" + ", ".join(self.expr(f["desc"]) for f in t["fields"]) + ")"
            else:
                body = f"{t['path']} {{ " + ", ".join(f"{f['name']}: {self.expr(f['desc'])}" for f in t["fields"]) + " }"
            return f"fn {name}(r: &mut R) -> Option<{rt}> {{ Some({body}) }}"
        v = pick_variant(self.m, t)  # the one variant the sample sends
        if v["shape"] == "unit":
            body = f"{t['path']}::{v['name']}"
        elif v["shape"] == "tuple":
            body = f"{t['path']}::{v['name']}(" + ", ".join(self.expr(f["desc"]) for f in v["fields"]) + ")"
        else:
            body = f"{t['path']}::{v['name']} {{ " + ", ".join(f"{f['name']}: {self.expr(f['desc'])}" for f in v["fields"]) + " }"
        return (f"fn {name}(r: &mut R) -> Option<{rt}> {{ if r.u16() != {v['num']} {{ return None; }} "
                f"Some({body}) }}")


def typed(m, d, td=None):
    """True when the diagnostic library can build this payload argument in Rust."""
    return decodable(m, d) or (td is not None and td.ok(d)) or (td is None and TDec(None, m).ok(d))


def rust_test_shape(rust, m):
    """The generated half of call op TEST_FIRE_SHAPE (mod.rs): decode the payload into the Rust
    types and fire through their Wire impls. Only in the diagnostic library."""
    td = TDec(rust, m)
    L = ["/// The handler-shape test (core/tests/gen, call op TEST_FIRE_SHAPE): decodes the payload of",
         "/// `shape` into the Rust types its listeners hand over and fires `h` through their own Wire",
         "/// impls; the handler's return value comes back encoded. None when this library cannot build",
         "/// that payload (the runtime then fires the bytes as they are).",
         '#[cfg(feature = "diag")]',
         "pub fn test_shape(shape: u16, h: genapi::HandlerRef, r: &mut R, window: &mut gpui_kit::Window, "
         "cx: &mut gpui_kit::App) -> Option<Vec<u8>> {",
         "    match shape {"]
    rust.call = "codec"
    for key in sorted(m.shapes, key=lambda k: m.shapes[k]["num"]):
        sh = m.shapes[key]
        if not sh["args"] or not all(td.ok(a) for a in sh["args"]):
            continue
        lets = " ".join(f"let a{n} = {td.expr(a)};" for n, a in enumerate(sh["args"]))
        refs = [f"&a{n}" for n in range(len(sh["args"]))]
        payload = refs[0] if len(refs) == 1 else "&(" + ", ".join(refs) + ")"
        if sh["ret"] == "str":  # values family (13.12): text returned, encoded as on the wire
            fire = (f"let v: gpui_kit::SharedString = genapi::fire(h, {sh['num']}, {payload}, window, cx); "
                    "let mut b = Vec::new(); genapi::Put::put_str(&mut b, &v); Some(b)")
        elif sh["ret"] == "bool":
            fire = f"let v: bool = genapi::fire(h, {sh['num']}, {payload}, window, cx); Some(vec![v as u8])"
        else:
            fire = f"let _: () = genapi::fire(h, {sh['num']}, {payload}, window, cx); Some(Vec::new())"
        L.append(f"        {sh['num']} => {{ {lets} {fire} }}")
    rust.call = None
    L += ["        _ => None,", "    }", "}"]
    for name in sorted(td.fns):
        L += ['#[cfg(feature = "diag")]', td.fns[name]]
    L += ['#[cfg(not(feature = "diag"))]',
          "pub fn test_shape(shape: u16, h: genapi::HandlerRef, r: &mut R, window: &mut gpui_kit::Window, "
          "cx: &mut gpui_kit::App) -> Option<Vec<u8>> {", "    None", "}", ""]
    return L


def lit(bs):
    return "{" + ", ".join(f"0x{b:02x}" for b in bs) + "}"


class StateArgs:
    """Fixed arguments for state methods: the sample's values, except the ones a call op cannot
    take outside a view build (elements, nodes, other handles)."""

    def __init__(self, sample):
        self.s = sample

    def val(self, d, depth=0):
        if d[0] in ("entity", "handle", "child", "fragment", "node", "build", "stylefn", "action", "shim", "rows", "obj", "refmut"):
            return None
        if d[0] in ("opt", "ref", "from", "list") and d[1][0] in ("entity", "handle", "child", "fragment", "node", "build", "obj"):
            return None
        if d[0] == "into":
            for mem in self.s.m.intos[d[1]]["members"]:
                v = self.val(mem["desc"], depth + 1)
                if v is not None:
                    return self.s.val(mem["desc"], depth + 1)
            return None
        if d[0] == "rec":
            if any(self.val(f["desc"], depth + 1) is None for f in self.s.m.records[d[1]]["fields"]):
                return None
        return self.s.val(d, depth)

    def args(self, it):
        vs = [self.val(a["desc"]) for a in it["args"] if a["desc"][0] != "ctx"]
        return None if None in vs else ", ".join(vs)


def states(m, sample, oname):
    """Per generated state type: its constructor call (with the builder chain), and the reads,
    writes, calls and events the test calls, by Odin name with their argument lists."""
    sa = StateArgs(sample)
    ok = lambda i: i["status"] == "bridged"  # noqa: E731
    out = []
    for key in sorted(m.states, key=lambda k: m.states[k].get("odin", "") or ""):
        st = m.states[key]
        S = st.get("odin")
        if not S:
            continue
        ctor = None
        for it in sorted([i for i in st["ctors"] if ok(i)], key=lambda i: i["odin"]):
            if any(a["desc"][0] in ("shim", "rows") for a in it["args"]):
                continue
            a = sa.args(it)
            if a is not None:
                ctor = f"gk.{it['odin']}(cx{', ' + a if a else ''})"
                break
        if ctor is None:
            why = "its constructor takes a row source (core/tests/rows)" if any(
                a["desc"][0] in ("shim", "rows") for i in st["ctors"] for a in i["args"]) else "no constructor with arguments this test can make"
            out.append({"odin": S, "skip": why})
            continue
        builders = sorted([i for i in st["builders"] if ok(i)], key=lambda i: i["name"])
        chain, nb = "", 0
        for it in builders:
            a = sa.args(it)
            if a is not None:
                chain += f"->{oname(it['name'])}({a})"
                nb += 1
        if builders:
            chain += "->build()"
        names = set()
        reads, writes, calls = [], [], []
        for group, dst in (("reads", reads), ("writes", writes), ("calls", calls)):
            for it in sorted([i for i in st.get(group, []) if ok(i)], key=lambda i: i["name"]):
                names.add(it["name"])
                a = sa.args(it)
                if a is not None:
                    dst.append((oname(it["name"]), a))
        events = [ev["odin"] for ev in st["events"] if ev.get("odin") and ev["odin"] not in names]
        out.append({"odin": S, "low": S.lower(), "make": ctor + chain, "builders": nb, "reads": reads,
                    "writes": writes, "calls": calls, "events": events})
    return out


def odin_test(m, od, sample, oname, path, skip):
    """core/tests/gen/main.odin: modes `shapes` and `states`."""
    sh = shapes(m, od)
    st = states(m, sample, oname)
    skip_calls = set(skip.get("calls", []))
    o = ["// Generated by gen/gkgen.py (gen/gk_tests.py). Do not edit.",
         "// The generated protocol tests: every handler shape the package registers fired once through",
         "// the library, every generated state type made, read, written, called, subscribed and released.",
         "// Run by core/tests/run-gen.sh (GK_PROTO=shapes | states), offscreen, with the diag library.",
         "package gen_test", "", 'import "core:bytes"', 'import "core:fmt"', 'import "core:os"', 'import "core:time"',
         'import gk "gpuikit:gk"', "", "@(private)", "_t :: time.Duration", "",
         "App :: struct {", "\tmode:   string,", "\troot:   gk.View,", "\tclicks: int,", "\tgot:    []u8,",
         "\tfired:  bool,", "\tok:     int,", "\tfailed: int,", "\ttyped:  int,", "\tevents: int,",
         "\tsubs:   [dynamic]gk.Subscription,"]
    for s in st:
        if "make" in s:
            o.append(f"\tst_{s['low']}: gk.{s['odin']},")
    o += ["}", "",
          "// Rows for every call that takes a data table (the sample's, gen/gkgen.py; PROTOCOLS.md section 12).",
          "Sample_Row :: struct {", "\tlabel: string,", "\tvalue: f64,", "\tgap:   Maybe(f64),", "\tcolor: gk.Color,", "}", "",
          "SAMPLE_ROWS := [?]Sample_Row{{\"a\", 1, 2, {}}, {\"b\", 3, nil, {}}}", "",
          "noop :: proc(a: ^App, cx: ^gk.Cx) {}",
          "// fits every shape: a handler without a payload parameter",
          "any_event :: proc(a: ^App, cx: ^gk.Cx) {a.events += 1}", "",
          "got :: proc(a: ^App, b: []u8) {", "\ta.got = bytes.clone(b)", "\ta.fired = true", "}", "",
          "hex :: proc(b: []u8) -> string {", "\treturn fmt.tprintf(\"%x\", b)", "}", "",
          "try_shape :: proc(a: ^App, cx: ^gk.Cx, num: u16, name: string, h: gk.Handler, payload: []u8, want_typed: bool, want_ret: []u8) {",
          "\ta.got, a.fired = nil, false",
          "\ttyped, ret := gk.test_fire_shape(cx, h, num, payload)",
          "\tok := a.fired && bytes.equal(a.got, payload) && bytes.equal(ret, want_ret) && typed == want_typed",
          '\tfmt.printf("shape %d %s %s %s\\n", num, "typed" if typed else "raw", "ok" if ok else "FAIL", name)',
          "\tif ok {", "\t\ta.ok += 1", "\t} else {", "\t\ta.failed += 1",
          '\t\tfmt.printf("  fired=%v sent=%s got=%s ret=%s want_ret=%s typed=%v want_typed=%v\\n", a.fired, hex(payload), hex(a.got), hex(ret), hex(want_ret), typed, want_typed)',
          "\t}", "\tif typed {", "\t\ta.typed += 1", "\t}", "}", ""]
    for s in sh:
        if "skip" in s:
            continue
        T, ret = s["odin"], {"bool": " -> bool", "str": " -> string"}.get(s["ret"], "")
        back = {"bool": "\n\treturn true", "str": '\n\treturn "s"'}.get(s["ret"], "")  # str: values family (13.12)
        o.append(f"// shape {s['num']}: ({', '.join(s['rust'])}){' -> bool' if s['ret'] == 'bool' else ''}")
        if T is None:
            o.append(f"h_{s['num']} :: proc(a: ^App, cx: ^gk.Cx){ret} {{\n\tgot(a, nil){back}\n}}")
        else:
            o.append(f"h_{s['num']} :: proc(a: ^App, cx: ^gk.Cx, e: {T}){ret} {{\n\tgot(a, gk.test_wire(e)){back}\n}}")
    o += ["", "run_shapes :: proc(a: ^App, cx: ^gk.Cx) {"]
    n = 0
    for s in sh:
        if "skip" in s:
            o.append(f"\t// shape {s['num']} ({', '.join(s['rust'])}): no sample ({s['skip']})")
            continue
        n += 1
        want_ret = {"bool": "{0x01}", "str": "{0x01, 0x00, 0x00, 0x00, 0x73}"}.get(s["ret"], "{}")
        o.append(f"\ttry_shape(a, cx, {s['num']}, \"({', '.join(s['rust'])})\", gk.on(h_{s['num']}), []u8{lit(s['bytes'])}, "
                 f"{'true' if s['typed'] else 'false'}, []u8{want_ret})")
    o += [f'\tfmt.printf("shapes done ok=%d failed=%d typed=%d of {n}\\n", a.ok, a.failed, a.typed)', "}", ""]
    # states: made, used and subscribed in one turn; read again and released in the next
    o += ["make_states :: proc(a: ^App, cx: ^gk.Cx) {"]
    made = 0
    for s in st:
        if "skip" in s:
            o.append(f"\t// {s['odin']}: not made here: {s['skip']}")
            continue
        made += 1
        L = s["low"]
        o.append(f"\ta.st_{L} = {s['make']}")
        for name, args in s["reads"]:
            o.append(f"\t_ = a.st_{L}->{name}(cx{', ' + args if args else ''})")
        for name, args in s["calls"]:
            if f"{s['odin']}.{name}" in skip_calls:
                o.append(f"\t// {name}: skipped (api/overrides.toml [tests] calls)")
                continue
            o.append(f"\t_ = a.st_{L}->{name}(cx{', ' + args if args else ''})")
        for name, args in s["writes"]:
            if f"{s['odin']}.{name}" in skip_calls:
                o.append(f"\t// {name}: skipped (api/overrides.toml [tests] calls)")
                continue
            o.append(f"\ta.st_{L}->{name}(cx{', ' + args if args else ''})")
        for ev in s["events"]:
            o.append(f"\tappend(&a.subs, a.st_{L}->{ev}(cx, gk.on(any_event)))")
        o.append(f"\tfmt.printf(\"states made {s['odin']} builders={s['builders']} reads={len(s['reads'])} "
                 f"writes={len(s['writes'])} calls={len(s['calls'])} events={len(s['events'])}\\n\")")
    o += [f'\tfmt.printf("states made all n={made}\\n")', "}", "",
          "release_states :: proc(a: ^App, cx: ^gk.Cx) {"]
    for s in st:
        if "skip" in s:
            continue
        L = s["low"]
        if s["reads"]:
            name, args = s["reads"][0]
            o.append(f"\t_ = a.st_{L}->{name}(cx{', ' + args if args else ''})")
        o.append(f"\tgk.release(cx, a.st_{L})")
    o += ["\tfor sub in a.subs {", "\t\tgk.release(cx, sub)", "\t}",
          f'\tfmt.printf("states released n={made} subscriptions=%d\\n", len(a.subs))', "}", "",
          "clicked :: proc(a: ^App, cx: ^gk.Cx) {", "\ta.clicks += 1", "\tswitch a.mode {", "\tcase \"shapes\":",
          "\t\tif a.clicks == 1 {", "\t\t\trun_shapes(a, cx)", "\t\t}", "\tcase \"states\":",
          "\t\tswitch a.clicks {", "\t\tcase 1:", "\t\t\tmake_states(a, cx)", "\t\tcase 2:", "\t\t\trelease_states(a, cx)",
          "\t\t}", "\t}", "}", "",
          "root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {",
          '\treturn gk.div(.Size_Full)->id("all")->on_click(gk.on(clicked))->text("gen")', "}", "",
          "main :: proc() {", "\tapp: App", '\tapp.mode, _ = os.lookup_env("GK_PROTO", context.allocator)',
          '\tscript := "size 320 200\\nframe\\nclick 100 100\\nframe\\nframe\\nclick 100 100\\nframe\\nframe\\nstat\\n"',
          "\tgk.run_headless(&app, script, proc(a: ^App, cx: ^gk.Cx) {", "\t\ta.root = gk.view(a, nil, root_view)",
          "\t\tgk.open_window(cx, \"gen\", {320, 200}, a.root)", "\t})",
          '\tfmt.printf("gen done mode=%s clicks=%d ok=%d failed=%d events=%d\\n", app.mode, app.clicks, app.ok, app.failed, app.events)',
          "}"]
    import os
    os.makedirs(os.path.dirname(path), exist_ok=True)
    open(path, "w").write("\n".join(o) + "\n")
    return {"shapes": n, "shape_skips": [s for s in sh if "skip" in s], "typed": sum(1 for s in sh if s.get("typed")),
            "states": made, "state_skips": [s for s in st if "skip" in s]}
