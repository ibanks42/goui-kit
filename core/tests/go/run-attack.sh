# run-attack.sh: attacks on the C interface by a raw client (core/tests/go/attack: its own callback,
# requests and replies through cgo, no Go package checks in between), sourced by
# core/tests/go/run.sh, which defines build and run. Odin twin: core/tests/run-attack.sh. Each must
# end in an error code or a stop with the message: never a crash, a hang, or memory written where
# it must not be.
build attack-go attack
# A request cut short in its arguments: the library stops (poisoned) with the message, refuses
# every later call, and the client exits 1.
GK_PROTO_BIN=attack-go run truncated 1 'attack truncated rc=6 msg=".*out of range' 'attack truncated after rc=6'
# An unknown op number, an empty request and a null one are refused (GK_ERR_BAD_OP) and the
# library goes on working.
GK_PROTO_BIN=attack-go run unknown-op 0 'attack unknown-op rc=1 ' 'attack empty rc=1 ' 'attack null rc=1 ' \
  'attack unknown-op after rc=0 view=0x' 'attack done mode=unknown-op rc=0'
# Handles: made up (stale), of another kind, released twice; a call from another thread.
GK_PROTO_BIN=attack-go run forged 0 'attack forged source rc=2 msg=".*names no row source this library made' 'attack wrong-kind rc=2 msg=".*is a view, not a row source' \
  'attack release-twice first=0 second=2 ' 'attack thread rc=5' 'attack done mode=forged rc=0'
# gk_reply with the gk_cb of a callback that has returned (it pointed into a stack frame that is
# gone; the library used to write the reply there): refused.
GK_PROTO_BIN=attack-go run stale-cb 0 'attack stale-cb rc=4 msg=".*not the running callback' 'attack done mode=stale-cb rc=0'
# A reply whose counts run past its bytes (here to the VIEWS turn that opening the window runs):
# the library stops with the message (poisoned; the call that ran the turn returns it), exit 1.
GK_PROTO_BIN=attack-go run bad-reply 1 'attack bad-reply sent rc=0' 'attack init view=0x[0-9a-f]+ window rc=6 msg=".*out of range'
