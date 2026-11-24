# App-lifecycle protocol tests: actions, keybindings, timers, multiple windows,
# clipboard, ambient reads, close vetoes, quit hooks, reduce-motion, and window icons.
build app-go app

# Proves custom and built-in action registration, keybinding lookup, capture-before-bubble
# dispatch order, and listener release; expects save twice, other twice before unbind, and cancel once.
GK_PROTO_BIN=app-go run actions 0 \
  'app actions save_builtin=false cancel_builtin=true same_name_same_id=true distinct=true' \
  'app capture save' \
  'app saved n=1 action_ok=true keys="[^"]+" has_keys=true available=true' \
  'app other n=1 action_ok=true' 'app other n=2 action_ok=true' 'app other released' \
  'app saved n=2 ' 'app cancel n=1 builtin=true same=true' \
  'app done mode=actions saves=2 others=2 cancels=1 '
# Proves OnAppAction rejects kit-owned built-in actions and exits 1 pointing at the Go call site.
GK_PROTO_BIN=app-go run builtin-app-listener 1 'OnAppAction takes the app.s own actions' 'at .*app/main.go:[0-9]+'
# Proves element OnAction rejects kit-owned built-in actions and exits 1 pointing at the Go call site.
GK_PROTO_BIN=app-go run builtin-on-action 1 'OnAction listens for the app.s own actions' 'at .*app/main.go:[0-9]+'
# Proves malformed keybinding context expressions fail with a parse diagnostic and exit 1.
GK_PROTO_BIN=app-go run bad-keys 1 'key binding "ctrl-b": context "Editor &&": ' 'at .*app/main.go:[0-9]+'

# Proves one-shot and repeating timers fire on schedule while a canceled timer never fires;
# expects once=1, ticks=3 before ticker release, and never=0.
GK_PROTO_BIN=app-go run timers 0 \
  'app timer once n=1' 'app timer tick n=1' 'app timer tick n=3' 'app timer ticker released' \
  'app timers view ticks=3 once=1' \
  'app done mode=timers .*once=1 ticks=3 never=0 '

# Proves multi-window open, title/bounds query, live rename, and OnClose notification;
# expects the window count to rise to 2 and return to 1 once the second window closes.
GK_PROTO_BIN=app-go run windows 0 \
  'app windows opened n=2' 'gk-headless: windows 2' 'app second view described' \
  'app second info title="Second" w=[0-9]+ h=[0-9]+ windows=2' \
  'app window closed n=1 same=true' 'gk-headless: windows 1' 'app windows after close n=1' \
  'app first title="Renamed"' 'app done mode=windows .*closed=1$'

# Proves WriteClipboard and ReadClipboard round-trip plain text identically.
GK_PROTO_BIN=app-go run clipboard 0 'app clipboard read ok=true text="hello gk"'
# Proves every generated ambient getter runs inside a view and returns expected values;
# expects called to equal want and active dialog state to report false.
GK_PROTO_BIN=app-go run ambient 0 'app ambient view blue=' 'ambient base_step_value' 'ambient close_dialog' \
  'app ambient called=([0-9]+) want=\1 dialog=false'

# Proves OnShouldClose can veto a close (false), allow it (true), and chain multiple handlers;
# expects first=false, subsequent attempts true, 3 handler invocations total, and 1 window left.
GK_PROTO_BIN=app-go run close-veto 0 'app asked may close n=1 answer=false' 'app asked may close n=2 answer=true' \
  'app asked by a second handler n=3' \
  'app should close first=false second=true third=true fourth=true asked=3 windows=1'
# Proves OnQuit runs once during graceful shutdown before Run returns.
GK_PROTO_BIN=app-go run quit-hook 0 'app quit hook ran n=1' 'app after the run quits=1'
# Proves SetReduceMotion toggles the preference and clears back to false while views can read it.
GK_PROTO_BIN=app-go run reduce-motion 0 'app reduce motion in view=' 'app reduce motion start=false set=true cleared=false'
# Proves a valid PNG icon can be attached when opening a second window without error.
GK_PROTO_BIN=app-go run window-icon 0 'app icon window opened n=2' 'app second view described'
# Proves invalid PNG icon bytes fail cleanly at open time and exit 1 with the byte count and Go call site.
GK_PROTO_BIN=app-go run bad-icon 1 'the window.s icon is not a PNG the library can read \(4 bytes\)' 'at .*app/main.go:[0-9]+'
