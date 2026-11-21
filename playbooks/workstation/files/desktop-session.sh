#!/usr/bin/env bash
# A persistent virtual X11 display; browser disconnects do not end the session.
set -Eeuo pipefail
unset SESSION_MANAGER DBUS_SESSION_BUS_ADDRESS
export DISPLAY=:1 XDG_SESSION_TYPE=x11 XDG_CURRENT_DESKTOP=XFCE
Xtigervnc :1 -localhost yes -SecurityTypes None -rfbport 5901 \
  -geometry 1920x1080 -depth 24 -AlwaysShared -nolisten tcp \
  -desktop 'Painkiller workstation' &
vnc=$!
trap 'kill "$vnc" 2>/dev/null || true' EXIT
ready=false
for _ in {1..100}; do
  kill -0 "$vnc"
  if xdpyinfo >/dev/null 2>&1; then ready=true; break; fi
  sleep 0.1
done
$ready || { echo 'Virtual display did not become ready' >&2; exit 1; }
tigervncconfig -nowin &
dbus-run-session -- startxfce4 &
desktop=$!
wait -n "$vnc" "$desktop"
