# run-gen.sh: the generated protocol tests in Go (core/tests/go/gen, written by `make gen` from
# gen/gk_tests_go.py), sourced by core/tests/go/run.sh, which defines build and run. Odin twin:
# core/tests/run-gen.sh, the same modes and patterns.
#   shapes  every handler shape the package registers, fired once through the library (call op
#           TEST_FIRE_SHAPE): typed where the library can build the payload as the Rust types a
#           listener hands over (through their own Wire impls), raw otherwise; the handler gets
#           the bytes sent (encoded again by the test's own generated encoders: Go has no
#           gk.test_wire), its return value comes back, the typed flag is the predicted one
#   states  every generated state type with a constructor the test can call: made through its
#           builder chain, every read, write and call once, every event subscribed, read again
#           after the writes were applied, released
build gen-go gen
GK_PROTO_BIN=gen-go run shapes 0 'shapes done ok=([0-9]+) failed=0 typed=[0-9]+ of \1$' 'gen done mode=shapes clicks=2 .*failed=0'
GK_PROTO_BIN=gen-go run states 0 'states made all n=([0-9]+)$' 'states released n=[0-9]+ subscriptions=[0-9]+' 'gen done mode=states clicks=2'
