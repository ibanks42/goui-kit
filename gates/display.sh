#!/bin/bash
# Isolated headless display wrapper (`gates/display.sh [--hz N] CMD...`).
#
# What it provides:
#   Starts a private D-Bus session and a headless Wayland or X11 compositor at the requested
#   refresh rate (`--hz N`, default 60) and viewport (`GK_WIDTH`x`GK_HEIGHT`, default 1600x1000).
#   Used by frame-paced gates (`fps.sh`, `memory.sh`, `counter.sh`, `profile.sh`, `readme-go.sh`)
#   so benchmarks run without disturbing the user's live desktop or competing with desktop windows.
#
# Why the isolation makes measurements fair:
#   Unsetting `WAYLAND_DISPLAY` and `DISPLAY`, isolating `XDG_RUNTIME_DIR` (mode 700), and running
#   a private D-Bus bus ensure every binary in a session sees the exact same virtual output, frame
#   pacing clock, and compositor without external notifications or desktop compositing interference.
#
# Backend selection:
#   1. `GK_DISPLAY=xvfb`: runs Xvfb at 60 Hz only (exits 3 for any other rate).
#   2. `hz=60` with `kwin_wayland` on PATH: runs virtual `kwin_wayland` on Wayland socket `gk-wl`.
#   3. Otherwise (any non-60 Hz rate or no `kwin_wayland`): runs headless `gamescope` at `-r <hz>`.
#
# Exit codes:
#   2 on missing command, 3 if the display server fails to start or cannot pace `--hz`, otherwise
#   the wrapped command's exact exit code.

hz=60
if [ "$1" = --hz ]; then hz=$2; shift 2; fi
[ $# -gt 0 ] || { echo "usage: display.sh [--hz N] CMD..." >&2; exit 2; }

# Keep the runtime directory path short so Unix domain sockets stay within sun_path limits.
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}
mkdir -p "$base"
R=$(mktemp -d "$base/x.XXXX"); chmod 700 "$R"
trap 'rm -rf "$R"; rmdir "$base" 2>/dev/null' EXIT

# Private D-Bus session config bound to $R so desktop portals and compositors stay isolated.
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
  # Poll up to 10 seconds for Xvfb to write its chosen display number to fd 5.
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
    # Wait up to 10 seconds for kwin_wayland to create its Wayland socket.
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
  # gamescope supports arbitrary headless refresh rates (-r). Save the child exit status to a file
  # because gamescope's own exit code does not reliably forward the wrapped command's status, and
  # restore GK_LD_SAVED if a setcap wrapper stripped LD_LIBRARY_PATH.
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
