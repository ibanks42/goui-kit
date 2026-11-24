#!/usr/bin/env python3
"""Inspect, verify, and assemble 64-bit Mach-O thin and universal (fat) binaries.

Usage:
    macho.py info FILE|-
    macho.py check FILE|- ARCH... [--kind KIND] [--id NAME] [--minos VER]
    macho.py fat OUT_FILE THIN_FILE...

Runs on any OS (pure Python stdlib) so Linux release checks can inspect macOS
universal dylibs and static archives without Apple tooling.
"""
import struct
import sys

# Mach-O magic constants, CPU type IDs, file types, and load command tags.
FAT_MAGIC, FAT_MAGIC_64 = 0xCAFEBABE, 0xCAFEBABF
MH_MAGIC_64 = 0xFEEDFACF
CPU = {0x01000007: "x86_64", 0x0100000C: "arm64"}
CPU_IDS = {v: k for k, v in CPU.items()}
FILETYPE = {1: "object", 2: "executable", 6: "dylib", 8: "bundle"}
LC_ID_DYLIB, LC_VERSION_MIN_MACOSX, LC_BUILD_VERSION = 0xD, 0x24, 0x32
PLATFORM_MACOS = 1


def version(v):
    """Decode a packed Mach-O X.Y.Z version integer into a dotted string."""
    return f"{v >> 16}.{(v >> 8) & 0xFF}" + (f".{v & 0xFF}" if v & 0xFF else "")


def thin(data, off=0, cpu=None):
    """Parse one thin 64-bit Mach-O image (or ar archive) at byte offset `off`."""
    if data[off:off + 8] == b"!<arch>\n":
        return CPU.get(cpu, f"cpu{cpu:#x}" if cpu is not None else "unknown"), "archive", None, None
    magic, cputype, _sub, filetype, ncmds, _size, _flags, _res = struct.unpack_from("<8I", data, off)
    if magic != MH_MAGIC_64:
        raise ValueError(f"not a 64-bit Mach-O slice at offset {off} (magic {magic:#x})")
    name = minos = None
    p = off + 32
    for _ in range(ncmds):
        cmd, size = struct.unpack_from("<2I", data, p)
        if cmd == LC_ID_DYLIB:
            (noff,) = struct.unpack_from("<I", data, p + 8)
            name = data[p + noff:p + size].split(b"\0", 1)[0].decode()
        elif cmd == LC_BUILD_VERSION:
            platform, mos = struct.unpack_from("<2I", data, p + 8)
            if platform == PLATFORM_MACOS:
                minos = version(mos)
        elif cmd == LC_VERSION_MIN_MACOSX and minos is None:
            (mos,) = struct.unpack_from("<I", data, p + 8)
            minos = version(mos)
        p += size
    return CPU.get(cputype, f"cpu{cputype:#x}"), FILETYPE.get(filetype, f"type{filetype}"), name, minos


def slices(data):
    """Return metadata tuples for every architecture slice in a thin or fat Mach-O buffer."""
    (magic,) = struct.unpack_from(">I", data, 0)
    if magic not in (FAT_MAGIC, FAT_MAGIC_64):
        return [thin(data)]
    (n,) = struct.unpack_from(">I", data, 4)
    out = []
    for i in range(n):
        if magic == FAT_MAGIC:
            cpu, _sub, off, _size, _align = struct.unpack_from(">5I", data, 8 + 20 * i)
        else:
            cpu, _sub, off, _size, _align, _res = struct.unpack_from(">2I2Q2I", data, 8 + 32 * i)
        out.append(thin(data, off, cpu))
    return out


def read(path):
    if path == "-":
        return sys.stdin.buffer.read()
    with open(path, "rb") as f:
        return f.read()


def fat(out, inputs):
    """Pack distinct thin 64-bit Mach-O files into a 16 KiB-aligned universal Mach-O binary."""
    align = 14
    parts = []
    for path in inputs:
        data = read(path)
        if len(data) < 32 or struct.unpack_from("<I", data, 0)[0] != MH_MAGIC_64:
            raise SystemExit(f"macho.py: {path} is not a thin 64-bit Mach-O file")
        cputype, cpusub = struct.unpack_from("<2I", data, 4)
        parts.append((cputype, cpusub, data))
    if len({p[0] for p in parts}) != len(parts):
        raise SystemExit("macho.py: two inputs have the same architecture")
    head = struct.pack(">2I", FAT_MAGIC, len(parts))
    off = 1 << align
    table, body = b"", b""
    for cputype, cpusub, data in parts:
        table += struct.pack(">5I", cputype, cpusub, off, len(data), align)
        pad = (-(off + len(data))) % (1 << align)
        body += data + b"\0" * pad
        off += len(data) + pad
    hdr = head + table
    with open(out, "wb") as f:
        f.write(hdr + b"\0" * ((1 << align) - len(hdr)) + body)


def main(argv):
    if len(argv) >= 2 and argv[0] == "info":
        for arch, kind, name, minos in slices(read(argv[1])):
            print(f"{arch} {kind} install name {name or '(none)'} minimum macOS {minos or '(none)'}")
        return 0
    if len(argv) >= 3 and argv[0] == "check":
        want, name, minos, kind_want, rest = [], None, None, "dylib", argv[2:]
        while rest:
            a = rest.pop(0)
            if a == "--kind":
                kind_want = rest.pop(0)
            elif a == "--id":
                name = rest.pop(0)
            elif a == "--minos":
                minos = rest.pop(0)
            else:
                want.append(a)
        got = slices(read(argv[1]))
        bad = []
        if sorted(s[0] for s in got) != sorted(want):
            bad.append(f"slices {' '.join(s[0] for s in got)}, want {' '.join(want)}")
        for arch, kind, n, m in got:
            if kind != kind_want:
                bad.append(f"{arch} is a {kind}, not a {kind_want}")
            if name is not None and n != name:
                bad.append(f"{arch} install name {n}, want {name}")
            if minos is not None and m != minos:
                bad.append(f"{arch} minimum macOS {m}, want {minos}")
        if bad:
            print("macho.py: " + "; ".join(bad), file=sys.stderr)
            return 1
        print(", ".join(f"{a} {k}" + (f" (install name {n}" if n else " (") + f"{', ' if n else ''}minimum macOS {m or '(none)'})"
                        for a, k, n, m in got))
        return 0
    if len(argv) >= 3 and argv[0] == "fat":
        fat(argv[1], argv[2:])
        return 0
    print(__doc__.strip(), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
