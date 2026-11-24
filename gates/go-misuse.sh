#!/bin/bash
# Misuse, safety-contract, and compile-time type-safety gate for Go (`gates/go-misuse.sh`).
#
# What it gates:
#   Verifies that invalid Go usage fails fast with clear, actionable diagnostics instead of
#   crashing in Rust or silently corrupting state, and that Go's static type system rejects wrong
#   element, handler, token, fill, color, and action types at compile time.
#
# How it measures:
#   - Runtime cases (`run <case> <want_rc> <regex>`): launches `bin/<NEG_BIN>-go` offscreen on
#     `lib/diag` (60 s timeout) with `GK_NEG=<case>` across 8 binaries (`misuse`, `misuse_rows`,
#     `misuse_data`, `misuse_overlay`, `misuse_values`, `misuse_lists`, `misuse_editor`,
#     `misuse_review`), asserting exact exit status and error message regex.
#   - Source-location check (`lines`): confirms all 15 app-originated stops in `misuse` cite
#     `misuse/main.go:<line>` so users see their own call site rather than internal runtime frames.
#   - Compile-fail cases (`compile <pkg> <regex>`): builds each package under
#     `examples/go/misuse_compile/` expecting a non-zero exit and the matching Go compiler error.
#   - Environment/build-mode checks: verifies `hello-go` fails cleanly with exit 1 when no display
#     is set, `CGO_ENABLED=0` builds via purego, and `CGO_ENABLED=0 -tags=gkstatic` stops first on
#     the explicit cgo guard symbol.
#
# What PASS means (exit 0):
#   Every single runtime, line-number, compile-fail, and build-mode check passes (`pass == total`),
#   printing `GATE misuse go PASS <N> of <N> cases`.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
out=$here/out/go/misuse
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0

# Run one GK_NEG scenario against $NEG_BIN-go (default misuse-go) and verify exit code + regex.
run() {
  total=$((total + 1))
  bin=${NEG_BIN:-misuse}
  log=$out/$1.log
  [ "$bin" = misuse ] || log=$out/$bin-$1.log
  GK_NEG=$1 timeout 60 "${GK_BIN:-$repo/bin}/$bin-go" > "$log" 2>&1
  rc=$?
  if [ $rc -eq "$2" ] && grep -Eq -- "$3" "$log"; then
    pass=$((pass + 1)); echo "PASS $1: exit $rc; $(grep -Em1 -- "$3" "$log" | cut -c1-150)"
  else
    echo "FAIL $1: exit $rc (wanted $2 and /$3/)"; tail -5 "$log"
  fi
}
pending() {
  echo "GATE misuse $1 PENDING needs $2"
}

# Core runtime misuse cases (context/element lifetimes, threads, UTF-8, NUL, panics, and data columns).
run rune58 0 'done mode=rune58 clicks=3 selected="item 0 of build after 2 clicks"'
run keep 0 'done mode=keep clicks=3 selected="" kept="typed text"'
run cx 1 'a \*gk.Cx was used after the function it was given to returned'
run element 1 'an element was used after the view function that made it returned'
run twice 1 'an element was given to a second parent'
run self 1 'an element was made a child of itself'
run view 1 'a view function may not create views'
run usize 1 'a negative number \(-1\) was given where the kit takes a count'
run nil 1 'a nil function was given as a handler'
run nil-child 1 'a nil element was given to a builder'
run zero 1 'never made'
run built 1 'a builder chain was used after Build'
run utf8 1 'a string given to the kit is not UTF-8 \(byte 0xff at offset 2 of 4'
run nul 0 'gk-neg nul len=3 same=true'
run goroutine 1 'not on the UI thread: only gk.Post may be called there'
run release 1 'through a handle that cannot be used'
run row-write 1 'a view function may not write'
run data-column 1 'a builder names the column "month", which the rows of this data \(main.sale\) do not have: Month, Total'
run panel-name 1 'a panel needs a name \(PanelData.Name\)'
run app-panic 2 'gk: panic in app code: runtime error: index out of range'
run go-panic 2 'gk: panic in the work function of the gk.Go at .*misuse/main.go:[0-9]+: assignment to entry in nil map'

# Confirm all 15 app-originated stops point directly to the offending line in misuse/main.go.
total=$((total + 1))
if grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log > /dev/null 2>&1 &&
   [ "$(grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log | wc -l)" = 15 ]; then
  pass=$((pass + 1)); echo "PASS lines: the 15 stops in app code name main.go and the line"
else
  echo "FAIL lines: a stop does not name main.go:LINE"; grep -L 'misuse/main.go:' "$out"/*.log
fi

# Virtual rows, table row-count overflow, and released row/input handle cases.
NEG_BIN=misuse_rows
run table-freed 1 'a view shows a row source through a handle that cannot be used'
run row-reenter 1 'a view function may not write: it only describes \(gk.Release\)'
run row-handler 0 'gk-neg row-handler clicked text="row 0 of a list"'
run huge-count 1 'table.NewSource was given 3000000000 rows of 2 columns; a table holds at most'
run input-release 1 'a view shows a .*InputBaseState<.*InputMode> through a handle that cannot be used'

# Data-table template validation: unknown columns, wrong kind, bad index, unclosed brace, unsupported field type.
NEG_BIN=misuse_data
run data-ok 0 'gk-neg done mode=data-ok drawn=1'
run data-column 1 'names the column "Dekstop", which the rows of this data \(main.month\) do not have: Month, Desktop'
run data-kind 1 'reads the column "Month" of main.month as a number, and it holds text'
run data-number 1 'asks for number 2, and this builder hands it 1'
run data-brace 1 'has a \{ that is not closed'
run data-row 1 'names the column "Month", and this builder has no row to read it from'
run data-field 1 'the field Parts of main.bad is a \[\]int, which a data table cannot carry'

# Built views (Dialog) and dock panel registration/node ID misuse.
NEG_BIN=misuse_overlay
run built-ok 0 'gk-neg done mode=built-ok built=[1-9][0-9]* active=true'
run built-freed 1 'a view shows a Dialog \(the view gk.NewBuilt made\) through a handle that cannot be used'
run built-view 1 'gk.NewBuilt in a view function'
run built-none 1 'no view given: make it with gk.NewBuilt'
run dock-node 1 'node id 424242 was never handed out by the dock'
run dock-register 1 'dock.RegisterPanel in a view function'

# Value builders, self-reference, actions in views, invalid time, released scroll handles, search rules, tooltips.
NEG_BIN=misuse_values
run val-ok 0 'gk-neg done mode=val-ok drawn=[0-9]* clicks=2'
run val-self 1 'a value was given as an argument to its own builder'
run val-self-view 1 'a value was given as an argument to its own builder'
run val-write-view 1 'a view function may not write'
run val-action-view 1 'gk.ActionOf in a view function'
run val-bad-time 1 'the time 24:00:00.000000000 does not exist'
run val-scroll-freed 1 'a view shows a .*ScrollHandle through a handle that cannot be used'
run c6-rules-source 1 'gk.SearchWillChange was given a row source gk.NewSearchSource did not make'
run c6-rules-limits 1 'min 3 and max 1 cannot both hold'
run c6-tooltip-kept 1 'was used after the function that made it returned'

# Search/select row sources, expression parser errors, NonZero bounds, and writes during view builds.
NEG_BIN=misuse_lists
run sel-ok 0 'gk-neg sel-ok value="b" ok=true'
run sel-foreign 1 'gk.SetSearchItems was given a row source gk.NewSearchSource did not make'
run sel-view 1 'a view function may not write'
run expr-syntax 1 'the expression "x <" does not parse'
run expr-arg 1 'the expression "z \* 2" uses z, but the closure is handed 2 argument'
run expr-func 1 'the expression "clamp\(x, 1\)" calls clamp with 2 argument\(s\); it takes 3'
run nonzero 1 '0 was given where the kit takes a number that is never 0'
run kept-view 1 'a view function may not write'

# Editor plugins, owner handle validation, temporary leaf lifetimes, and lent view values.
NEG_BIN=misuse_editor
run ed-ok 0 'gk-neg done mode=ed-ok drawn=[1-9][0-9]* rows=[1-9]'
run ed-owner 1 'is not an input, a textarea or an editor'
run ed-owner-released 1 'a view shows a kit state \(as an entity id\) through a handle that cannot be used'
run ed-plugin-freed 1 'a view shows a row source through a handle that cannot be used'
run ed6-ok 0 'gk-neg done mode=ed6-ok drawn=[1-9][0-9]* rows=[1-9][0-9]* leaves=[1-9][0-9]*'
run ed6-held-stale 1 'cannot be used'
run ed6-leaf-stale 1 'is temporary: it lives until the next frame begins'
run ed6-decorate-freed 1 'a view shows a row source through a handle that cannot be used'
run ed6-lent-view 1 'a value made in a view function was given to a call'
run ed7-ok 0 'gk-neg done mode=ed7-ok drawn=[1-9][0-9]* rows=0 leaves=[1-9][0-9]*'
run ed7-custom-unretained 1 'is temporary|cannot be used'

# Timers, window/panel release checks, key-binding writes in view, and temporary handle expiry.
NEG_BIN=misuse_review
run rv-ok 0 'gk-neg done mode=rv-ok drawn=[0-9]* clicks=2 fired=1 closed=1 acted=[1-9]'
run rv-timer-view 1 'a view function may not start timers'
run rv-timer-zero 1 'a timer of 0s: give 1 ms'
run rv-window-stale 1 '\(window\) was released'
run rv-keys-view 1 'a view function may not write'
run rv-temp-later 1 'temporary: it lives until the next frame begins'
run rv-panel-released 1 'was released'
NEG_BIN=misuse

# Compile-fail checks: each package in examples/go/misuse_compile must fail `go build` with the expected error.
compile() {
  total=$((total + 1))
  (cd "$repo/examples/go" && go build -o /dev/null "./misuse_compile/$1") > "$out/compile-$1.log" 2>&1
  rc=$?
  if [ $rc -ne 0 ] && grep -Eq -- "$2" "$out/compile-$1.log"; then
    pass=$((pass + 1)); echo "PASS compile-$1: does not build; $(grep -Em1 -- "$2" "$out/compile-$1.log" | sed 's/.*main.go:/main.go:/' | cut -c1-150)"
  else
    echo "FAIL compile-$1: exit $rc (wanted a build error matching /$2/)"; tail -5 "$out/compile-$1.log"
  fi
}
compile handler_type 'cannot use func\(cx \*gk.Cx, e gk.ClickEvent\) \{.*\} .* as func\(cx \*gk.Cx, e bool\) value'
compile not_element 'cannot use 42 .* as gk.Element value'
compile token_color 'cannot use s.Gap2 .* as gk.Color value'
compile el_types 'cannot use kit.Button\("b"\) .* as kit.LabelEl value'
compile no_method 'OnClick undefined'
compile value_child 'cannot use gk.NewStyleRefinementDefault\(\) .* as gk.Element value'
compile effect_target 'gk.Text\("x"\) .* does not implement (gk|rt).AnimatableElement'
compile shortcut_type 'does not satisfy gk.ShortcutHandler'
compile fill_token 'cannot use s.Gap2 .* as gk.Fill value.*does not implement (gk|rt).Fill'
compile gradient_color 'cannot use g .* as gk.Color value'
compile built_root 'gk.Built\[kit.LabelEl\] does not satisfy gk.Builder\[overlay.DialogEl\]'
compile action_forged 'cannot refer to unexported field id in struct literal of type gk.Action'

# Startup and build-mode checks: missing display error, purego no-cgo build, and static-tag cgo guard.
total=$((total + 1))
timeout 30 "${GK_BIN:-$repo/bin}/hello-go" > "$out/no-display.log" 2>&1; rc=$?
if [ $rc = 1 ] && grep -q 'the app has no display to open its window on' "$out/no-display.log"; then
  pass=$((pass + 1)); echo "PASS no-display: exit 1; $(head -1 "$out/no-display.log" | cut -c1-120)"
else
  echo "FAIL no-display: exit $rc (wanted 1 and the no-display message)"; head -3 "$out/no-display.log"
fi
total=$((total + 1))
(cd "$repo/examples/go" && CGO_ENABLED=0 go build -tags= -o /dev/null ./hello) > "$out/compile-nocgo.log" 2>&1
if [ $? -eq 0 ]; then
  pass=$((pass + 1)); echo "PASS compile-nocgo: CGO_ENABLED=0 builds (the library is loaded at start-up, through purego)"
else
  echo "FAIL compile-nocgo: CGO_ENABLED=0 does not build"; head -5 "$out/compile-nocgo.log"
fi
total=$((total + 1))
(cd "$repo/examples/go" && CGO_ENABLED=0 go build -tags=gkstatic -o /dev/null ./hello) > "$out/compile-static-nocgo.log" 2>&1
if [ $? -ne 0 ] && grep -v '^#' "$out/compile-static-nocgo.log" | head -1 | grep -q 'undefined: gk_static_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1_or_drop_tag_gkstatic'; then
  pass=$((pass + 1)); echo "PASS compile-static-nocgo: -tags gkstatic with CGO_ENABLED=0 stops first at $(grep -v '^#' "$out/compile-static-nocgo.log" | head -1 | sed 's/.*gk\//gk\//' | cut -c1-120)"
else
  echo "FAIL compile-static-nocgo: the first error is not the cgo one"; head -5 "$out/compile-static-nocgo.log"
fi
compile cx_handler 'cannot use func\(e gk.ClickEvent\) \{.*\} .* as func\(cx \*gk.Cx, e gk.ClickEvent\) value'
echo "GATE misuse go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total cases"
[ $pass = $total ]
