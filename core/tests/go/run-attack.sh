# C ABI robustness tests sending truncated buffers, unknown/empty/null ops,
# forged/wrong-kind/double-released handles, off-thread calls, and bad replies.
CGO_ENABLED=1 CGO_LDFLAGS="-L$lib" build attack-go attack
# Proves a truncated call buffer returns ERR_POISONED (6) with an out-of-range message
# and keeps the runtime poisoned on subsequent calls, exiting 1.
GK_PROTO_BIN=attack-go run truncated 1 'attack truncated rc=6 msg=".*out of range' 'attack truncated after rc=6'
# Proves unknown opcodes, empty buffers, and null pointers return ERR_BAD_OP (1)
# without poisoning the runtime, so a valid call afterward still returns rc=0.
GK_PROTO_BIN=attack-go run unknown-op 0 'attack unknown-op rc=1 ' 'attack empty rc=1 ' 'attack null rc=1 ' \
  'attack unknown-op after rc=0 view=0x' 'attack done mode=unknown-op rc=0'
# Proves forged handles, wrong-kind handles, and double releases return ERR_BAD_HANDLE (2)
# and off-thread gk_call returns ERR_WRONG_THREAD (5) without poisoning the session.
GK_PROTO_BIN=attack-go run forged 0 'attack forged source rc=2 msg=".*names no row source this library made' 'attack wrong-kind rc=2 msg=".*is a view, not a row source' \
  'attack release-twice first=0 second=2 ' 'attack thread rc=5' 'attack done mode=forged rc=0'
# Proves calling gk_reply with a stale gk_cb pointer after its callback returns yields
# ERR_NO_CONTEXT (4) cleanly and leaves the session usable.
GK_PROTO_BIN=attack-go run stale-cb 0 'attack stale-cb rc=4 msg=".*not the running callback' 'attack done mode=stale-cb rc=0'
# Proves sending a truncated reply buffer from the host callback poisons the runtime
# so the following call fails with ERR_POISONED (6) and exits 1.
GK_PROTO_BIN=attack-go run bad-reply 1 'attack bad-reply sent rc=0' 'attack init view=0x[0-9a-f]+ window rc=6 msg=".*out of range'
