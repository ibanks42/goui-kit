# Regression tests for edge-case lifetimes and reentrancy: timers outliving
# views, closing own window, freed dock panels, reentrant actions, and retained handles.
build review3-go review3
# Proves a repeating timer attached to a child window stops firing against that view after
# the child window is closed (wrong=0, pings stays 1) while the main window remains alive.
GK_PROTO_BIN=review3-go run timer-after-view 0 \
  'review3 child opened windows=2' 'review3 child before release ticks=[1-9][0-9]* pings=1' \
  'review3 timer-after-view ticks=[1-9][0-9]* pings=1 wrong=0 windows=1' \
  'review3 done mode=timer-after-view .*final=1'
# Proves a view callback can safely close its own window without deadlocking or corrupting state,
# firing OnClose once and returning the window count to 1.
GK_PROTO_BIN=review3-go run close-own-window 0 \
  'review3 second opened windows=2' 'review3 close-self n=1' 'review3 second closed n=1 same=true' \
  'review3 close-own-window closes=1 closed=1 pings=[01] windows=1' 'review3 done mode=close-own-window'
# Proves releasing a dock panel's backing view prevents any further describes of that panel
# (described_after=0) without crashing the dock.
GK_PROTO_BIN=review3-go run dock-panel-freed 0 \
  'review3 panel view released drawn=[1-9]' 'review3 dock-panel-freed described_after=0 ' \
  'review3 done mode=dock-panel-freed'
# Proves dispatching a second action from inside an action listener queues rather than nesting
# re-entrantly (nested=0) and still delivers every queued action (n1=2, n2=10).
GK_PROTO_BIN=review3-go run reentrant-action 0 \
  'review3 reentrant window n1=2 n2=4 nested=0' 'review3 reentrant-action n1=2 n2=10 nested=0' \
  'review3 done mode=reentrant-action'
# Proves handles explicitly retained outlive the temporary callback/owner scope that handed them out
# and remain readable afterward (panels=1).
GK_PROTO_BIN=review3-go run handle-after-owner 0 \
  'review3 owner kept tree=true ctx=true panels=1' 'review3 handle-after-owner panels=1 ctx_open=(true|false)' \
  'review3 done mode=handle-after-owner'
