# Generated API smoke tests exercising every generated node shape and state constructor.
build gen-go gen
# Proves every generated widget builder encodes onto the tape and decodes in Rust with zero failures;
# expects failed=0 and all typed builders accounted for across two click cycles.
GK_PROTO_BIN=gen-go run shapes 0 'shapes done ok=([0-9]+) failed=0 typed=[0-9]+ of \1$' 'gen done mode=shapes clicks=2 .*failed=0'
# Proves every generated state handle can be constructed, subscribed to, and released cleanly.
GK_PROTO_BIN=gen-go run states 0 'states made all n=([0-9]+)$' 'states released n=[0-9]+ subscriptions=[0-9]+' 'gen done mode=states clicks=2'
