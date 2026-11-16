# run-app.sh: the protocol tests (docs/PROTOCOLS.md section 11), sourced by
# core/tests/run.sh, which defines build and run. The app is core/tests/app: actions and key
# bindings dispatched to Odin handlers, timers between frames, several windows, the clipboard,
# driven by gk-meter scripts offscreen.
build app-odin "$here/app"

# Key bindings dispatch app actions to an element's on_action (on the focus path, in its key
# context) and to an app-wide listener; capture_action runs first; a handler dispatches an
# action; a released app-wide listener hears nothing; a built-in action (ui::Cancel, id with bit
# 31) reaches on_boxed_action; the same name gives the same id; keystrokes and availability.
GK_PROTO_BIN=app-odin run actions 0 \
  'app actions save_builtin=false cancel_builtin=true same_name_same_id=true distinct=true' \
  'app capture save' \
  'app saved n=1 action_ok=true keys="[^"]+" has_keys=true available=true' \
  'app other n=1 action_ok=true' 'app other n=2 action_ok=true' 'app other released' \
  'app saved n=2 ' 'app cancel n=1 builtin=true same=true' \
  'app done mode=actions saves=2 others=2 cancels=1 '
# Misuse: an app-wide listener for a built-in action, on_action given a built-in action, and a
# key binding whose context does not parse each stop with the message (exit 2); the binding's
# write is sent with the next call that returns a value, so it stops at that line.
GK_PROTO_BIN=app-odin run builtin-app-listener 2 'on_app_action takes the app.s own actions' 'at .*app/main.odin:[0-9]+'
GK_PROTO_BIN=app-odin run builtin-on-action 2 'on_action listens for the app.s own actions' 'at .*app/main.odin:[0-9]+'
GK_PROTO_BIN=app-odin run bad-keys 2 'key binding "ctrl-b": context "Editor &&": ' 'at .*app/main.odin:[0-9]+'
# Timers fire between frames as handlers that redraw their view: a one-shot once, a repeating
# one until its handler releases it at 3, one released before it fired never.
GK_PROTO_BIN=app-odin run timers 0 \
  'app timer once n=1' 'app timer tick n=1' 'app timer tick n=3' 'app timer ticker released' \
  'app timers view ticks=3 once=1' \
  'app done mode=timers .*once=1 ticks=3 never=0 '
# A second window with options, opened from a timer and drawn; its info; closed by the app: the
# close listener runs, the handle list shrinks, the first window's title is set and read back.
# Offscreen every window has the meter's size, so the size is only read here, not compared. The
# title is the one the app gave(the library keeps it, as X11, Wayland
# and the headless platform give none back).
GK_PROTO_BIN=app-odin run windows 0 \
  'app windows opened n=2' 'gk-headless: windows 2' 'app second view described' \
  'app second info title="Second" w=[0-9]+ h=[0-9]+ windows=2' \
  'app window closed n=1 same=true' 'gk-headless: windows 1' 'app windows after close n=1' \
  'app first title="Renamed"' 'app done mode=windows .*closed=1$'
# The clipboard: written in one turn, read back in a later one.
GK_PROTO_BIN=app-odin run clipboard 0 'app clipboard read ok=true text="hello gk"'
# Every ambient call of the package once (core/tests/app/gen_ambient.odin, generated): free
# functions of the kit and the methods of WindowExt, from a timer with the window in scope; a
# palette call in a view procedure.
GK_PROTO_BIN=app-odin run ambient 0 'app ambient view blue=' 'ambient base_step_value' 'ambient close_dialog' \
  'app ambient called=([0-9]+) want=\1 dialog=false'
# The port round (PROTOCOLS.md 11.38). A window's close asked of the app: a handler that says no keeps
# it, yes lets it go, a released one and one without an answer hold nothing (asked as the platform asks,
# through the library's test hook: offscreen there is no close button). The quit hook runs once as the
# offscreen app shuts down, a released one never. Reduced motion read, set and cleared, also in a view.
# A window opened with an icon (a PNG) and an app id; an icon that is not a PNG stops with the message.
GK_PROTO_BIN=app-odin run close-veto 0 'app asked may close n=1 answer=false' 'app asked may close n=2 answer=true' \
  'app asked by a handler without an answer n=3' \
  'app should close first=false second=true third=true fourth=true asked=3 windows=1'
GK_PROTO_BIN=app-odin run quit-hook 0 'app quit hook ran n=1' 'app after the run quits=1'
GK_PROTO_BIN=app-odin run reduce-motion 0 'app reduce motion in view=' 'app reduce motion start=false set=true cleared=false'
GK_PROTO_BIN=app-odin run window-icon 0 'app icon window opened n=2' 'app second view described'
GK_PROTO_BIN=app-odin run bad-icon 2 'the window.s icon is not a PNG the library can read \(4 bytes\)' 'at .*app/main.odin:[0-9]+'
