#!/bin/bash
# go-proto.sh [MODE...]: the protocol tests of the Go front end (examples/go/proto, the port of
# core/tests/proto), offscreen with the diag library. Same modes and checks as core/tests/run.sh
# where the protocol is the same; where Go differs the check says so. Prints one line per check
# and "GATE proto go PASS|FAIL n of m". Needs bin/proto-go (examples/go/build.sh proto).
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$here/out/go/proto
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0
check() {
  mode=$1; want=$2; shift 2
  log=$out/$mode.log
  if [ ! -e "$log" ]; then
    GK_PROTO=$mode timeout 60 "$bin/${GK_PROTO_BIN:-proto-go}" > "$log" 2>&1
    echo $? > "$out/$mode.rc"
  fi
  rc=$(cat "$out/$mode.rc")
  total=$((total + 1))
  if [ "$rc" != "$want" ]; then
    echo "FAIL $mode: exit $rc, wanted $want"; tail -5 "$log"; return
  fi
  for pattern in "$@"; do
    if ! grep -Eq -- "$pattern" "$log"; then
      echo "FAIL $mode: exit $rc, but no line matches /$pattern/"; tail -5 "$log"; return
    fi
  done
  pass=$((pass + 1)); echo "PASS $mode: exit $rc; $(grep -Em1 -- "${1:-.}" "$log" | cut -c1-140)"
}
wanted() { [ ${#only[@]} -eq 0 ] && return 0; for m in "${only[@]}"; do [ "$m" = "$mode_name" ] && return 0; done; return 1; }
run() { mode_name=$1; if wanted; then check "$@"; fi; }
only=("$@")

run handlers 0 \
  'proto click n=1 button=Left count=1 kind=Mouse x=36 y=32' \
  'proto returns true=1 false=0' \
  'proto value back="payload/7"' \
  'proto value back="with-data/8/9/row-9"' \
  'proto value default=""' \
  'proto toggle checked=true' 'proto toggle checked=false' \
  'proto hover inside=true' 'proto hover inside=false' \
  'proto row id=5 name="row 5" button=Left' \
  'proto mouse button=Left count=1' \
  'proto key key="k" char="k" has_char=true held=false' \
  'proto key key="enter" .*mods=.*\{Shift\}' \
  'proto done mode=handlers views=[0-9]+ .*clicks=1'
# Odin's `shape` and `state-type` modes (a handler that does not fit stops at registration) are
# compile errors in Go: examples/go/misuse_compile, checked by gates/go-misuse.sh.
run state 0 \
  'proto echo view=1 frame=0 value="abc"' \
  'proto input change value="abcx"' 'proto input change value="abcxy"' \
  'proto echo view=[0-9]+ frame=0 value="abcxy"' \
  'proto input enter secondary=false shift=false' \
  'proto read-after-write value="written"' \
  'proto unsubscribed' \
  'proto echo view=[0-9]+ frame=0 value="writtenz"' \
  'proto done mode=state .*events=3 '
run stale 2 'handle 0x[0-9a-f]+ \(state\) was released' 'at .*proto/main.go:[0-9]+'
run stale-write 2 'proto stale-write buffered' 'a change a handler asked for could not be made: handle 0x[0-9a-f]+ \(state\) was released' 'GK_DEBUG=1'
run wrong-type 2 'handle 0x[0-9a-f]+ is a rows, not a state' 'at .*proto/main.go:[0-9]+'
run lease 0 \
  'proto lease busy=true' \
  'proto lease ret=1 value_after="abc"' \
  'proto queued not yet' \
  'proto queued row=42 ok=true value="written inside the update"'
run write-rule 0 \
  'proto echo view=[0-9]+ frame=1 value="frame 1"' \
  'proto echo view=[0-9]+ frame=2 value="frame 2"' \
  'proto echo view=[0-9]+ frame=3 value="frame 3"' \
  'frame_turns: [34],'
run frames 0 'proto done mode=frames views=1 .*frame=4 ' 'frame_turns: 4,' 'views_turns: 1,'
run view-write 2 'a view function may not write' 'at .*proto/main.go:[0-9]+'
run model 0 \
  'proto side view run=1 count=0' \
  'proto model set count=1 observer_saw=1' \
  'proto side view run=2 count=1' \
  'proto model set count=2 observer_saw=2' \
  'proto side view run=3 count=2' \
  'proto done mode=model views=1 sides=3 '
run post 0 \
  'proto post batch items=3 first="item 0" last="item 2" nums=4 sum=10' \
  'proto done mode=post .*count=20000 seen=20000 ticks=20001 sent=20001' \
  'wake_turns: ([2-9]|1[0-9]|2[0-2]),' \
  'views_turns: ([2-9]|1[01]),'
run panic-go 2 "gk: panic in app code: the app.s own panic" 'proto/main.go:[0-9]+'
run panic-call 1 'gk: the library panicked: test panic asked for by the app' 'in the call at .*proto/main.go:[0-9]+'
run panic-later 1 'gk: the library panicked: test panic asked for by the app.*while applying buffered writes'
run thread 2 'was called from a goroutine that is not on the UI thread'
run version 0 \
  'proto version hash rc=8 message="the binding and the library are from different versions' \
  'proto version major rc=8' 'proto version newer-minor rc=8' \
  'proto version size rc=8 message="the binding passed a gk_app of [0-9]+ bytes' \
  'proto version callback rc=8'
run released-shown 2 'proto released-shown released' \
  'a view shows a .*InputBaseState.* through a handle that cannot be used: handle 0x[0-9a-f]+ \(state\) was released'
# Go: posted closures are the collector's. 300,000 lines of 100 bytes queued at once, twice: once
# delivered nothing of them is kept (the heap after a collection is under 10 MB; Odin's check is its
# own owned-bytes count).
run post-memory 0 'proto post-memory round=2 ' 'proto done mode=post-memory .*' 'proto post-memory done lines=600000 .*heap_kb=([0-9]{1,4})$'
# Handlers of a view described on every frame with new closures: the heap stays small (Go: the
# collector frees the old tables; Odin's check is its owned-bytes count).
run handler-memory 0 'proto handler-memory done frames=60[01] .*heap_kb=([0-9]{1,4})$'
run rows 0 \
  'proto row clicked row=5000' \
  'proto source request id=1 text="ping"' \
  'proto answered ok=true text="pong ping" bound=20' \
  'rows_turns: 1,' 'placeholder_frames: 1 ' 'rows_missing: [1-9][0-9]?,' \
  'proto rows described=[0-9]+ again=0'
# The kit's List over a list source (PROTOCOLS.md 9.5; Odin: core/tests/run-rows.sh list): the
# first frame complete; a click selects and confirms, Down selects the next row, a right click,
# Escape; a far scroll draws placeholders for one frame; back at the top nothing is described again.
run list 0 \
  'proto list Select has=true section=0 row=0 ' 'proto list Confirm has=false .*secondary=false' \
  'proto list Select has=true section=0 row=1 ' 'proto list RightClick has=true section=0 row=[0-9]+ ' \
  'proto list Cancel ' 'proto list-state read selected_index ok=true row=0' 'placeholder_frames: 0 ' 'placeholder_frames: 1 ' \
  'rows_turns: [1-9],' 'proto rows described=[0-9]+ again=0$'
# A dialog over a view-built closure (PROTOCOLS.md 11.6; Odin: core/tests/run-overlay.sh dialog):
# described before the kit first builds it, described again while open, escape runs on_cancel of
# the second description, opened again on the same view, closed by the app.
run dialog 0 \
  'proto dialog opened' 'proto dialog described n=1 count=0' 'proto dialog active=true' \
  'proto dialog described n=2 count=1' 'proto dialog cancel n=1 seen=1 count=1' \
  'proto dialog closed active=false' \
  'proto dialog done active=false described=[3-9] cancels=1 count=1'
# Go only: a *gk.Cx kept past its turn, and an element kept across builds, stop with the line.
run stale-cx 2 'a \*gk.Cx was used after the function it was given to returned' 'at .*proto/main.go:[0-9]+'
run stale-element 2 'an element was used after the view function that made it returned' 'at .*proto/main.go:[0-9]+'
# The port round (PROTOCOLS.md 11.38, 12.17, and the Go ports of 11.17, 11.30, 13.20, 13.31):
# examples/go/port, bin/port-go. The first five are the Odin twins of core/tests/run-app.sh's modes.
port() { mode_name=$1; if wanted; then GK_PROTO_BIN=port-go check "$@"; fi; }
port close-veto 0 'port asked may close n=1 answer=false' 'port asked may close n=2 answer=true' \
  'port asked by a second handler n=3' \
  'port should close first=false second=true third=true fourth=true asked=3 windows=1'
port quit-hook 0 'port quit hook ran n=1' 'port after the run quits=1'
port reduce-motion 0 'port reduce motion in view=' 'port reduce motion start=false set=true cleared=false'
port window-icon 0 'port icon window opened n=2' 'port second view described'
port bad-icon 2 'the window.s icon is not a PNG the library can read \(4 bytes\)' 'at .*port/main.go:[0-9]+'
# A drag of value 7 (its preview a view of the app's) onto a target whose CanDrop takes 7: its drop
# runs; onto one that takes only 8: none (drops stays 1). What a DropAccept takes, as decoded.
port drag 0 'port drag preview described' 'port drop on seven id=7 text=seven drops=1' \
  'port drop accept 7=true 8=false any=true none=false files=false,true' 'port drag done drops=1 previewed=true'
# The menu bar set and read back: two menus, a separator, a submenu with an OS action, checked; an
# item without an action stops with its name.
port menus 0 'port menus Port:4,Edit:1 edit_disabled=true sub=More copy_os=true checked=true quit_same=true'
port menu-no-action 2 'the item "Open" has no action' 'at .*port/main.go:[0-9]+'
# Images from encoded bytes, from pixels and through the custom loader drawn; pixels as gpui keeps
# them (BGRA). An image made once and named on every redraw is handed to gpui as one image (one id,
# so one texture); released, its texture is dropped from the window (no new texture per frame,
# PROTOCOLS.md 13.31). Pixels of the wrong size stop; so does an image made in a view function.
port images 0 'port pixel image 2x1 first=ffff0000 custom 2x1 first=ffff0000 same=true' \
  'port image kept redrawn=true same=true new images=0' 'port image released dropped=1'
port bad-pixels 2 'a gk.PixelImage of 2x2 pixels holds 3 bytes' 'at .*port/main.go:[0-9]+'
port image-in-view 2 'a view function may not make an image' 'at .*port/main.go:[0-9]+'
# A dropdown's item clicked after its view was described again ten times while the menu was open:
# its own OnClick fires exactly once, as an action item does (PROTOCOLS.md section 4).
port menu-redraw 0 'port menu fired alpha=1 bravo=0 redrawn=true'
port menu-redraw-b 0 'port menu fired alpha=0 bravo=1 redrawn=true'
# An element's tooltip: a view of the app's shown after resting on the element, the kit's tooltip
# with an element of the app's, and with text.
port tooltip 0 'port tooltip view described' 'gk-headless: wrote tooltip-kit.png'
# A plot of the kit's grid and the binding's own paths and quads, with a gradient and theme colours,
# and options; a nil gk.Fill stops.
port plot 0 'port plot described=true'
port fill-nil 2 'a nil gk.Fill was given to a builder' 'at .*port/main.go:[0-9]+'
mode_name=version-lib
if wanted; then
  log=$out/version-lib.log
  GK_PROTO_STALE_HASH=1 timeout 60 "$bin/proto-go" > "$log" 2>&1; echo $? > "$out/version-lib.rc"
  check version-lib 1 'gk: the library and this package are from different versions: the library is ABI [0-9.]+ with interface hash'
fi
echo "GATE proto go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
