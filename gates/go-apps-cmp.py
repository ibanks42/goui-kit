#!/usr/bin/env python3
"""go-apps-cmp.py DIR NAME [BOX[;BOX..]]: the reader's verdict for gates/go-apps.sh. DIR holds odin/, odin2/
(the Odin app run a second time, the session's own noise) and go/. An image passes when Go's is
byte-identical to Odin's, or when every pixel that differs lies inside a BOX (x0,y0,x1,y1 in image
pixels: for the reader, the load toast's "in N ms", a time each app measures, and the list's
scrollbar, whose fade is timed by the wall clock, not the script's clock: two Odin runs differ
there now and then) or inside the box where the two Odin runs of this session differ.
Prints one GATE line and the reason for every image that is not byte-identical."""
import os
import struct
import sys
import zlib

args = sys.argv[1:]
# --labels REF,REF2,CAND: the folders compared (default odin,odin2,go; gates/go-apps.sh without
# Odin passes go,go2,go3: the Go app against itself, three runs of one session).
ref, ref2, cand = "odin", "odin2", "go"
if "--labels" in args:
    i = args.index("--labels")
    ref, ref2, cand = args[i + 1].split(",")
    del args[i:i + 2]
d, name = args[0], args[1]
boxes = [tuple(int(v) for v in b.split(",")) for b in args[2].split(";")] if len(args) > 2 else []


def png(path):
    """(width, height, rows of bytes) of an 8-bit RGB or RGBA PNG, with the standard library only."""
    data = open(path, "rb").read()
    pos, idat, w = 8, b"", 0
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        body = data[pos + 8:pos + 8 + n]
        if kind == b"IHDR":
            w, h, depth, color = struct.unpack(">IIBB", body[:10])
            if depth != 8 or color not in (2, 6):
                sys.exit(f"go-apps-cmp: {path}: only 8-bit RGB or RGBA images")
            bpp = 3 if color == 2 else 4
        elif kind == b"IDAT":
            idat += body
        pos += 12 + n
    raw, stride, rows, prev = zlib.decompress(idat), w * bpp, [], bytearray(w * bpp)
    for y in range(h):
        f, line = raw[y * (stride + 1)], bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b, c = prev[i], prev[i - bpp] if i >= bpp else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + (a + b) // 2) & 255
            elif f == 4:
                pa, pb, pc = abs(b - c), abs(a - c), abs(a + b - 2 * c)
                line[i] = (line[i] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append(bytes(line))
        prev = line
    return w, h, bpp, rows


def diff(a, b):
    """None when the files are the same; else the differing pixels as a set of (x, y)."""
    if open(a, "rb").read() == open(b, "rb").read():
        return None
    wa, ha, pa, ra = png(a)
    wb, hb, pb, rb = png(b)
    if (wa, ha, pa) != (wb, hb, pb):
        return {(x, y) for x in range(max(wa, wb)) for y in range(max(ha, hb))}
    out = set()
    for y in range(ha):
        if ra[y] != rb[y]:
            out |= {(i // pa, y) for i in range(len(ra[y])) if ra[y][i] != rb[y][i]}
    return out


def bbox(px):
    xs, ys = [p[0] for p in px], [p[1] for p in px]
    return (min(xs), min(ys), max(xs) + 1, max(ys) + 1) if px else (0, 0, 0, 0)


def inside(p, outer):
    return outer and outer[0] <= p[0] < outer[2] and outer[1] <= p[1] < outer[3]


files = sorted(f for f in os.listdir(os.path.join(d, ref)) if f.endswith(".png"))
same, ok, bad = 0, 0, []
for f in files:
    g = os.path.join(d, cand, f)
    if not os.path.exists(g):
        bad.append(f"{f}: missing in {cand}")
        continue
    px = diff(os.path.join(d, ref, f), g)
    if px is None:
        same += 1
        ok += 1
        continue
    noise = diff(os.path.join(d, ref, f), os.path.join(d, ref2, f)) or set()
    # each differing pixel lies in the load time's text, or near where Odin differs from itself
    nb = bbox(noise)
    rest = [p for p in px if not any(inside(p, b) for b in boxes) and not (noise and inside(p, nb))]
    if not rest:
        ok += 1
        why = f" or where the two {ref} runs differ too ({nb})" if noise else ""
        print(f"{f}: differs at {bbox(px)}, only inside {boxes}{why}")
    else:
        bad.append(f"{f}: differs at {bbox(rest)} besides the load time and {ref}'s own noise")
for b in bad:
    print(b)
verdict = "PASS" if not bad and files else "FAIL"
print(f"GATE go-apps {name} {verdict} {same} of {len(files)} images byte-identical, "
      f"{ok - same} differing only in the boxes of wall-clock time or where {ref} differs from itself, {len(bad)} otherwise")
sys.exit(0 if verdict == "PASS" else 1)
