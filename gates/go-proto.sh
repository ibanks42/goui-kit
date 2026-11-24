#!/bin/bash
# Wire-protocol and runtime-behavior gate for Go (`gates/go-proto.sh [mode...]`).
#
# What it gates:
#   End-to-end correctness of the Go <-> Rust C ABI and byte tape protocol across 51 scenarios:
#   - Events and handler payloads (`handlers`)
#   - State reads, buffered writes, subscriptions, and stale/wrong-type handle checks
#   - Re-entrant leases, frame/write rules, model observers, and batched `gk.Post` wakeups
#   - Panic propagation across Go app code, immediate Rust calls, and deferred write flushes
#   - UI-thread enforcement and ABI version / interface-hash handshake validation
#   - Bounded Go heap growth under high-volume `gk.Post` and handler loops (`< 10,000 kB`)
#   - Virtual row sources, list events, dialogs, and stale context/element guards
#   - Platform features (`port-go`): close veto, quit hooks, reduce-motion, icons, drag/drop,
#     menus, pixel images, tooltips, plots, and nil fill rejection
#   - Value kinds (dates, times, cells, colors, diffs, expressions), item/tree/element callbacks,
#     and dock/kit bridges
#
# How it measures:
#   Runs each scenario offscreen on `lib/diag` (display vars cleared, 60 s timeout) via
#   `GK_PROTO=<mode>` against `bin/proto-go` (or `bin/port-go` for `port` cases), asserting exact
#   exit code and every required log regex. Passing mode names on the CLI filters to that subset.
#
# What PASS means (exit 0):
#   Every executed scenario exits with its expected status and matches all log assertions,
#   printing `GATE proto go PASS <N> of <N> protocol tests`.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$here/out/go/proto
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0

# Run one mode (reusing its log if pre-populated, as in version-lib) and verify exit code + patterns.
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

# Event dispatch, return values, toggle/hover/row/mouse/key payloads.
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
# State reads/writes, unsubscription, stale handles, type mismatches, and re-entrant leases.
run state 0 \
  'proto echo view=1 frame=0 value="abc"' \
  'proto input change value="abcx"' 'proto input change value="abcxy"' \
  'proto echo view=[0-9]+ frame=0 value="abcxy"' \
  'proto input enter secondary=false shift=false' \
  'proto read-after-write value="written"' \
  'proto unsubscribed' \
  'proto echo view=[0-9]+ frame=0 value="writtenz"' \
  'proto done mode=state .*events=3 '
run stale 1 'handle 0x[0-9a-f]+ \(state\) was released' 'at .*proto/main.go:[0-9]+'
run stale-write 1 'proto stale-write buffered' 'a change a handler asked for could not be made: handle 0x[0-9a-f]+ \(state\) was released' 'GK_DEBUG=1'
run wrong-type 1 'handle 0x[0-9a-f]+ is a rows, not a state' 'at .*proto/main.go:[0-9]+'
run lease 0 \
  'proto lease busy=true' \
  'proto lease ret=1 value_after="abc"' \
  'proto queued not yet' \
  'proto queued row=42 ok=true value="written inside the update"'
# Frame turn rules, read-only view enforcement, model observers, and coalesced background Post wakes.
run write-rule 0 \
  'proto echo view=[0-9]+ frame=1 value="frame 1"' \
  'proto echo view=[0-9]+ frame=2 value="frame 2"' \
  'proto echo view=[0-9]+ frame=3 value="frame 3"' \
  'frame_turns: [34],'
run frames 0 'proto done mode=frames views=1 .*frame=4 ' 'frame_turns: 4,' 'views_turns: 1,'
run view-write 1 'a view function may not write' 'at .*proto/main.go:[0-9]+'
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
# Panic boundaries (Go app panic, immediate FFI call panic, deferred write panic), UI thread check, ABI negotiation.
run panic-go 2 "gk: panic in app code: the app.s own panic" 'proto/main.go:[0-9]+'
run panic-call 1 'gk: the library panicked: test panic asked for by the app' 'in the call at .*proto/main.go:[0-9]+'
run panic-later 1 'gk: the library panicked: test panic asked for by the app.*while applying buffered writes'
run thread 1 'was called from a goroutine that is not on the UI thread'
run version 0 \
  'proto version hash rc=8 message="the binding and the library are from different versions' \
  'proto version major rc=8' 'proto version newer-minor rc=8' \
  'proto version size rc=8 message="the binding passed a gk_app of [0-9]+ bytes' \
  'proto version callback rc=8'
run released-shown 1 'proto released-shown released' \
  'a view shows a .*InputBaseState.* through a handle that cannot be used: handle 0x[0-9a-f]+ \(state\) was released'
# Heap growth ceilings: heap_kb must stay under 10,000 kB (1 to 4 digits).
run post-memory 0 'proto post-memory round=2 ' 'proto done mode=post-memory .*' 'proto post-memory done lines=600000 .*heap_kb=([0-9]{1,4})$'
run handler-memory 0 'proto handler-memory done frames=60[01] .*heap_kb=([0-9]{1,4})$'
# Virtual row sources, list selection/cancel events, dialogs, and stale context/element guards.
run rows 0 \
  'proto row clicked row=5000' \
  'proto source request id=1 text="ping"' \
  'proto answered ok=true text="pong ping" bound=20' \
  'rows_turns: 1,' 'placeholder_frames: 1 ' 'rows_missing: [1-9][0-9]?,' \
  'proto rows described=[0-9]+ again=0'
run list 0 \
  'proto list Select has=true section=0 row=0 ' 'proto list Confirm has=false .*secondary=false' \
  'proto list Select has=true section=0 row=1 ' 'proto list RightClick has=true section=0 row=[0-9]+ ' \
  'proto list Cancel ' 'proto list-state read selected_index ok=true row=0' 'placeholder_frames: 0 ' 'placeholder_frames: 1 ' \
  'rows_turns: [1-9],' 'proto rows described=[0-9]+ again=0$'
run dialog 0 \
  'proto dialog opened' 'proto dialog described n=1 count=0' 'proto dialog active=true' \
  'proto dialog described n=2 count=1' 'proto dialog cancel n=1 seen=1 count=1' \
  'proto dialog closed active=false' \
  'proto dialog done active=false described=[3-9] cancels=1 count=1'
run stale-cx 1 'a \*gk.Cx was used after the function it was given to returned' 'at .*proto/main.go:[0-9]+'
run stale-element 1 'an element was used after the view function that made it returned' 'at .*proto/main.go:[0-9]+'

# Platform/windowing protocol cases hosted in bin/port-go.
port() { mode_name=$1; if wanted; then GK_PROTO_BIN=port-go check "$@"; fi; }
port close-veto 0 'port asked may close n=1 answer=false' 'port asked may close n=2 answer=true' \
  'port asked by a second handler n=3' \
  'port should close first=false second=true third=true fourth=true asked=3 windows=1'
port quit-hook 0 'port quit hook ran n=1' 'port after the run quits=1'
port reduce-motion 0 'port reduce motion in view=' 'port reduce motion start=false set=true cleared=false'
port window-icon 0 'port icon window opened n=2' 'port second view described'
port bad-icon 1 'the window.s icon is not a PNG the library can read \(4 bytes\)' 'at .*port/main.go:[0-9]+'
port drag 0 'port drag preview described' 'port drop on seven id=7 text=seven drops=1' \
  'port drop accept 7=true 8=false any=true none=false files=false,true' 'port drag done drops=1 previewed=true'
port menus 0 'port menus Port:4,Edit:1 edit_disabled=true sub=More copy_os=true checked=true quit_same=true'
port menu-no-action 1 'the item "Open" has no action' 'at .*port/main.go:[0-9]+'
port images 0 'port pixel image 2x1 first=ffff0000 custom 2x1 first=ffff0000 same=true' \
  'port image kept redrawn=true same=true new images=0' 'port image released dropped=1'
port bad-pixels 1 'a gk.PixelImage of 2x2 pixels holds 3 bytes' 'at .*port/main.go:[0-9]+'
port image-in-view 1 'a view function may not make an image' 'at .*port/main.go:[0-9]+'
port menu-redraw 0 'port menu fired alpha=1 bravo=0 redrawn=true'
port menu-redraw-b 0 'port menu fired alpha=0 bravo=1 redrawn=true'
port tooltip 0 'port tooltip view described' 'gk-headless: wrote tooltip-kit.png'
port plot 0 'port plot described=true'
port fill-nil 1 'a nil gk.Fill was given to a builder' 'at .*port/main.go:[0-9]+'

# Value kinds, item/tree callbacks, and dock/kit bridge tests.
run kinds-values 0 \
  'kinds calendar date=2024-02-29 today_ok=true weeks=[56] week_len=7' \
  'kinds calendar range applied=(true|false) start=2026-01-05 end=9' \
  'kinds calendar select=true date=2026-03-14' 'kinds time 23:59:00' \
  'kinds cells made b=true s=first f=1.5' 'kinds cells again b=false s=second f=2.25' \
  'kinds background solid ok=true kind=0 l=0.50 a=1.00' 'kinds background bad ok=false has=false err=true' \
  'kinds color h=0.00 s=1.00 l=0.50 err=<nil>' 'kinds color bad err=true' 'kinds diff count=2 err=<nil>' \
  'kinds expr ok="" bad="does not parse: it ends where a number, an argument or a function was due at byte 3" args="uses y, but the closure is handed 1 argument\(s\)"'
run kinds-bad-date 1 'kinds writing February 30' 'the date 2026-02-30 does not exist' 'at .*proto/kinds.go:[0-9]+'
run kinds-bad-expr 1 'the expression "x \+" does not parse' 'at .*proto/kinds.go:[0-9]+'
run kinds-items 0 'kinds calendar described=(2[89]|[3-4][0-9]) days=(2[89]|3[0-9]|4[0-2]) weekdays=7 active=[01] labels=0'
run kinds-label 0 'kinds calendar described=[0-9]+ days=[0-9]+ weekdays=[0-9]+ active=[0-9]+ labels=([1-9][0-9]+)'
run kinds-tree 0 'kinds tree row ix=0 label=Alpha depth=0 folder=false selected=false' \
  'kinds tree row ix=1 label=Beta depth=0 folder=false selected=false' 'kinds done mode=kinds-tree described=2 '
run kinds-wrong-kind 1 'overlay.Items was given a function of the kind func\(cx \*Cx, v0 int, v1 TreeEntryReads' 'at .*proto/kinds.go:[0-9]+'
run kinds-elements 0 'kinds ask highlight lang=go code="func x"' 'kinds ask image uri=pic.png' \
  'kinds ask counts asked=2 answered=2 gets_more=true' 'kinds asks highlight=1 image=1'
# Dock layout save/load/panel builds and kit bridge integration (cells, tree, virtual list, table).
run bridge-dock 0 'bridges dock made sub=true' 'bridges skin style=1 toggle=false collapsed=true divider=false' \
  'bridges ids group=true panels=2 same=true found=true' 'bridges tree split=true children=1 last_tabs=true panels=2 walk=2 nodes=2' \
  'bridges layout saved .*first:panel.\{"file":"a.go"\}' 'bridges layout loaded err=<nil>' \
  'bridges layout build first n=1 dock=true' 'bridges layout build second n=2 dock=true' \
  'bridges layout again same=true builds=2 described=true' 'bridges layout tree to_state=.*first.*second.* from_state_panels=2' \
  'bridges layout build_panel known=true unknown=false' 'bridges renderer sent=0 answers=0 drawn=0' \
  'bridges dock builds=5' 'bridges done mode=bridge-dock frames=12'
run bridge-kit 0 'bridges kit cells=true tree=true entry="main.go" depth=1 virtual=3 providers=true' \
  'bridges kit table visible=true clock_moves=true' 'bridges done mode=bridge-kit frames=12'

# Simulate an ABI interface-hash mismatch at library startup.
mode_name=version-lib
if wanted; then
  log=$out/version-lib.log
  GK_PROTO_STALE_HASH=1 timeout 60 "$bin/proto-go" > "$log" 2>&1; echo $? > "$out/version-lib.rc"
  check version-lib 1 'gk: the library and this package are from different versions: the library is ABI [0-9.]+ with interface hash'
fi
echo "GATE proto go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
