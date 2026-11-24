"""The generated protocol tests (core/tests/go/gen, docs/PROTOCOLS.md section 8): one case per
handler shape and one per generated state type, written by `make gen` from the same model the
decoders and the package are written from, so a new shape or state type is tested the day the
generator first emits it.

Shapes. Every shape the package registers is fired once through the library (call op
TEST_FIRE_SHAPE) with a payload whose bytes are written here from the wire model, the model
the Rust `Wire` impls are emitted from. When the generated codecs can build that payload as
the Rust types a real listener hands over, the library decodes it into those types and fires
through their own `Wire` impls (typed); otherwise it fires the bytes as they are (raw). The
app's handler, registered for the payload type the package registered, encodes what it
received again: the test passes when those bytes are the bytes sent, the return value
arrives, and the typed flag is the one predicted here.

States. Every generated state type with a constructor this file can call: made (through its
builder chain, every builder once), every read, write and call once with fixed arguments,
every event subscribed; read again in a later turn, after the writes were applied; released.
"""
import struct

from gk_model import INT_WIRE

INT_FMT = {"u8": "B", "u16": "H", "u32": "I", "u64": "Q", "i8": "b", "i16": "h", "i32": "i", "i64": "q"}


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


def shapes(m):
    """Every shape the package registers: number, return, the Rust payload as written, sample
    bytes, and whether the library can fire it typed."""
    out = []
    for key in sorted(m.shapes, key=lambda k: m.shapes[k]["num"]):
        sh = m.shapes[key]
        num = sh["num"]
        try:
            bs = Bytes(m)  # one counter over all arguments, so two arguments of one type differ
            payload = b"".join(bs.of(a) for a in sh["args"])
        except ValueError as e:
            out.append({"num": num, "skip": str(e), "rust": sh["rust"]})
            continue
        out.append({"num": num, "ret": sh["ret"], "rust": sh["rust"], "args": sh["args"],
                    "bytes": payload, "typed": bool(sh["args"]) and all(typed(m, a) for a in sh["args"])})
    return out


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

