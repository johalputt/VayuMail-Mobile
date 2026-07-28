#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# screenshots.sh — capture REAL screenshots of the running app for the Play
# listing.
#
# Play requires store screenshots to represent the app accurately, so this
# drives the actual binary under a virtual display rather than mocking a UI.
# The captures are genuine renders of the same code that ships.
#
# Sizes are the phone/tablet LOGICAL sizes, captured natively. Gio ignores the
# X server's DPI, so rendering at 1080x1920 leaves the layout floating in a sea
# of whitespace; capturing at 412x915 gives a correctly proportioned, crisp
# image, and Play accepts anything from 320px up.
#
# Screens past sign-in need a real account — set VAYUMAIL_DEMO_EMAIL and
# VAYUMAIL_DEMO_PASSWORD to a throwaway mailbox and the script fills the form.
# Use a mailbox you do not mind appearing in a public store listing.
#
# Deps (Debian/Ubuntu):
#   apt-get install -y xvfb imagemagick xdotool libxkbcommon-dev \
#     libxkbcommon-x11-dev libwayland-dev libx11-dev libx11-xcb-dev \
#     libxcursor-dev libxfixes-dev libgles2-mesa-dev libegl1-mesa-dev \
#     libvulkan-dev
set -euo pipefail

OUT="${OUT:-assets/play/screenshots}"
DISPLAY_NUM="${DISPLAY_NUM:-:99}"
BIN="${BIN:-/tmp/vayumail-shots}"
PHONE_W=412 PHONE_H=915
TABLET7_W=800 TABLET7_H=1280
TABLET10_W=1280 TABLET10_H=800

# Where "Use a setup code" lands, as a permille of window height. It moves with
# the aspect ratio because the form is vertically centred, so each size carries
# its own measured value rather than one shared guess.
PHONE_CLICK=727 TABLET7_CLICK=690 TABLET10_CLICK=760

mkdir -p "$OUT"
echo "==> building the real app"
go build -o "$BIN" ./cmd/vayumail

cleanup() {
  [ -n "${APP_PID:-}" ] && kill "$APP_PID" 2>/dev/null || true
  [ -n "${XVFB_PID:-}" ] && kill "$XVFB_PID" 2>/dev/null || true
}
trap cleanup EXIT

start_app() {  # $1=width $2=height
  local home; home="$(mktemp -d)"
  nohup env HOME="$home" DISPLAY="$DISPLAY_NUM" LIBGL_ALWAYS_SOFTWARE=1 \
    "$BIN" >/tmp/vayumail-shots.log 2>&1 &
  APP_PID=$!
  # Gio needs a moment to create its window and complete the first frame.
  for _ in $(seq 1 30); do
    WID="$(xdotool search --name '^VayuMail$' 2>/dev/null | head -1 || true)"
    [ -n "$WID" ] && break
    sleep 1
  done
  [ -n "$WID" ] || { echo "the app never opened a window; see /tmp/vayumail-shots.log" >&2; exit 1; }
  xdotool windowsize "$WID" "$1" "$2"
  xdotool windowmove "$WID" 0 0
  # Software GL reallocates the surface slowly, and Gio only repaints on an
  # event: capturing too early yields a half-black frame that looks like a
  # rendering bug rather than a timing one. Wait, then nudge the pointer to
  # force one full redraw.
  sleep 14
  xdotool mousemove $(( $1 / 2 )) $(( $2 / 2 )); sleep 3
}

shot() { import -window "$WID" "$OUT/$1"; echo "    $OUT/$1"; }

echo "==> starting the virtual display"
nohup Xvfb "$DISPLAY_NUM" -screen 0 1600x2560x24 -ac >/tmp/xvfb-shots.log 2>&1 &
XVFB_PID=$!
sleep 4
export DISPLAY="$DISPLAY_NUM"

echo "==> phone (${PHONE_W}x${PHONE_H})"
start_app "$PHONE_W" "$PHONE_H"
shot phone-1-signin.png

if [ -n "${VAYUMAIL_DEMO_EMAIL:-}" ] && [ -n "${VAYUMAIL_DEMO_PASSWORD:-}" ]; then
  echo "==> signing in to capture the screens behind the login"
  xdotool mousemove --window "$WID" 205 455 click 1
  xdotool type --window "$WID" --delay 40 "$VAYUMAIL_DEMO_EMAIL"
  xdotool mousemove --window "$WID" 205 533 click 1
  xdotool type --window "$WID" --delay 40 "$VAYUMAIL_DEMO_PASSWORD"
  xdotool mousemove --window "$WID" 205 601 click 1     # Connect
  sleep 25                                              # first IMAP sync
  shot phone-2-inbox.png
  # Open the first message in the list.
  xdotool mousemove --window "$WID" 205 220 click 1; sleep 6
  shot phone-3-message.png
else
  echo "    (set VAYUMAIL_DEMO_EMAIL / VAYUMAIL_DEMO_PASSWORD for inbox screens)"
fi
kill "$APP_PID" 2>/dev/null || true; sleep 2

echo "==> tablet (${TABLET_W}x${TABLET_H})"
start_app "$TABLET_W" "$TABLET_H"
shot tablet-1-signin.png

echo
echo "Done. Review every image before uploading — a screenshot showing a real"
echo "address is both a privacy problem and a Play rejection risk."
