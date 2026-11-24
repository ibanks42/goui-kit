# run-overlay.sh: the protocol tests (docs/PROTOCOLS.md section 11.6), sourced by
# core/tests/go/run.sh, which defines build and run. The app is core/tests/go/overlay: dialogs, an
# alert and a sheet opened with view-built functions, a questionnaire, offscreen.
# Go has no String for the kit's enums: the app prints the names the checks expect
# (Added, Left, Tab_Bar, ...) itself.
build overlay-go overlay

# A dialog opened from a handler: its function is described before the kit first builds it;
# described again while open (the count bumped): escape runs OnCancel from the second
# description's tape (seen=1); opened again on the same view (Go: the Built the app kept);
# closed by the app.
GK_PROTO_BIN=overlay-go run dialog 0 \
  'overlay dialog opened' 'overlay dialog described n=1 count=0' 'overlay dialog active=true' \
  'overlay dialog described n=2 count=1' 'overlay dialog cancel n=1 seen=1 count=1' \
  'overlay dialog closed active=false; again same=true' \
  'overlay done mode=dialog active=false described=[3-9] cancels=1 count=1'
# An alert: enter confirms, OnOk runs with the view's state and closes it.
GK_PROTO_BIN=overlay-go run alert 0 'overlay alert opened' 'overlay alert ok n=1' 'overlay done mode=alert active=false oks=1'
# A sheet at the left: described, open while active, closed by the app (OnClose is its close
# button's click, not run by CloseSheet).
GK_PROTO_BIN=overlay-go run sheet 0 'overlay sheet described' 'overlay sheet active=true' 'overlay done mode=sheet active=false'
# A questionnaire: a state whose constructor can fail, updates that return the error's text
# (Result<(), E>: Go's error), a read of the current item, navigation.
GK_PROTO_BIN=overlay-go run questionnaire 0 \
  'overlay q made total=2 current=color' 'overlay q choose unknown failed=true err=UnknownChoice' \
  'overlay q choose blue failed=false' 'overlay q next moved=true current=size' \
  'overlay q set_current_item unknown failed=true' 'overlay done mode=questionnaire complete=false'
# Misuse: a dialog still open on a released view stops at its next build; a function that returns
# another node than the one asked for stops (Go: a gk.Built made around a plain view, since the
# compiler checks a view-built function's type); a questionnaire with two items of one name stops
# (exit 2 each).
GK_PROTO_BIN=overlay-go run release 2 'overlay released the open dialog.s view' 'through a handle that cannot be used'
GK_PROTO_BIN=overlay-go run wrong-root 2 'returned a node of type [0-9]+, not a Dialog'
GK_PROTO_BIN=overlay-go run questionnaire-dup 2 'a constructor refused its arguments: DuplicateItem'
# A dropdown opened, its view described again ten times while it is open (a label that changes on
# every tick), then one item clicked: an item's own click handler (Alpha) fires exactly once, as an
# item that runs an action (Bravo) does. The open menu holds the description that made it, so its
# handler table stays (PROTOCOLS.md section 4); before, a handler two descriptions old was dropped.
GK_PROTO_BIN=overlay-go run menu-redraw 0 'overlay menu fired alpha=1 bravo=0 redrawn=true'
GK_PROTO_BIN=overlay-go run menu-redraw-b 0 'overlay menu fired alpha=0 bravo=1 redrawn=true'
# Dock panels (11.7): two Go views as panels of the kit's styled dock, described when the dock
# draws them; the dock tells them they were added and made active (PanelEvent, queued); the app
# renames one and emits PanelZoomIn (delivered; the kit 0.9.1 zooms tab groups, never a panel, so
# the dock stays unzoomed), a removed panel hears it.
GK_PROTO_BIN=overlay-go run dock 0 'overlay dock made' 'overlay panel 0 event Added true' \
  'overlay panel [01] event Active true' 'overlay dock described first=[1-9][0-9]* second=[0-9]+ empty=false' \
  'overlay dock zoomed=false' 'overlay panel 1 event Removed false' 'overlay done mode=dock'
# A real window on a display (gates/display.sh, here its private Xvfb): the size the app gave
# comes back through gk.GetWindowInfo, and the X server holds the title the app gave and the one it
# set later (xwininfo while the app runs; GPUI reads titles back on macOS only). Go: the window's
# title is "Go on the kit".
cat > "$bin/overlay-display-go" <<SH
#!/bin/bash
exec "$repo/gates/display.sh" bash -c '"\$0" & p=\$!; sleep 0.6; xwininfo -root -tree | grep -E "Go on the kit|Renamed" | sed "s/^/overlay x11 before /"; sleep 1.0; xwininfo -root -tree | grep -E "Go on the kit|Renamed" | sed "s/^/overlay x11 after /"; wait \$p' "$bin/overlay-go"
SH
chmod +x "$bin/overlay-display-go"
GK_PROTO_BIN=overlay-display-go run window-real 0 'overlay window title="[^"]*" w=420 h=300' \
  'overlay x11 before .*"Go on the kit".* 420x300' 'overlay x11 after .*"Renamed by the app".* 420x300'

# Round 3 (PROTOCOLS.md 11.8). Node ids and the tab group the dock put a panel in: the group's
# reads (its node, its panels as the app's own panel handles, its context), a split at the group's
# node (upstream's split_at inserts the panel beside the node and leaves it in its group too: the
# first child keeps both), sizes set and a zoom by node id, read back through the pane tree, the
# pane nodes' kinds and walk, the skin's settings and the saved layout.
GK_PROTO_BIN=overlay-go run dock-ids 0 'overlay ids group=true' \
  'overlay ids panels=2 same=true,true active=[01] ctx_node_same=true ctx_panels=2 closable=true' \
  'overlay ids dock panel found=true same=true' 'overlay ids split tree=true nodes=3 first_kept=true second_moved=true' \
  'overlay ids kind split=true children=2 walk=3' 'overlay ids first child tabs=true panels=2' \
  'overlay ids zoomed=true group=true' 'overlay ids skin style=Tab_Bar toggle=false' 'overlay done mode=dock-ids children=2 sizes=\[[0-9.]+, [0-9.]+\]'
# A node id the dock never handed out stops the app when the write is applied (exit 2).
GK_PROTO_BIN=overlay-go run dock-bad-node 2 'node id 987654321 was never handed out'
# A saved layout: dumped (a panel's own JSON data in it), loaded back through the registry (a
# PanelBuild request per panel, queued, answered with gk.PanelFill), dumped again the same; a pane
# tree written out and read back (its two panels built through the registry again), and
# PanelRegistry::build_panel for a known and an unknown name: 5 PanelBuild requests in all.
GK_PROTO_BIN=overlay-go run dock-layout 0 'overlay layout saved .*first:panel.\{"file":"a.demo"\}' \
  'overlay layout loaded failed=false' 'overlay layout build first n=1 dock=true' \
  'overlay layout build second n=2 dock=true' 'overlay layout again same=true described=[1-9][0-9]*,[1-9][0-9]*' \
  'overlay layout tree to_state=.*first.*second.* from_state_panels=2' 'overlay layout build_panel known=true unknown=false' \
  'overlay done mode=dock-layout builds=5'
# The nav stack takes app views and gives them back (AnyView both ways); ResizableState made by
# its Default.
GK_PROTO_BIN=overlay-go run nav 0 'overlay nav depth=2 current=true views=2,true' \
  'overlay nav popped=true now=true forward=1' 'overlay done mode=nav sizes=0 container=0'

# Round 4 (PROTOCOLS.md 11.9 to 11.13). A dock drawn by the app's renderer: the frame, a dock's chrome
# and the tab bar are asked once per key and context (queued requests; the tab bar again when its
# group's context changed), answered with app views and drawn from them (the dock's content where the
# chrome's view put gk.DockSlot()); the chrome's own call (DockContext.Toggle, from the request)
# closes the dock, which upstream then does not draw (no new request); the kit reports no toggle of
# the app's (DockToggleEvent is the user's).
GK_PROTO_BIN=overlay-go run render 0 'overlay render made' 'overlay render asked Dock placement=Left open=true key=1 same=true' \
  'overlay render asked Tab_Bar panels=1' 'overlay render frame=1 dock=1 tab_bar=[1-9] answers=[3-9] drawn_some=true' \
  'overlay render described chrome=true tabbar=true frame=true bodies=true' \
  'overlay done mode=render open=false dock_asks=1 chrome_open=true toggles=0 sub=true'
# Misuse: a frame hook answered with a view whose root is not a div stops where the dock draws (exit 2).
# Go: the message may name gk.Div().
GK_PROTO_BIN=overlay-go run render-bad-frame 2 'a frame is a gk.([dD]iv)\(\)'
# The toast stack's bookkeeping (ToastManager<u64, u64>) on the library clock: pushed (one replaced),
# presented after the motion, a timeout ending one, removed after its exit, dismissed.
GK_PROTO_BIN=overlay-go run toast 0 'overlay toast len=2 replaced=200 get2=201' \
  'overlay toast presented=\[1, 2\] changed=true' 'overlay toast ending=\[1\]' 'overlay toast removed=1 value=100 left=1' \
  'overlay toast dismissed=true visible=1 status=Ending empty=false' 'overlay done mode=toast iter=2 dismissed_all=\[3\] clock_moves=true'
# A popover opened and closed by the app (Open(bool) from the app's state), not by a click.
GK_PROTO_BIN=overlay-go run popover 0 'overlay popover opened by the app' 'overlay popover open=true' \
  'overlay done mode=popover open=false changes=0'

# (PROTOCOLS.md 11.14, 11.15). Observations: a positioner at a corner and a
# popup report where they were laid out (Positioner::on_position, Popup::on_position, the popup's as it
# settles on its trigger's bounds of the frame before):
# after the frame, once for each new position (the app moves the positioner once: two reports; the corner
# at x 0 is clamped to the 4 px margin), never
# again for the same values while the views are described again.
GK_PROTO_BIN=overlay-go run observe 0 'overlay observe first positions=1 x=4 y=50 placement=nil pops=[1-9][0-9]* pop_w=true' \
  'overlay observe moved positions=2 x=120' 'overlay done mode=observe positions=2 pops=[1-9][0-9]* frames_seen=true'
# Value calls: computations over values the app built, as ambient calls: a dock's size clamped against
# the area and the opposite dock (1000 - 100 - 200 = 700; at least 100), the size a pointer implies, a
# sankey layout's layers, and a layout refused (a link to a node that is not there).
GK_PROTO_BIN=overlay-go run values 0 'overlay values clamp=700,100 pointer=300' \
  'overlay done mode=values ok=true layers=3 bad_ok=false bad_err=true'
