# run-platform.sh: the protocol tests (docs/PROTOCOLS.md 11.30), sourced
# by core/tests/run.sh, which defines build and run. The app is core/tests/platform: requests that
# answer later (a completion and its DONE turn), file dialogs and a prompt answered by the test
# (offscreen the platform is scripted), the menu bar read back, open_url recorded.
build platform-odin "$here/platform"

# A file dialog answered later, a save dialog answered before it was asked (taken at once), a
# prompt (the second button), a directory dialog dismissed: four DONE turns, in the order answered,
# each to its handler with its payload; the view that asked is described again after them;
# releasing an answered completion does nothing.
GK_PROTO_BIN=platform-odin run dialogs 0 \
  'platform early answer taken=false' 'platform saved n=1 status=Answered paths=/tmp/new.txt' \
  'platform answers taken=true,true,true' \
  'platform opened n=2 status=Answered paths=/tmp/a.txt,/tmp/b.txt error=""' \
  'platform prompt n=3 status=Answered answer=1' 'platform dismissed n=4 status=Cancelled paths=0' \
  'platform view described=[0-9]+ done=4' \
  'platform log requests=4 delivered=4 discarded=0 waiting=0 scripted=true done=4' \
  'platform done mode=dialogs done=4 '
# Released before its answer: cancelled, the answer finds nothing waiting and no turn comes.
GK_PROTO_BIN=platform-odin run cancel 0 'platform late answer taken=false' \
  'platform log requests=1 delivered=0 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=cancel done=0 '
# Released and answered in the same turn: the answer is delivered before the release lands; the
# handler, dropped with the release, does not run, and the release still succeeds.
GK_PROTO_BIN=platform-odin run race 0 'platform race answer taken=true' \
  'platform log requests=1 delivered=1 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=race done=0 '
# A request made in a child view's init goes with the view: releasing the view cancels it.
GK_PROTO_BIN=platform-odin run view-released 0 'platform child asked' 'platform child released' \
  'platform late answer taken=false' \
  'platform log requests=1 delivered=0 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=view-released done=0 '
# The menu bar set from app actions and read back (submenu, separator, the OS's Services menu, an
# item's OS action, checked and disabled flags); open_url recorded offscreen.
GK_PROTO_BIN=platform-odin run menus 0 'platform menus n=2' 'platform menu "File" disabled=false items=4' \
  'platform item action "Open" same=true os=None checked=false disabled=false' 'platform item separator' \
  'platform item submenu "Recent" items=1' 'platform item services "Services"' \
  'platform menu "Edit" disabled=true items=1' 'platform item action "Save" same=true os=Copy checked=true disabled=true' \
  'platform urls n=1 first="https://example.com/gk"'
# Misuse, each stopped at the app's line (exit 2): a completion released twice, a request from a
# view procedure, a handler whose payload is not the request's, a menu item with no action.
GK_PROTO_BIN=platform-odin run release-twice 2 'this completion was released already' 'at .*platform/main.odin:[0-9]+'
GK_PROTO_BIN=platform-odin run request-in-view 2 'a view procedure may not start a request' 'at .*platform/main.odin:[0-9]+'
GK_PROTO_BIN=platform-odin run wrong-payload 2 'this handler takes a .*Window, but the event of the builder it is given to is a .*Paths_Done' 'platform/main.odin:[0-9]+'
GK_PROTO_BIN=platform-odin run menu-no-action 2 'the item "Open" has no action' 'at .*platform/main.odin:[0-9]+'

# Round 6 (docs/PROTOCOLS.md 13.31): gpui's element leftovers.
# The decoders generated code uses, run by the library on what the package wrote (can_drop's ids and
# files, a drag's files for the platform, a prepaint order, an image as pixels and through
# ImageSource::Custom's loader, its first pixel in gpui's BGRA); the image cache, scroll anchor and
# text selection scope made, the elements that take them drawn (a cache on a div and an img of
# pixels, an anchored scrolled child with synthetic accessibility nodes, a prepaint order, a scope,
# a drag's files), scroll_anchor_to run, then all three released.
GK_PROTO_BIN=platform-odin run elements 0 \
  'platform drop accept 7=true 8=false any=true none=false files=false,true' \
  'platform drag payload n=2 first=/tmp/a.txt dir=false,true' 'platform drag payload none n=0' \
  'platform prepaint order=\[2, 0, 1\]' \
  'platform pixel image 2x1 first=ffff0000 custom 2x1 first=ffff0000' \
  'platform file drop entered at=3,4 paths=/tmp/a.txt,/tmp/b dir' \
  'platform sequence steps=2 from=10 targets=40,80' \
  'platform objects cache=true anchor=true scope=true' 'platform elements released described=[0-9]+'
# A drag of the library's value 7 onto a target whose can_drop takes 7: its drop runs and its
# on_drag_move handler gets the value and the element's bounds; then onto one that takes only 8: no
# drop (drops stays 1).
GK_PROTO_BIN=platform-odin run drag 0 'platform drag move id=7 text=seven bounds=100x100 at x>0=true' \
  'platform drop on seven id=7 text=seven drops=1' 'platform drag done drops=1 moved=true'
# A vertical and a horizontal virtual list over a row source of 1000 rows of three heights: rows in
# view described after one frame of placeholders, not all of them.
GK_PROTO_BIN=platform-odin run vlist 0 'platform vlist rows described=true of 1000 some=true'
# Misuse: an image cache made in a view procedure; a text selection scope the library never made.
GK_PROTO_BIN=platform-odin run cache-in-view 2 'a view procedure may not make an image cache' 'at .*platform/main.odin:[0-9]+'
GK_PROTO_BIN=platform-odin run scope-zero 2 'gk.text_selection_scope: a scope made by gk.text_selection_scope_id' 'platform/main.odin:[0-9]+'

# Round 7 (docs/PROTOCOLS.md 11.32): the platform asked for real, not the script. bin/platform-real
# runs the same app on a real window inside gates/display.sh (a private Xvfb or compositor with its own
# D-Bus session bus with no service files, so no xdg-desktop-portal answers), GK_SCRIPTED_PLATFORM
# unset, a fake xdg-open first on PATH that writes its arguments to $GK_URL_LOG. One display run at a
# time.
cat > "$bin/platform-real" <<'WRAP'
#!/bin/bash
# platform-real (written by core/tests/run-platform.sh): bin/platform-odin on a real window.
here=$(cd "$(dirname "$0")" && pwd)
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}; mkdir -p "$base"
d=$(mktemp -d "$base/xdg.XXXX") || exit 3
cat > "$d/xdg-open" <<'OPENER'
#!/bin/sh
printf '%s\n' "$@" >> "$GK_URL_LOG"
OPENER
chmod +x "$d/xdg-open"
unset GK_SCRIPTED_PLATFORM
PATH=$d:$PATH GK_URL_LOG=$d/args "$GK_REPO/gates/display.sh" "$here/platform-odin"; rc=$?
rm -rf "$d"; exit $rc
WRAP
chmod +x "$bin/platform-real"
# File dialogs with no portal: both answer Failed with gpui's reason (ashpd finds no
# org.freedesktop.portal.Desktop on the bus), in time; nothing hangs, the app goes on.
export GK_REPO=$repo
GK_PROTO_BIN=platform-real run real-dialogs 0 'platform real asked' \
  'platform real answer n=1 status=Failed paths=0 error="[^"]+" in_time=true' \
  'platform real answer n=2 status=Failed paths=0 error="[^"]+" in_time=true' \
  'platform log requests=2 delivered=2 discarded=0 waiting=0 scripted=false done=2' 'platform done mode=real-dialogs done=2 '
# open_url reaches the system opener (xdg-open, the open crate's first, on gpui's background executor)
# with the URL as its one argument; nothing is recorded in the scripted log.
GK_PROTO_BIN=platform-real run real-url 0 'platform real url opener got https://example.com/gk-real\?q=1$' \
  'platform real url recorded=0 scripted=false' 'platform done mode=real-url '
# set_menus where the platform has no menu bar (Linux): the kit's AppMenuBar is laid out and drawn in
# a real window's frame with the menus set.
GK_PROTO_BIN=platform-real run real-menus 0 'platform real menubar drawn w>100=true h>10=true menus=2 first="File"' \
  'platform done mode=real-menus '
# An input's context menu (InputState::on_context_menu, set in init) under the offscreen driver, whose
# right click is a move, a press and a release, each in its own update as a real window delivers them
# (core/meter/src/headless.rs). Drawn by the base element (gk.base_input) the handler runs
# after the release with upstream's empty menu, the position and the input's capabilities; the press
# moved the cursor into the text first.
GK_PROTO_BIN=platform-odin run context-menu 0 'platform context-menu fired items=0 copy=0 at=40,16 editable=true disabled=false cursor=[1-8]$' \
  'platform context-menu check mode=context-menu fired=1 cursor=[1-8]$'
# Drawn by the kit's element (gk.input) it never runs: gpui-component's Input::render installs the kit's
# own handler on the state each time it draws (the kit's Cut/Copy/Paste/Select All menu opens instead;
# PROTOCOLS.md 11.32). The same in Rust. The press still reached the state.
GK_PROTO_BIN=platform-odin run context-menu-kit 0 'platform context-menu check mode=context-menu-kit fired=0 cursor=[1-8]$'
