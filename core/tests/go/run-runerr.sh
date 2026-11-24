# Builds and runs `go test` checks verifying that Go misuse and panics unwind
# cleanly into returned errors while running deferred cleanup functions.
rm -f "$bin/runerr-test"
if ! (cd "$here" && CGO_LDFLAGS="-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" go test -c -tags "$tags" -o "$bin/runerr-test" ./runerr) \
    > "$out/build-runerr-test.log" 2>&1; then
  echo "FAIL build runerr-test: go test -c ./runerr"; head -20 "$out/build-runerr-test.log"
fi
# Proves using a stale *gk.Cx returns a structured error with line info, runs deferred cleanup,
# and exits Run cleanly without crashing the test binary (PASS).
GK_PROTO_BIN=runerr-test run gotest-misuse 0 \
  'runerr misuse error="a \*gk.Cx was used after the function it" line=true deferred=true after=0' '^PASS$'
# Proves a Go runtime panic (nil map write) inside app code is caught with stack/location details,
# runs deferred cleanup, and unwinds into Run's returned error so go test passes (PASS).
GK_PROTO_BIN=runerr-test run gotest-panic 0 \
  'runerr panic value="assignment to entry in nil map" runtime_error=true where="app code" stack=true deferred=true after=0' '^PASS$'
