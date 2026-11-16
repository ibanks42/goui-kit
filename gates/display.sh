#!/bin/bash
# display.sh [--hz N] CMD [ARGS...]: run CMD inside a private headless compositor and exit
# with its status. Nothing appears on the real screen and everything dies with this job:
#   - its own XDG_RUNTIME_DIR (its Wayland socket cannot collide with the real session),
#   - its own D-Bus session bus with no service files (so no portals or wallets start),
#   - a virtual output rendered on the GPU.
# --hz 60 (default) uses `kwin_wayland --virtual`; its
# virtual output is fixed at 60 Hz. Any other rate uses `gamescope --backend headless -r N`,
# which sends frame callbacks at N Hz (measured: 240 gives a 4.166 ms interval). CMD sees
# WAYLAND_DISPLAY only, never DISPLAY. Where kwin_wayland is not installed, 60 Hz uses
# gamescope too. GK_GAMESCOPE names the gamescope to run (default: the one on
# PATH); a wrapper that sets LD_LIBRARY_PATH for an unpacked gamescope saves the caller's in
# GK_LD_SAVED, and CMD gets it back.
# GK_DISPLAY=xvfb runs CMD on a private Xvfb instead (GK_XVFB names the Xvfb binary): for a
# machine where no compositor can reach the GPU (for example a user outside the render group,
# so gamescope has no DRM node, or a machine without a GPU). Xvfb paces frames at 60 Hz only, so any other
# --hz is refused there. CMD then sees DISPLAY, not WAYLAND_DISPLAY.
# Example:
#   gates/display.sh --hz 240 CMD...
# Scratch: a folder in $GK_SCRATCH (default $TMPDIR/gk-gates), removed on exit.
hz=60
if [ "$1" = --hz ]; then hz=$2; shift 2; fi
[ $# -gt 0 ] || { echo "usage: display.sh [--hz N] CMD..." >&2; exit 2; }
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}
mkdir -p "$base"
R=$(mktemp -d "$base/x.XXXX"); chmod 700 "$R"   # short: a unix socket path is at most 107 bytes
trap 'rm -rf "$R"; rmdir "$base" 2>/dev/null' EXIT
cat > "$R/bus.conf" <<CONF
<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <keep_umask/>
  <listen>unix:dir=$R</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
CONF
export XDG_RUNTIME_DIR=$R GK_HZ=$hz
export GK_WIDTH=${GK_WIDTH:-1600} GK_HEIGHT=${GK_HEIGHT:-1000}
unset WAYLAND_DISPLAY DISPLAY
if [ "${GK_DISPLAY:-}" = xvfb ]; then
  if [ "$hz" != 60 ]; then echo "display.sh: Xvfb paces frames at 60 Hz; --hz $hz cannot be run here"; exit 3; fi
  "${GK_XVFB:-Xvfb}" -displayfd 5 -screen 0 "${GK_WIDTH}x${GK_HEIGHT}x24" -nolisten tcp 5>"$R/displayno" >"$R/compositor.log" 2>&1 &
  XV=$!
  for i in $(seq 100); do [ -s "$R/displayno" ] && break; sleep 0.1; done
  if [ ! -s "$R/displayno" ]; then echo "display.sh: Xvfb did not start"; cat "$R/compositor.log"; kill $XV; exit 3; fi
  export DISPLAY=:$(head -1 "$R/displayno")
  dbus-run-session --config-file="$R/bus.conf" -- "$@"
  rc=$?
  kill $XV; wait $XV 2>/dev/null
  exit $rc
elif [ "$hz" = 60 ] && command -v kwin_wayland >/dev/null; then
  dbus-run-session --config-file="$R/bus.conf" -- bash -c '
    kwin_wayland --virtual --no-lockscreen --no-global-shortcuts --socket gk-wl \
      --width "$GK_WIDTH" --height "$GK_HEIGHT" >"$XDG_RUNTIME_DIR/compositor.log" 2>&1 &
    KW=$!
    for i in $(seq 100); do [ -S "$XDG_RUNTIME_DIR/gk-wl" ] && break; sleep 0.1; done
    if [ ! -S "$XDG_RUNTIME_DIR/gk-wl" ]; then
      echo "display.sh: kwin did not create its socket"; cat "$XDG_RUNTIME_DIR/compositor.log"; kill $KW; exit 3
    fi
    export WAYLAND_DISPLAY=gk-wl
    "$@"
    rc=$?
    kill $KW; wait $KW 2>/dev/null
    exit $rc
  ' _ "$@"
else
  # gamescope runs CMD as its child and exits with it. Its own exit status is not CMD's, so
  # CMD's is passed out through a file.
  dbus-run-session --config-file="$R/bus.conf" -- \
    "${GK_GAMESCOPE:-gamescope}" --backend headless --expose-wayland -r "$hz" \
      -W "$GK_WIDTH" -H "$GK_HEIGHT" -w "$GK_WIDTH" -h "$GK_HEIGHT" -- bash -c '
        if [ -z "$GAMESCOPE_WAYLAND_DISPLAY" ]; then echo "display.sh: gamescope gave no Wayland socket"; echo 3 > "$XDG_RUNTIME_DIR/rc"; exit 3; fi
        export WAYLAND_DISPLAY=$GAMESCOPE_WAYLAND_DISPLAY; unset DISPLAY
        if [ -n "${GK_LD_SAVED+x}" ]; then export LD_LIBRARY_PATH=$GK_LD_SAVED; unset GK_LD_SAVED; fi
        "$@"; echo $? > "$XDG_RUNTIME_DIR/rc"
      ' _ "$@" 2>"$R/compositor.log"
  rc=$(cat "$R/rc" 2>/dev/null || echo 3)
  [ "$rc" = 0 ] || tail -5 "$R/compositor.log" >&2
  exit "$rc"
fi
