# Platform integration tests: scripted and real file/prompt dialogs, menus,
# URL opening, drag-and-drop, images/caches, virtual lists, and context menus.
build platform-go platform

# Proves scripted save/open/prompt/dismiss dialogs reject answers before a request is live
# (taken=false) and deliver all 4 answered/canceled replies cleanly with waiting=0 at exit.
GK_PROTO_BIN=platform-go run dialogs 0 \
  'platform early answer taken=false' 'platform saved n=1 status=Answered paths=/tmp/new.txt' \
  'platform answers taken=true,true,true' \
  'platform opened n=2 status=Answered paths=/tmp/a.txt,/tmp/b.txt error=""' \
  'platform prompt n=3 status=Answered answer=1' 'platform dismissed n=4 status=Cancelled paths=0' \
  'platform view described=[0-9]+ done=4' \
  'platform log requests=4 delivered=4 discarded=0 waiting=0 scripted=true done=4' \
  'platform done mode=dialogs done=4 '
# Proves canceling a pending dialog completion causes any later scripted answer to be ignored (taken=false, delivered=0).
GK_PROTO_BIN=platform-go run cancel 0 'platform late answer taken=false' \
  'platform log requests=1 delivered=0 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=cancel done=0 '
# Proves a completion canceled after the answer was already enqueued drops the callback (done=0)
# even though the scripted queue accepted the answer (taken=true, delivered=1).
GK_PROTO_BIN=platform-go run race 0 'platform race answer taken=true' \
  'platform log requests=1 delivered=1 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=race done=0 '
# Proves releasing the view that started a platform dialog automatically cancels the pending request
# so a subsequent answer is ignored (taken=false, done=0).
GK_PROTO_BIN=platform-go run view-released 0 'platform child asked' 'platform child released' \
  'platform late answer taken=false' \
  'platform log requests=1 delivered=0 discarded=0 waiting=0 scripted=true done=0' 'platform done mode=view-released done=0 '
# Proves SetMenus preserves menu hierarchy (actions, separators, submenus, OS roles, checked/disabled flags)
# and scripted OpenURL records the target URL.
GK_PROTO_BIN=platform-go run menus 0 'platform menus n=2' 'platform menu "File" disabled=false items=4' \
  'platform item action "Open" same=true os=None checked=false disabled=false' 'platform item separator' \
  'platform item submenu "Recent" items=1' 'platform item services "Services"' \
  'platform menu "Edit" disabled=true items=1' 'platform item action "Save" same=true os=Copy checked=true disabled=true' \
  'platform urls n=1 first="https://example.com/gk"'
# Proves releasing the same Completion handle twice fails cleanly and exits 1 at the Go call site.
GK_PROTO_BIN=platform-go run release-twice 1 'this completion was released already' 'at .*platform/main.go:[0-9]+'
# Proves starting an async platform dialog request inside a view function is rejected with exit 1.
GK_PROTO_BIN=platform-go run request-in-view 1 'a view (procedure|function) may not start a request' 'at .*platform/main.go:[0-9]+'
# Proves a menu action item without an attached action fails validation and exits 1 at the Go call site.
GK_PROTO_BIN=platform-go run menu-no-action 1 'the item "Open" has no action' 'at .*platform/main.go:[0-9]+'

# Proves drop-predicate filtering, external file-drop payloads, prepaint child ordering,
# RGBA->BGRA pixel/custom image creation, image-cache reuse (new images=0), and GPU texture drop on release.
GK_PROTO_BIN=platform-go run elements 0 \
  'platform drop accept 7=true 8=false any=true none=false files=false,true' \
  'platform drag payload n=2 first=/tmp/a.txt dir=false,true' 'platform drag payload none n=0' \
  'platform prepaint order=\[2, 0, 1\]' \
  'platform pixel image 2x1 first=ffff0000 custom 2x1 first=ffff0000 same=true' \
  'platform file drop entered at=3,4 paths=/tmp/a.txt,/tmp/b dir' \
  'platform sequence steps=2 from=10 targets=40,80' \
  'platform objects cache=true anchor=true scope=true' \
  'platform image kept redrawn=true same=true new images=0' 'platform elements released described=[0-9]+' \
  'platform image released dropped=1'
# Proves in-app drag-and-drop delivers move geometry (bounds 100x100, x>0) and completes a drop with id=7.
GK_PROTO_BIN=platform-go run drag 0 'platform drag move id=7 text=seven bounds=100x100 at x>0=true' \
  'platform drop on seven id=7 text=seven drops=1' 'platform drag done drops=1 moved=true'
# Proves virtualized list renders only a visible window of its 1000 items (some=true).
GK_PROTO_BIN=platform-go run vlist 0 'platform vlist rows described=true of 1000 some=true'
# Proves creating an image cache inside a view function is rejected with exit 1 at the Go call site.
GK_PROTO_BIN=platform-go run cache-in-view 1 'a view (procedure|function) may not make an image cache' 'at .*platform/modes.go:[0-9]+'
# Proves creating an image inside a view function is rejected with exit 1 at the Go call site.
GK_PROTO_BIN=platform-go run image-in-view 1 'a view (procedure|function) may not make an image' 'at .*platform/main.go:[0-9]+'
# Proves passing a zero-value motion scope fails validation and exits 1 pointing at the Go call site.
GK_PROTO_BIN=platform-go run scope-zero 1 'a scope made by (gk|motion).[A-Za-z_.]+' 'platform/modes.go:[0-9]+'

# Unscripted Xvfb wrapper injecting a stub xdg-open so real OS paths, URL opening, and menu bars are exercised.
cat > "$bin/platform-real-go" <<'WRAP'
#!/bin/bash
here=$(cd "$(dirname "$0")" && pwd)
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}; mkdir -p "$base"
d=$(mktemp -d "$base/xdg.XXXX") || exit 3
cat > "$d/xdg-open" <<'OPENER'
#!/bin/sh
printf '%s\n' "$@" >> "$GK_URL_LOG"
OPENER
chmod +x "$d/xdg-open"
unset GK_SCRIPTED_PLATFORM
PATH=$d:$PATH GK_URL_LOG=$d/args "$GK_REPO/gates/display.sh" "$here/platform-go"; rc=$?
rm -rf "$d"; exit $rc
WRAP
chmod +x "$bin/platform-real-go"
export GK_REPO=$repo
# Proves unscripted OS dialog calls on headless Xvfb (without a desktop portal) report status=Failed
# with an error string promptly instead of hanging (scripted=false, done=2).
GK_PROTO_BIN=platform-real-go run real-dialogs 0 'platform real asked' \
  'platform real answer n=1 status=Failed paths=0 error="[^"]+" in_time=true' \
  'platform real answer n=2 status=Failed paths=0 error="[^"]+" in_time=true' \
  'platform log requests=2 delivered=2 discarded=0 waiting=0 scripted=false done=2' 'platform done mode=real-dialogs done=2 '
# Proves unscripted OpenURL invokes the system xdg-open binary with the exact URL rather than the scripted log.
GK_PROTO_BIN=platform-real-go run real-url 0 'platform real url opener got https://example.com/gk-real\?q=1$' \
  'platform real url recorded=0 scripted=false' 'platform done mode=real-url '
# Proves an unscripted Xvfb window draws a real menu bar with positive dimensions and the "File" menu.
GK_PROTO_BIN=platform-real-go run real-menus 0 'platform real menubar drawn w>100=true h>10=true menus=2 first="File"' \
  'platform done mode=real-menus '

# Proves a custom input context-menu handler fires on right-click at (40,16) with editable/cursor state.
GK_PROTO_BIN=platform-go run context-menu 0 'platform context-menu fired items=0 copy=0 at=40,16 editable=true disabled=false cursor=[1-8]$' \
  'platform context-menu check mode=context-menu fired=1 cursor=[1-8]$'
# Proves leaving the kit default input context menu does not invoke the custom handler (fired=0).
GK_PROTO_BIN=platform-go run context-menu-kit 0 'platform context-menu check mode=context-menu-kit fired=0 cursor=[1-8]$'
