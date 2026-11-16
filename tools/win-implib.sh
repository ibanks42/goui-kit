#!/bin/bash
# win-implib.sh DIR: the MinGW import library of gpuikit.dll, for Go's cgo on Windows (MinGW-w64
# gcc cannot use the MSVC import library gpuikit.dll.lib reliably, and must never pick up the MSVC
# static library gpuikit.lib, whose C runtime differs). Writes
#   DIR/gpuikit.def        LIBRARY gpuikit.dll and the C interface's functions, read from
#                          core/include/gpuikit.h (the seven functions of DESIGN.md section 5)
#   DIR/libgpuikit.dll.a   made from it; `-L DIR -lgpuikit` finds it before anything else
# The DLL itself is not needed: the names come from the header, which is the interface. Any
# dlltool does: DLLTOOL, else MinGW's dlltool, llvm-dlltool, or `zig dlltool` (ZIG, a zig binary).
# Runs on Windows (tools/build-lib.sh) and on Linux (gates/cross.sh).
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
dir=${1:?usage: win-implib.sh DIR}
mkdir -p "$dir"
names=$(grep -v '^typedef' "$repo/core/include/gpuikit.h" | grep -oE '\bgk_[a-z_]+\(' | tr -d '(' | sort -u)
[ -n "$names" ] || { echo "win-implib.sh: no functions found in core/include/gpuikit.h" >&2; exit 1; }
{ echo "LIBRARY gpuikit.dll"; echo "EXPORTS"; for n in $names; do echo "    $n"; done; } > "$dir/gpuikit.def"
if [ -n "${DLLTOOL:-}" ]; then read -r -a tool <<< "$DLLTOOL"
elif command -v x86_64-w64-mingw32-dlltool > /dev/null; then tool=(x86_64-w64-mingw32-dlltool)
elif command -v dlltool > /dev/null; then tool=(dlltool)
elif command -v llvm-dlltool > /dev/null; then tool=(llvm-dlltool)
elif [ -n "${ZIG:-}" ]; then tool=("$ZIG" dlltool)
else echo "win-implib.sh: no dlltool (install MinGW-w64 binutils or LLVM, or set DLLTOOL or ZIG)" >&2; exit 1
fi
"${tool[@]}" -m i386:x86-64 -d "$dir/gpuikit.def" -l "$dir/libgpuikit.dll.a"
echo "win-implib: $dir/libgpuikit.dll.a ($(echo $names | wc -w) functions, ${tool[*]})"
