# Overlay, dock, toast, popover, navigation, and real-X11 window title tests.
build overlay-go overlay

# Proves modal Dialog open, reactive re-describe on state updates, Cancel action handling,
# and idempotent close transitions active from true back to false.
GK_PROTO_BIN=overlay-go run dialog 0 \
  'overlay dialog opened' 'overlay dialog described n=1 count=0' 'overlay dialog active=true' \
  'overlay dialog described n=2 count=1' 'overlay dialog cancel n=1 seen=1 count=1' \
  'overlay dialog closed active=false; again same=true' \
  'overlay done mode=dialog active=false described=[3-9] cancels=1 count=1'
# Proves AlertDialog confirms via its OK button, runs the callback once, and closes (active=false).
GK_PROTO_BIN=overlay-go run alert 0 'overlay alert opened' 'overlay alert ok n=1' 'overlay done mode=alert active=false oks=1'
# Proves Sheet opens, describes its body while active=true, and returns to active=false on close.
GK_PROTO_BIN=overlay-go run sheet 0 'overlay sheet described' 'overlay sheet active=true' 'overlay done mode=sheet active=false'
# Proves Questionnaire tracks item order (color -> size), rejects unknown choices/items with structured errors,
# and advances only on valid answers.
GK_PROTO_BIN=overlay-go run questionnaire 0 \
  'overlay q made total=2 current=color' 'overlay q choose unknown failed=true err=UnknownChoice' \
  'overlay q choose blue failed=false' 'overlay q next moved=true current=size' \
  'overlay q set_current_item unknown failed=true' 'overlay done mode=questionnaire complete=false'
# Proves releasing an open dialog's backing view causes the next render to fail cleanly with exit 1.
GK_PROTO_BIN=overlay-go run release 1 'overlay released the open dialog.s view' 'through a handle that cannot be used'
# Proves a Dialog builder returning a non-Dialog root node is caught and exits 1 with the mismatched node type.
GK_PROTO_BIN=overlay-go run wrong-root 1 'returned a node of type [0-9]+, not a Dialog'
# Proves constructing a Questionnaire with duplicate item names fails with DuplicateItem and exits 1.
GK_PROTO_BIN=overlay-go run questionnaire-dup 1 'a constructor refused its arguments: DuplicateItem'

# Proves clicking item alpha in a popup menu fires alpha once and triggers a view redraw.
GK_PROTO_BIN=overlay-go run menu-redraw 0 'overlay menu fired alpha=1 bravo=0 redrawn=true'
# Proves clicking item bravo in the same popup menu fires bravo once and triggers a view redraw.
GK_PROTO_BIN=overlay-go run menu-redraw-b 0 'overlay menu fired alpha=0 bravo=1 redrawn=true'
# Proves Dock panels emit Added, Active, and Removed lifecycle events and describe both panels while not zoomed.
GK_PROTO_BIN=overlay-go run dock 0 'overlay dock made' 'overlay panel 0 event Added true' \
  'overlay panel [01] event Active true' 'overlay dock described first=[1-9][0-9]* second=[0-9]+ empty=false' \
  'overlay dock zoomed=false' 'overlay panel 1 event Removed false' 'overlay done mode=dock'
# Probe the real Xvfb window tree before and after SetTitle to verify OS title propagation.
cat > "$bin/overlay-display-go" <<SH
#!/bin/bash
GK_DISPLAY=xvfb exec "$repo/gates/display.sh" bash -c '"\$0" & p=\$!; sleep 0.6; xwininfo -root -tree | grep -E "Go on the kit|Renamed" | sed "s/^/overlay x11 before /"; sleep 1.0; xwininfo -root -tree | grep -E "Go on the kit|Renamed" | sed "s/^/overlay x11 after /"; wait \$p' "$bin/overlay-go"
SH
chmod +x "$bin/overlay-display-go"
# Proves SetTitle updates the actual X11 window title visible to xwininfo while preserving the 420x300 geometry.
GK_PROTO_BIN=overlay-display-go run window-real 0 'overlay window title="[^"]*" w=420 h=300' \
  'overlay x11 before .*"Go on the kit".* 420x300' 'overlay x11 after .*"Renamed by the app".* 420x300'

# Proves Dock pane-tree node IDs survive splits and walks, identify tab groups and panels consistently,
# reflect zoom state, and expose tab-bar skin configuration and split sizes.
GK_PROTO_BIN=overlay-go run dock-ids 0 'overlay ids group=true' \
  'overlay ids panels=2 same=true,true active=[01] ctx_node_same=true ctx_panels=2 closable=true' \
  'overlay ids dock panel found=true same=true' 'overlay ids split tree=true nodes=3 first_kept=true second_moved=true' \
  'overlay ids kind split=true children=2 walk=3' 'overlay ids first child tabs=true panels=2' \
  'overlay ids zoomed=true group=true' 'overlay ids skin style=Tab_Bar toggle=false' 'overlay done mode=dock-ids children=2 sizes=\[[0-9.]+, [0-9.]+\]'
# Proves passing a fabricated dock node ID (987654321) fails with a clear error and exits 1.
GK_PROTO_BIN=overlay-go run dock-bad-node 1 'node id 987654321 was never handed out'
# Proves dock layout JSON save/load reconstructs registered panels by type and payload, round-trips
# to_state/from_state, and returns false for unknown panel IDs.
GK_PROTO_BIN=overlay-go run dock-layout 0 'overlay layout saved .*first:panel.\{"file":"a.demo"\}' \
  'overlay layout loaded failed=false' 'overlay layout build first n=1 dock=true' \
  'overlay layout build second n=2 dock=true' 'overlay layout again same=true described=[1-9][0-9]*,[1-9][0-9]*' \
  'overlay layout tree to_state=.*first.*second.* from_state_panels=2' 'overlay layout build_panel known=true unknown=false' \
  'overlay done mode=dock-layout builds=5'
# Proves NavigationStack tracks stack depth (2), pop-back behavior, and forward history count (1).
GK_PROTO_BIN=overlay-go run nav 0 'overlay nav depth=2 current=true views=2,true' \
  'overlay nav popped=true now=true forward=1' 'overlay done mode=nav sizes=0 container=0'

# Proves custom DockAreaRenderer and TabGroupRenderer hooks are invoked for Dock, Tab_Bar, and Frame slots,
# describe custom chrome/tabbar/frame/bodies, and propagate panel-open toggle state.
GK_PROTO_BIN=overlay-go run render 0 'overlay render made' 'overlay render asked Dock placement=Left open=true key=1 same=true' \
  'overlay render asked Tab_Bar panels=1' 'overlay render frame=1 dock=1 tab_bar=[1-9] answers=[3-9] drawn_some=true' \
  'overlay render described chrome=true tabbar=true frame=true bodies=true' \
  'overlay done mode=render open=false dock_asks=1 chrome_open=true toggles=0 sub=true'
# Proves returning a non-div root from a dock frame renderer fails cleanly with exit 1.
GK_PROTO_BIN=overlay-go run render-bad-frame 1 'a frame is a gk.([dD]iv)\(\)'
# Proves ToastStore enqueue, replace-by-id, presentation/ending/removal lifecycle, dismiss-all,
# and monotonic clock progression.
GK_PROTO_BIN=overlay-go run toast 0 'overlay toast len=2 replaced=200 get2=201' \
  'overlay toast presented=\[1, 2\] changed=true' 'overlay toast ending=\[1\]' 'overlay toast removed=1 value=100 left=1' \
  'overlay toast dismissed=true visible=1 status=Ending empty=false' 'overlay done mode=toast iter=2 dismissed_all=\[3\] clock_moves=true'
# Proves programmatic Popover open/close flips open from true back to false without spurious user-change events.
GK_PROTO_BIN=overlay-go run popover 0 'overlay popover opened by the app' 'overlay popover open=true' \
  'overlay done mode=popover open=false changes=0'

# Proves post-layout position and bounds observers fire on initial layout (x=4) and again when moved (x=120).
GK_PROTO_BIN=overlay-go run observe 0 'overlay observe first positions=1 x=4 y=50 placement=nil pops=[1-9][0-9]* pop_w=true' \
  'overlay observe moved positions=2 x=120' 'overlay done mode=observe positions=2 pops=[1-9][0-9]* frames_seen=true'
# Proves overlay layout math helpers clamp coordinates, resolve pointer anchors, validate 3 layers,
# and reject invalid configurations with an error.
GK_PROTO_BIN=overlay-go run values 0 'overlay values clamp=700,100 pointer=300' \
  'overlay done mode=values ok=true layers=3 bad_ok=false bad_err=true'
