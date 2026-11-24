# run-runerr.sh: gk.Run's contract under go test (core/tests/go/runerr, docs/GO.md section 5),
# sourced by core/tests/go/run.sh. The package's test binary (go test -c) runs the app in TestMain.
rm -f "$bin/runerr-test"
if ! (cd "$here" && CGO_LDFLAGS="-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" go test -c -tags "$tags" -o "$bin/runerr-test" ./runerr) \
    > "$out/build-runerr-test.log" 2>&1; then
  echo "FAIL build runerr-test: go test -c ./runerr"; head -20 "$out/build-runerr-test.log"
fi
# A mistake in app code: RunHeadless returns the *gk.Error with the app's line, the caller's
# defer runs, no handler runs after the stop, and the tests pass.
GK_PROTO_BIN=runerr-test run gotest-misuse 0 \
  'runerr misuse error="a \*gk.Cx was used after the function it" line=true deferred=true after=0' '^PASS$'
# A panic in app code: RunHeadless panics again with a *gk.PanicError (errors.As finds the
# runtime.Error), after the app has shut down; recover in the caller works.
GK_PROTO_BIN=runerr-test run gotest-panic 0 \
  'runerr panic value="assignment to entry in nil map" runtime_error=true where="app code" stack=true deferred=true after=0' '^PASS$'
