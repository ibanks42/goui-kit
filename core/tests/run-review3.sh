# run-review3.sh: five attacks on the protocols built since milestone 2 (core/tests/review3),
# sourced by core/tests/run.sh, which defines build and run.
# Each must end correctly or stop with a message: never a crash, a hang, or app code run against a
# view that is gone. Run alone: core/tests/run.sh timer-after-view close-own-window ...
build review3-odin "$here/review3"
# A released view's timer and app-wide listener (kept in its init) never run again; the app's own
# release of their handles afterwards is harmless, and a handler kept after that still fires.
GK_PROTO_BIN=review3-odin run timer-after-view 0 \
  'review3 child opened windows=2' 'review3 child before release ticks=[1-9][0-9]* pings=1' \
  'review3 timer-after-view ticks=[1-9][0-9]* pings=1 wrong=0 windows=1' \
  'review3 done mode=timer-after-view .*final=1'
# A key binding's handler closes its own window (the turn's) and dispatches an action in that turn.
GK_PROTO_BIN=review3-odin run close-own-window 0 \
  'review3 second opened windows=2' 'review3 close-self n=1' 'review3 second closed n=1 same=true' \
  'review3 close-own-window closes=1 closed=1 pings=[01] windows=1' 'review3 done mode=close-own-window'
# A docked panel's view released while shown (it is never described again, a click on what is left
# reaches nobody), then the panel released and the dock laid out without it.
GK_PROTO_BIN=review3-odin run dock-panel-freed 0 \
  'review3 panel view released drawn=[1-9]' 'review3 dock-panel-freed described_after=0 ' \
  'review3 done mode=dock-panel-freed'
# An app-wide listener that dispatches its own action, releases itself and adds another while it
# runs: one dispatch at a time, with a window and without one.
GK_PROTO_BIN=review3-odin run reentrant-action 0 \
  'review3 reentrant window n1=2 n2=4 nested=0' 'review3 reentrant-action n1=2 n2=10 nested=0' \
  'review3 done mode=reentrant-action'
# A dock's layout and a dock context (retained from a render request) used after the dock was released.
GK_PROTO_BIN=review3-odin run handle-after-owner 0 \
  'review3 owner kept tree=true ctx=true panels=1' 'review3 handle-after-owner panels=1 ctx_open=(true|false)' \
  'review3 done mode=handle-after-owner'
