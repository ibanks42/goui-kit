#!/usr/bin/env python3
"""macho.py: read and join Mach-O files without Apple's tools, on any system (standard library only).

  macho.py info FILE|-                 one line per slice: architecture, file type, install name
                                       (LC_ID_DYLIB) and minimum macOS (LC_BUILD_VERSION, or the older
                                       LC_VERSION_MIN_MACOSX); `-` reads standard input
  macho.py check FILE|- ARCH... [--kind KIND] [--id NAME] [--minos VERSION]
                                       exits non-zero unless FILE is a KIND (default dylib; or
                                       executable) with exactly the slices ARCH... (arm64, x86_64),
                                       each with install name NAME and minimum macOS VERSION when
                                       given
  macho.py fat OUT THIN...             joins thin Mach-O files into one universal file, as
                                       `lipo -create -output OUT THIN...` does

tools/release-assets.sh checks the macOS download's universal library with `check` (on the Linux
release runner, which has no lipo or otool); gates/cross.sh makes its universal stand-in library
with `fat` and checks the Go programs it links with `check --kind executable`. On a Mac, lipo -info and otool -l say the same.
"""
import struct
import sys

FAT_MAGIC, FAT_MAGIC_64 = 0xCAFEBABE, 0xCAFEBABF
MH_MAGIC_64 = 0xFEEDFACF
CPU = {0x01000007: "x86_64", 0x0100000C: "arm64"}
CPU_IDS = {v: k for k, v in CPU.items()}
FILETYPE = {1: "object", 2: "executable", 6: "dylib", 8: "bundle"}
LC_ID_DYLIB, LC_VERSION_MIN_MACOSX, LC_BUILD_VERSION = 0xD, 0x24, 0x32
PLATFORM_MACOS = 1


def version(v):
    return f"{v >> 16}.{(v >> 8) & 0xFF}" + (f".{v & 0xFF}" if v & 0xFF else "")


def thin(data, off=0):
    """One 64-bit little-endian Mach-O slice at data[off:]: (arch, filetype, install name, minos)."""
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
    """Every slice of a universal or thin file, in file order."""
    (magic,) = struct.unpack_from(">I", data, 0)
    if magic not in (FAT_MAGIC, FAT_MAGIC_64):
        return [thin(data)]
    (n,) = struct.unpack_from(">I", data, 4)
    out = []
    for i in range(n):
        if magic == FAT_MAGIC:
            _cpu, _sub, off, _size, _align = struct.unpack_from(">5I", data, 8 + 20 * i)
        else:
            _cpu, _sub, off, _size, _align, _res = struct.unpack_from(">2I2Q2I", data, 8 + 32 * i)
        out.append(thin(data, off))
    return out


def read(path):
    if path == "-":
        return sys.stdin.buffer.read()
    with open(path, "rb") as f:
        return f.read()


def fat(out, inputs):
    """A universal file of the thin inputs, each slice aligned to 16 KiB (arm64's page; lipo
    aligns x86_64 slices to 4 KiB, and either is valid for both)."""
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
