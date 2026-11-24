#!/usr/bin/env python3
"""gkgen: the generator (docs/DESIGN.md section 13). Python 3 standard library only.

  gkgen.py strip JSON_DIR        stage 1b: reduce rustdoc's JSON (the output of gen/extract.sh)
                                 to the committed extract api/rustdoc/*.json.gz
  gkgen.py gen                   stages 2, 3 and 6 in one command, from the committed extract:
                                 classify every public function, apply api/quarantine.json, number
                                 from api/opcodes.lock, write the Rust decoders (core/src/gen, or
                                 --rust-out), the Go package (go/gk/gen_*.go, or --go-out), the
                                 generated tests (core/tests/go/gensample, gen, app), the opcode
                                 tables (core/wire/gen.py), api/gk.api.json.gz, api/coverage.json,
                                 gen/out/report.md and the coverage block of docs/STATUS.md.
                                 Exits 1 when a signature meets no rule (gen/out/no-rule.md lists
                                 them), unless --allow-no-rule.
  gkgen.py quarantine rust LOG   charge the errors of a `cargo check --message-format short` log to
                                 items (api/quarantine.json) and run `gen` again; exit 3 if any moved

rustc and the Go compiler are the judges: nothing here decides that an item compiles.
"""
import gzip
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import gk_model  # noqa: E402
import gk_names  # noqa: E402
import gk_rust  # noqa: E402
import gk_values  # noqa: E402  (the values family: partly reachable options, action types)
import rdj  # noqa: E402

REPO = os.path.dirname(HERE)
API = os.path.join(REPO, "api")
OUT = os.path.join(HERE, "out")
QPATH = os.path.join(API, "quarantine.json")
KEEP_STD = {"From", "Default"}
OURS = ("gpui", "gpui_base", "gpui_component", "gpui_kit")


# Public items rustdoc leaves out because their module is #[doc(hidden)], though a public module
# re-exports them: (crate, the item's own path, the public module that re-exports it). Taken from
# the --document-hidden-items pass (gen-json/hidden, gen/extract.sh) with their impls, every id
# they name mapped to the public pass's by path. gpui-component's styled code editor
# (`#[doc(hidden)] mod editor; pub use editor::Editor;` in input/mod.rs) is the kit's Editor.
GRAFTS = [("gpui_component", ["gpui_component", "input", "editor", "Editor"], ["gpui_component", "input"])]


def graft(d, h, path, into):
    """Adds the struct at `path` of the hidden pass `h` to the public pass `d`, in module `into`."""
    hpaths = h["paths"]
    hid = next((k for k, p in hpaths.items() if p["path"] == path and p["kind"] == "struct" and p["crate_id"] == 0), None)
    if hid is None or hid not in h["index"]:
        raise SystemExit(f"strip: graft: {'::'.join(path)} is not in the hidden pass")
    if any(p["path"] == path for p in d["paths"].values()):
        return  # public now: nothing to graft
    hnames = {k: v["name"] for k, v in h["external_crates"].items()}
    dnames = {v["name"]: k for k, v in d["external_crates"].items()}
    # The public pass's ids by (crate name, path, kind).
    def crate_of(p, names):
        return p["path"][0] if p["crate_id"] == 0 else names.get(str(p["crate_id"]))
    pub = {}
    for k, p in d["paths"].items():
        pub[(crate_of(p, {k2: v["name"] for k2, v in d["external_crates"].items()}), tuple(p["path"]), p["kind"])] = k
    # The grafted items: the struct, its impls and their items.
    item = h["index"][hid]
    take = [hid]
    for i in item["inner"]["struct"]["impls"]:
        im = h["index"].get(str(i))
        if im is None:
            continue
        take.append(str(i))
        take += [str(x) for x in im["inner"]["impl"]["items"] if str(x) in h["index"]]
    nxt = max(int(k) for k in d["index"]) + 1
    new = {}
    for k in take:
        new[k] = nxt
        nxt += 1

    def remap(x):
        x = str(x)
        if x in new:
            return new[x]
        p = hpaths.get(x)
        if p is None:
            return 2**31 - 1  # an item the public pass does not have: unresolved, as a hidden one is
        hit = pub.get((crate_of(p, hnames), tuple(p["path"]), p["kind"]))
        return int(hit) if hit is not None else 2**31 - 1

    def walk(o):
        if isinstance(o, dict):
            out = {}
            for k, v in o.items():
                if k == "id" and isinstance(v, int):
                    out[k] = remap(v)
                elif k in ("items", "impls", "fields") and isinstance(v, list) and all(isinstance(x, int) for x in v):
                    out[k] = [remap(x) for x in v]
                elif k == "links" and isinstance(v, dict):
                    out[k] = {a: remap(b) for a, b in v.items()}
                else:
                    out[k] = walk(v)
            return out
        if isinstance(o, list):
            return [walk(x) for x in o]
        return o

    for k in take:
        it = walk(h["index"][k])
        it["id"] = new[k]
        d["index"][str(new[k])] = it
    d["paths"][str(new[hid])] = {"crate_id": 0, "path": path, "kind": "struct"}
    mod = pub.get((into[0], tuple(into), "module"))
    if mod is None:
        raise SystemExit(f"strip: graft: no public module {'::'.join(into)}")
    d["index"][mod]["inner"]["module"]["items"].append(new[hid])
    print(f"strip: grafted {'::'.join(path)} from the hidden pass into {'::'.join(into)} ({len(take)} items)")


def strip(src):
    """rustdoc JSON -> api/rustdoc/NAME.json.gz: docs, spans and links removed, and the impls
    nothing reads (auto traits, blanket and std-trait impls of third-party traits); the docs and
    their links -> api/rustdoc/docs.json.gz."""
    dst = os.path.join(API, "rustdoc")
    os.makedirs(dst, exist_ok=True)
    docs = {}
    for n in OURS:
        d = json.load(open(os.path.join(src, n + ".json")))
        for crate, path, into in GRAFTS:
            if crate == n:
                hidden = os.path.join(os.path.dirname(os.path.abspath(src)), "hidden", n + ".json")
                graft(d, json.load(open(hidden)), path, into)
        idx, drop = d["index"], set()
        names = {k: v["name"] for k, v in d["external_crates"].items()}
        # The doc comments go to docs.json.gz (gen/gk_doc.py writes them into the bindings): per
        # documented item its text and intra-doc links, and the full path of every linked id.
        mine, linked = {}, {}
        for k, it in idx.items():
            if it.get("docs"):
                mine[k] = {"d": it["docs"], "l": it.get("links") or {}}
                for i in (it.get("links") or {}).values():
                    p = d["paths"].get(str(i))
                    if p:
                        linked[str(i)] = "::".join(p["path"])
        docs[n] = {"items": mine, "paths": linked}
        for k, it in idx.items():
            for f in ("docs", "links", "span"):
                it.pop(f, None)
            im = it["inner"].get("impl")
            if not im:
                continue
            if im.get("is_synthetic"):
                drop.add(k)
                continue
            tr = im.get("trait")
            if tr:
                p = d["paths"].get(str(tr["id"]))
                ext = names.get(str(p["crate_id"])) if p and p["crate_id"] else None
                ours = p is not None and (p["crate_id"] == 0 or ext in OURS or (ext or "").startswith("gpui"))
                if not ours and not (ext in ("core", "std", "alloc") and tr["path"].split("::")[-1] in KEEP_STD
                                     and im.get("blanket_impl") is None):
                    drop.add(k)
                    drop.update(str(i) for i in im["items"])
        for k in drop:
            idx.pop(k, None)
        out = {k: d[k] for k in ("format_version", "crate_version", "index", "paths", "external_crates", "root")}
        for e in out["external_crates"].values():
            e.pop("html_root_url", None)
        b = json.dumps(out, separators=(",", ":"), sort_keys=True).encode()
        with open(os.path.join(dst, n + ".json.gz"), "wb") as raw:
            with gzip.GzipFile(fileobj=raw, mode="wb", compresslevel=9, mtime=0) as f:
                f.write(b)
    b = json.dumps(docs, separators=(",", ":"), sort_keys=True).encode()
    with open(os.path.join(dst, "docs.json.gz"), "wb") as raw:
        with gzip.GzipFile(fileobj=raw, mode="wb", compresslevel=9, mtime=0) as f:
            f.write(b)
        print(f"strip: {n}: {len(b) >> 10} KiB of JSON, {os.path.getsize(os.path.join(dst, n + '.json.gz')) >> 10} KiB packed")


# ---------------------------------------------------------------- quarantine


def load_q():
    if os.path.exists(QPATH):
        return json.load(open(QPATH))
    return {"rounds": [], "entries": {}}


def apply_quarantine(m, q):
    """Marks what a compiler refused: the item fails, a refused codec fails its users."""
    ent = q["entries"]
    byid = {}
    for it in m.items:
        byid.setdefault(it["id"], it)
    for node in m.nodes.values():
        for it in node["ctors"] + node.get("converts", []):
            byid.setdefault(it["id"], it)
    dead, stale = set(), []
    for key, e in list(ent.items()):
        k = json.loads(key)
        if k[0] == "item" and k[1] in byid:
            if byid[k[1]]["status"] not in ("bound", "bridged"):
                stale.append(key)  # a rule changed and the item is no longer generated
                continue
            byid[k[1]].update(status="failed", reason=e["error"], judge=e["judge"])
            for it in m.items:  # a type-changing trait method is counted in its trait's row
                if k[1].endswith(f"::{it.get('trait') or ''}::{it['name']}") and False:
                    pass
        elif k[0] == "variant":
            for table in (m.enums, m.denums):
                if k[1] in table:
                    table[k[1]]["variants"] = [v for v in table[k[1]]["variants"] if v["name"] != k[2]]
        elif k[0] in ("enum", "denum", "rec", "into", "shape"):
            dead.add((k[0], k[1]))
        elif k[0] == "trait":
            for t in m.traits.values():
                if t["key"] == k[1]:
                    for it in t["ops"]:
                        it.update(status="failed", reason=e["error"], judge=e["judge"])
        elif k[0] == "table":
            if k[1] in m.nodes:
                m.nodes[k[1]]["tables"] = [t for t in m.nodes[k[1]]["tables"] if t != k[2]]
        elif k[0] == "node" and k[1] in m.nodes:
            for it in m.nodes[k[1]]["ctors"]:
                if it["status"] in ("bound", "bridged"):
                    it.update(status="failed", reason=e["error"], judge=e["judge"])
        elif k[0] == "theme" and k[1] in m.theme:
            m.theme.remove(k[1])
    for it in list(byid.values()):
        pass
    m.prune(dead)
    m.flow()
    if stale:
        for key in stale:
            del ent[key]
        json.dump(q, open(QPATH, "w"), indent=1, sort_keys=True)


def uses(d, dead):
    for x in gk_model.walk_desc(d):
        if x[0] in ("enum", "denum", "rec", "into") and (x[0], x[1]) in dead:
            return True
        if x[0] == "handler" and ("shape", x[1]) in dead:
            return True
    return False


def quarantine(judge, log_path):
    q = load_q()
    log = open(log_path, errors="replace").read()
    moved = 0
    if judge != "rust":
        sys.exit(f"quarantine: unknown judge {judge} (rust)")
    where = json.load(open(os.path.join(OUT, "rust-lines.json")))
    pat = re.compile(r"(?:^|/)gen/(\w+\.rs):(\d+):\d+: error(?:\[(E\d+)\])?: (.*)", re.M)
    hits = [(m.group(1), m.group(2), f"{m.group(3) or ''} {m.group(4)}".strip()) for m in pat.finditer(log)]
    look = lambda f, line: where.get(f, {}).get(line)  # noqa: E731
    unmapped = []
    for f, line, msg in hits:
        key = look(f, line)
        if key is None:
            unmapped.append(f"{f}:{line}: {msg}")
            continue
        ks = json.dumps(key)
        if ks not in q["entries"]:
            q["entries"][ks] = {"error": msg[:300], "judge": judge, "round": len(q["rounds"]) + 1}
            moved += 1
    q["rounds"].append({"judge": judge, "errors": len(hits), "moved": moved, "unmapped": len(unmapped)})
    json.dump(q, open(QPATH, "w"), indent=1, sort_keys=True)
    print(f"quarantine: {judge}: {len(hits)} errors, {moved} items or types refused, {len(unmapped)} errors outside any item")
    for u in unmapped[:15]:
        print("  unmapped:", u)
    return moved, unmapped


# ---------------------------------------------------------------- gen


def gen(argv):
    rust_out = os.path.join(REPO, "core", "src", "gen")
    go_out = os.path.join(REPO, "go", "gk")
    allow = "--allow-no-rule" in argv
    for n, a in enumerate(argv):
        if a == "--rust-out":
            rust_out = argv[n + 1]
        if a == "--go-out":
            go_out = argv[n + 1]
    os.makedirs(OUT, exist_ok=True)
    m = gk_model.Model(os.environ.get("GK_JSON", os.path.join(API, "rustdoc")), os.path.join(API, "overrides.toml"))
    q = load_q()
    apply_quarantine(m, q)
    lock = gk_rust.Lock(os.path.join(API, "opcodes.lock"))
    gk_rust.number(m, lock)
    lock.save()
    rs = gk_rust.Rust(m, rust_out)
    where = rs.emit()
    json.dump(where, open(os.path.join(OUT, "rust-lines.json"), "w"))
    import gk_doc  # noqa: PLC0415 (GPUI-kit's doc comments, translated into Go)
    docs, doc_names = gk_doc.Docs(), gk_doc.resolver(m)
    names = gk_names.Names(m)
    if names.collisions:
        with open(os.path.join(OUT, "collisions.md"), "w") as f:
            f.write("# Names that collided and got a number\n\n")
            for base, key, other in names.collisions:
                f.write(f"- `{base}`: {key} (taken by {other})\n")
    report(m, q, names)
    norule = [i for i in m.items if i["status"] == "norule"]
    if norule:
        with open(os.path.join(OUT, "no-rule.md"), "w") as f:
            f.write("# Signatures no rule covers\n\n")
            for i in norule:
                f.write(f"- `{i['id']}`: `{i['sig']}`: {i['reason']}\n")
        print(f"gkgen: {len(norule)} functions meet no rule (gen/out/no-rule.md)", file=sys.stderr)
        for i in norule[:10]:
            print(f"  {i['id']}: {i['reason']}", file=sys.stderr)
        if not allow:
            sys.exit(1)
    elif os.path.exists(os.path.join(OUT, "no-rule.md")):
        os.remove(os.path.join(OUT, "no-rule.md"))
    import gk_go  # noqa: PLC0415 (the Go package, docs/GO.md: from the model and the stems gk_names gave it)
    go = gk_go.Go(m, go_out)
    go.docs, go.doc_names = docs, doc_names
    go.emit()
    import gk_tests_go  # noqa: PLC0415 (the generated protocol tests in Go: core/tests/go/gensample, gen, app)
    gk_tests_go.emit(m, go, REPO)
    n = docs.report()
    print(f"gen: docs: {len(docs.found):,} of {len(docs.asked):,} upstream items have a doc comment, carried into "
          f"Go; {n} sentences left out as Rust-only (gen/out/doc-rust-only.md)")
    rs.fam.go_link(go_out)  # each family linked only when the program calls into it (gen/gk_fam.py)
    rows, tot = go.report(os.path.join(OUT, "go-report.md"), family)
    print(f"gen: go: {tot[1]:,} of {tot[0]:,} bound or bridged functions generated into {os.path.relpath(go_out, REPO)} "
          f"({tot[2]:,} not yet: gen/out/go-report.md)")
    if os.path.abspath(go_out) == os.path.join(REPO, "go", "gk"):
        import gk_apidoc  # noqa: PLC0415 (the component index of docs/API-GO.md, read from the package just written)
        gk_apidoc.main([])


# What remains, by the family of work that will finish it (docs/STATUS.md). First match wins,
# on the upstream path of the row in lower case.
FAMILIES = [
    ("tables and lists with delegates", r"table|::list|searchable_list|virtual|tree|select|combobox|autocomplete"),
    ("dialogs, popovers and overlays", r"dialog|popover|sheet|hover_card|tooltip|notification|toast|drawer|modal|preview_card"),
    ("menus, actions and commands", r"menu|command|action|keybinding|kbd|shortcut"),
    ("dock, panels and layout", r"dock|panel|tab_group|pane|resizable|grid_layout|sidebar|nav"),
    ("editor and text input", r"input|editor|highlighter|diff|markdown|text_view|textview|text::|completion|rope|"
                              r"diagnostic|displaymap|display_map|search|otp|history|selection"),
    ("charts and plots", r"chart|plot"),
    ("ambient context, theme and window", r"theme|windowext|window_ext|globalstate|root|activetheme|title_bar"),
]


def family(path):
    low = path.lower()
    for name, pat in FAMILIES:
        if re.search(pat, low):
            return name
    return "other components and values"


def verified_ids(m):
    """Functions a byte-identical gallery page shows (gates/verified.json, written by
    gates/gallery.sh --record): exact ids from the generated one-function pages; the
    hand-written pages name a function Type.method, a free constructor, or style.name."""
    path = os.path.join(REPO, "gates", "verified.json")
    if not os.path.exists(path):
        return set()
    names = set(json.load(open(path)).get("functions", []))
    items = m.all_items()
    ids = {it["id"] for it in items}
    out = names & ids
    by_tail = {}
    for it in items:
        if it["status"] in ("bound", "bridged"):
            by_tail.setdefault(it["id"].rsplit("::", 1)[-1], []).append(it)
    nodes = {n["name"]: n for n in m.nodes.values() if "::component::" in n["path"] or n["crate"] == "gpui"}
    for name in names - ids:
        if name.startswith("style."):
            cands = [it for it in by_tail.get(name[6:], []) if it["id"].startswith("gpui_kit::Styled::")]
        elif "." in name:
            ty, fn = name.split(".", 1)
            cands = [it for it in by_tail.get(fn, []) if it["id"].endswith(f"::{ty}::{fn}")]
            cands = [it for it in cands if "::component::" in it["id"]] or cands
            if not cands and ty in nodes:  # a builder trait's method, called on this type
                tabs = set(nodes[ty]["tables"])
                cands = [it for t in m.traits.values() if t["key"] in tabs for it in t["ops"]
                         if it["name"] == fn and it["status"] in ("bound", "bridged")]
        else:
            cands = [it for it in by_tail.get(name, []) if it.get("free") or it["id"].count("::") <= 2]
        if cands:
            out.add(cands[0]["id"])
    return out


def report(m, q, names):
    rows, by_reason = [], {}
    ver = verified_ids(m)
    for r in m.owners:
        if not r["items"]:
            continue
        c = {"bound": 0, "bridged": 0, "skipped": 0, "failed": 0, "todo": 0}
        todo, skips, nver = {}, {}, 0
        for it in r["items"]:
            st = "todo" if it["status"] == "norule" else it["status"]
            c[st] += 1
            if st == "skipped":
                skips[it["reason"]] = skips.get(it["reason"], 0) + 1
            if st in ("bound", "bridged") and (it["id"] in ver or any(a in ver for a in it.get("also", ()))):
                nver += 1
            if st == "todo":
                key = it["reason"] if it["status"] != "norule" else "no rule covers the signature"
                todo[key] = todo.get(key, 0) + 1
            if st in ("skipped", "failed"):
                by_reason.setdefault((st, it["reason"] if st == "skipped" else it.get("judge", "rust")), []).append(it)
        rows.append({"name": r["name"], "path": r["path"], "group": r["group"], "total": len(r["items"]),
                     "verified": nver, "todo_reasons": todo, "skip_reasons": skips, "family": family(r["path"]), **c})
    kit_fns = sum(r["total"] for r in rows if r["group"] not in ("gpui", "style"))
    cov = {"schema": 1,
           "pin": {"gpui-kit": m.api.crates["gpui_component"].version, "gpui-fast": m.api.crates["gpui"].version,
                   "features": ["gpui-fast", "syntect"]},
           "universe": {"functions": 9425, "types": 586,
                        "note": f"this table: every public function of the kit crates ({kit_fns}; a trait's methods are counted "
                                "once, in the trait's row, not per implementor), the gpui traits and types the kit's elements "
                                "are made of, and the style table; an inventory of the pinned crates counts 4,350 functions in the kit crates and "
                                "5,075 in gpui-fast"},
           "components": rows,
           "partial": gk_values.partials(m)}  # options only partly reachable (tools/status.py)
    json.dump(cov, open(os.path.join(API, "coverage.json"), "w"), indent=1)
    # the interface description: language-neutral (no emitter's names)
    drop = {"fn", "stem", "fcrate", "members", "short_stem"}
    clean = lambda d: {k: v for k, v in d.items() if k not in drop}  # noqa: E731
    desc = {"schema": 1, "pin": cov["pin"],
            "items": [{k: v for k, v in it.items() if k not in ("fn", "stem", "fcrate", "crate", "row", "emitted")}
                      for it in m.all_items()],
            "states": {k: {"path": st["path"], **{g: [i["id"] for i in st[g]] for g in ("ctors", "builders", "reads", "writes", "calls", "events")}}
                       for k, st in sorted(m.states.items())},
            "nodes": {k: {"path": n["path"], "element": n["element"], "styled": n["styled"], "tables": n["tables"],
                          "ctors": [i["id"] for i in n["ctors"]], "ops": [i["id"] for i in n["ops"]],
                          "converts": [i["id"] for i in n.get("converts", [])]} for k, n in sorted(m.nodes.items())},
            "traits": {t["key"]: {"number": t.get("num"), "via_style": t["via_style"], "ops": [i["id"] for i in t["ops"]]}
                       for t in m.traits.values() if t["key"]},
            "enums": {k: clean(v) for k, v in sorted(m.enums.items())},
            "data_enums": {k: clean(v) for k, v in sorted(m.denums.items())},
            "records": {k: clean(v) for k, v in sorted(m.records.items())},
            "into": {k: {"target": v["target"], "members": v["members"]} for k, v in sorted(m.intos.items())},
            "shapes": {str(v["num"]): {"args": v["args"], "ret": v["ret"], "rust": v["rust"]} for v in m.shapes.values()},
            "theme": m.theme_nums, "palette": {i["id"]: i["op"] for i in m.palette},
            "values": gk_values.describe(m)}  # the values family: action types (gates/accept.py)
    with open(os.path.join(API, "gk.api.json.gz"), "wb") as raw:
        with gzip.GzipFile(fileobj=raw, mode="wb", compresslevel=9, mtime=0) as f:
            f.write(json.dumps(desc, separators=(",", ":"), sort_keys=True, default=str).encode())
    tot = {k: sum(r[k] for r in rows) for k in ("total", "bound", "bridged", "skipped", "failed", "todo")}
    o = ["# Generator report", "", "Written by `gen/gkgen.py gen`. Counts are functions.", "",
         f"- {tot['total']} functions in {len(rows)} rows: {tot['bound']} bound, {tot['bridged']} bridged, "
         f"{tot['skipped']} skipped, {tot['failed']} failed, {tot['todo']} not attempted yet.",
         f"- {len(m.nodes)} node types ({sum(1 for n in m.nodes.values() if n['element'])} elements), "
         f"{sum(1 for t in m.traits.values() if t['ops'])} builder traits, {len(m.enums)} enums, {len(m.denums)} enums with data, "
         f"{len(m.records)} records, {sum(1 for s in m.intos.values() if len(s['members']) > 1)} Into sets, "
         f"{len(m.shapes)} handler shapes, {len(m.tokens)} style tokens, {len(m.palette)} palette colours, "
         f"{len(m.theme)} theme colours.",
         f"- Names that collided: {len(names.collisions)}.", "",
         "## Compiler rounds", "", "| round | judge | errors | items or types refused | errors outside any item |", "|---|---|---|---|---|"]
    for n, r in enumerate(q["rounds"], 1):
        o.append(f"| {n} | {r['judge']} | {r['errors']} | {r['moved']} | {r['unmapped']} |")
    o += ["", "## Not attempted yet, by reason", "", "| reason | functions |", "|---|---|"]
    todo = {}
    for r in rows:
        for k, v in r["todo_reasons"].items():
            todo[k] = todo.get(k, 0) + v
    o += [f"| {k} | {v} |" for k, v in sorted(todo.items(), key=lambda kv: -kv[1])]
    o += ["", "## Skipped, by reason", "", "| reason | functions |", "|---|---|"]
    o += [f"| {k[1]} | {len(v)} |" for k, v in sorted(by_reason.items(), key=lambda kv: -len(kv[1])) if k[0] == "skipped"]
    o += ["", "## Refused by a compiler (quarantined)", "", "| what | judge | error |", "|---|---|---|"]
    for key, e in sorted(q["entries"].items()):
        o.append(f"| `{' '.join(map(str, json.loads(key)))[:110]}` | {e['judge']} | {e['error'][:160].replace('|', '/')} |")
    open(os.path.join(OUT, "report.md"), "w").write("\n".join(o) + "\n")
    print(f"gen: {tot['total']} functions: {tot['bound']} bound, {tot['bridged']} bridged, {tot['skipped']} skipped, "
          f"{tot['failed']} failed, {tot['todo']} not yet")
    # the opcode tables and the interface hash, which covers api/opcodes.lock
    subprocess.run([sys.executable, os.path.join(REPO, "core", "wire", "gen.py")], check=True)
    subprocess.run([sys.executable, os.path.join(REPO, "tools", "status.py")], check=False)


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "strip":
        strip(sys.argv[2])
    elif cmd == "gen":
        gen(sys.argv[2:])
    elif cmd == "quarantine":
        moved, unmapped = quarantine(sys.argv[2], sys.argv[3])
        gen(["--allow-no-rule"])
        sys.exit(3 if moved else (4 if unmapped else 0))
    else:
        sys.exit(__doc__)
