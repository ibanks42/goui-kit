#!/bin/bash
# go-misuse.sh: the Go front end's misuse gate (the port of gates/misuse.sh). Run-time cases
# (examples/go/misuse, offscreen with the diag library): each must behave correctly or stop with
# its message; exit 0 with wrong content fails. Compile-time cases (examples/go/misuse_compile):
# each must fail to build, with the compiler's message naming the mistake. Prints
# "GATE misuse go ...". Needs bin/misuse-go (examples/go/build.sh misuse).
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
out=$here/out/go/misuse
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0
run() { # mode expected-exit pattern
  total=$((total + 1))
  GK_NEG=$1 timeout 60 "${GK_BIN:-$repo/bin}/misuse-go" > "$out/$1.log" 2>&1
  rc=$?
  if [ $rc -eq "$2" ] && grep -Eq -- "$3" "$out/$1.log"; then
    pass=$((pass + 1)); echo "PASS $1: exit $rc; $(grep -Em1 -- "$3" "$out/$1.log" | cut -c1-150)"
  else
    echo "FAIL $1: exit $rc (wanted $2 and /$3/)"; tail -5 "$out/$1.log"
  fi
}
run rune58 0 'done mode=rune58 clicks=3 selected="item 0 of build after 2 clicks"'
run keep 0 'done mode=keep clicks=3 selected="" kept="typed text"'
run cx 2 'a \*gk.Cx was used after the function it was given to returned'
run element 2 'an element was used after the view function that made it returned'
run twice 2 'an element was given to a second parent'
run self 2 'an element was made a child of itself'
run view 2 'a view function may not create views'
run usize 2 'a negative number \(-1\) was given where the kit takes a count'
run nil 2 'a nil function was given as a handler'
run nil-child 2 'a nil element was given to a builder'
run zero 2 'never made'
run built 2 'a builder chain was used after Build'
run utf8 2 'a string given to the kit is not UTF-8 \(byte 0xff at offset 2 of 4'
run nul 0 'gk-neg nul len=3 same=true'
run goroutine 2 'not on the UI thread: only gk.Post may be called there'
run release 2 'through a handle that cannot be used'
run row-write 2 'a view function may not write'
run data-column 2 'a builder names the column "month", which the rows of this data \(main.sale\) do not have: Month, Total'
run panel-name 2 'a panel needs a name \(PanelData.Name\)'
run app-panic 2 'gk: panic in app code: runtime error: index out of range'
run go-panic 2 'gk: panic in the work function of the gk.Go at .*misuse/main.go:[0-9]+: assignment to entry in nil map'
# Every run-time stop names the app's line.
total=$((total + 1))
if grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log > /dev/null 2>&1 &&
   [ "$(grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log | wc -l)" = 15 ]; then
  pass=$((pass + 1)); echo "PASS lines: the 15 stops in app code name main.go and the line"
else
  echo "FAIL lines: a stop does not name main.go:LINE"; grep -L 'misuse/main.go:' "$out"/*.log
fi
compile() { # case pattern
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
compile el_types 'cannot use gk.Button\("b"\) .* as gk.LabelEl value'
compile no_method 'OnClick undefined'
compile value_child 'cannot use gk.NewStyleRefinementDefault\(\) .* as gk.Element value'
compile effect_target 'gk.Text\("x"\) .* does not implement gk.AnimatableElement'
compile shortcut_type 'does not satisfy gk.ShortcutHandler'
# The port round (PROTOCOLS.md 11.38): .Bg takes a gk.Fill (a colour or a gradient); a gradient fits
# nowhere a single colour goes.
compile fill_token 'cannot use s.Gap2 .* as gk.Fill value.*does not implement gk.Fill'
compile gradient_color 'cannot use g .* as gk.Color value'
# No display and no script: GPUI would wait without a word; gk.Run says so (exit 1).
total=$((total + 1))
timeout 30 "${GK_BIN:-$repo/bin}/hello-go" > "$out/no-display.log" 2>&1; rc=$?
if [ $rc = 1 ] && grep -q 'the app has no display to open its window on' "$out/no-display.log"; then
  pass=$((pass + 1)); echo "PASS no-display: exit 1; $(head -1 "$out/no-display.log" | cut -c1-120)"
else
  echo "FAIL no-display: exit $rc (wanted 1 and the no-display message)"; head -3 "$out/no-display.log"
fi
# Without cgo (CGO_ENABLED=0, or no C compiler) the first error says what to do.
total=$((total + 1))
(cd "$repo/examples/go" && CGO_ENABLED=0 go build -o /dev/null ./hello) > "$out/compile-nocgo.log" 2>&1
if [ $? -ne 0 ] && grep -v '^#' "$out/compile-nocgo.log" | head -1 | grep -q 'undefined: gk_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1'; then
  pass=$((pass + 1)); echo "PASS compile-nocgo: CGO_ENABLED=0 stops first at $(grep -v '^#' "$out/compile-nocgo.log" | head -1 | sed 's/.*gk\//gk\//' | cut -c1-120)"
else
  echo "FAIL compile-nocgo: the first error is not the cgo one"; head -5 "$out/compile-nocgo.log"
fi
compile cx_handler 'cannot use func\(e gk.ClickEvent\) \{.*\} .* as func\(cx \*gk.Cx, e gk.ClickEvent\) value'
echo "GATE misuse go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total cases"
[ $pass = $total ]
