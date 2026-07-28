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
# SIZING IS NOT FREE CHOICE. Play rejects anything that is not exactly 16:9 or
# 9:16, and imposes a per-slot pixel floor:
#
#   phone      16:9 or 9:16, each side  320..3840
#   7-inch     16:9 or 9:16, each side  320..3840
#   10-inch    16:9 or 9:16, each side 1080..7680
#
# Hence every size below is an exact 9:16 / 16:9 pair rather than a real
# device's dimensions — a Pixel is 412x915, which is neither.
#
# On scaling: Gio derives its UI scale solely from the Xft.dpi X resource
# (app/os_x11.go -> scale = Xft.dpi/96). Setting RESOURCE_MANAGER on this Xvfb
# leaves the render unchanged, so 1dp == 1px here and the window size IS the
# logical size. That is why the capture sizes are small: asking for a 1080-wide
# window does not enlarge the UI, it strands a phone-width form in a field of
# whitespace. The 10-inch slot's 1080px floor is therefore the one place we
# must upscale after the fact.
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

# Xvfb signals readiness with SIGUSR1 to its parent, which otherwise kills this
# script (exit 144).
trap '' USR1

OUT="${OUT:-assets/play/screenshots}"
DISPLAY_NUM="${DISPLAY_NUM:-:99}"
BIN="${BIN:-/tmp/vayumail-shots}"

# Capture sizes: exact 9:16 / 16:9, sized so the app's centred form keeps the
# proportions it has on a real device of that class.
PHONE_W=450    PHONE_H=800      # 9:16 x50
TABLET7_W=720  TABLET7_H=1280   # 9:16 x80
TABLET10_W=1280 TABLET10_H=720  # 16:9 x80

# Ship sizes. Every slot goes out at >=1080px per side: the 10-inch slot demands
# it outright, and Play only considers a listing for promotion when every
# screenshot clears 1080px. The scale factors keep the ratios exact.
PHONE_OUT="1080x1920"     # 2.4x
TABLET7_OUT="1080x1920"   # 1.5x
TABLET10_OUT="1920x1080"  # 1.5x

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
  env HOME="$home" DISPLAY="$DISPLAY_NUM" LIBGL_ALWAYS_SOFTWARE=1 \
    "$BIN" >/tmp/vayumail-shots.log 2>&1 &
  APP_PID=$!
  WID=""
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

# The sign-in form is vertically centred, so its links sit at fixed offsets from
# the window centre rather than at a fixed fraction of its height. Measured off a
# reference render: "Use a setup code" +207px below centre, "Set up manually"
# +240px, both 103px left of it.
click_at() {  # $1=width $2=height $3=offset-below-centre
  xdotool mousemove --sync $(( $1 / 2 - 103 )) $(( $2 / 2 + $3 ))
  sleep 1
  xdotool click 1
  sleep 4
  # The click lands, but Gio only draws when an event arrives: without this the
  # window still holds the PREVIOUS screen's frame and the capture silently
  # records the wrong page. Move the pointer away to force the redraw.
  xdotool mousemove $(( $1 / 2 )) $(( $2 / 4 ))
  sleep 4
}

# A screenshot that silently matches the previous one means the click missed and
# we are about to ship the same screen twice.
assert_differs() {  # $1=first $2=second
  if [ "$(md5sum < "$OUT/$1")" = "$(md5sum < "$OUT/$2")" ]; then
    echo "ERROR: $2 is identical to $1 — the navigation click missed." >&2
    exit 1
  fi
}

# Play checks the ratio to the pixel; catching it here beats a console rejection.
assert_spec() {  # $1=file $2=WxH
  local got; got="$(identify -format '%wx%h' "$OUT/$1")"
  [ "$got" = "$2" ] || { echo "ERROR: $1 is $got, expected $2" >&2; exit 1; }
}

echo "==> starting the virtual display"
Xvfb "$DISPLAY_NUM" -screen 0 2600x2600x24 -ac >/tmp/xvfb-shots.log 2>&1 &
XVFB_PID=$!
sleep 4
export DISPLAY="$DISPLAY_NUM"

capture_set() {  # $1=prefix $2=width $3=height $4=shipsize
  echo "==> $1 (capture ${2}x${3} -> ship $4)"

  start_app "$2" "$3"
  shot "$1-1-signin.png"
  if [ -n "${VAYUMAIL_DEMO_EMAIL:-}" ] && [ -n "${VAYUMAIL_DEMO_PASSWORD:-}" ]; then
    # A real mailbox gives a far better listing than onboarding screens alone.
    xdotool mousemove $(( $2 / 2 )) $(( $3 / 2 - 40 )) click 1
    xdotool type --delay 40 "$VAYUMAIL_DEMO_EMAIL"
    xdotool mousemove $(( $2 / 2 )) $(( $3 / 2 + 38 )) click 1
    xdotool type --delay 40 "$VAYUMAIL_DEMO_PASSWORD"
    xdotool mousemove $(( $2 / 2 )) $(( $3 / 2 + 106 )) click 1   # Connect
    sleep 25                                                     # first IMAP sync
    shot "$1-2-inbox.png"
    assert_differs "$1-1-signin.png" "$1-2-inbox.png"
  else
    click_at "$2" "$3" 207          # Use a setup code
    shot "$1-2-setupcode.png"
    assert_differs "$1-1-signin.png" "$1-2-setupcode.png"
  fi
  kill "$APP_PID" 2>/dev/null || true; sleep 2

  # Third screen. There is no back affordance on the setup-code screen, so this
  # starts from a fresh window rather than trying to navigate back.
  start_app "$2" "$3"
  click_at "$2" "$3" 240            # Set up manually
  shot "$1-3-manual.png"
  assert_differs "$1-1-signin.png" "$1-3-manual.png"
  if [ -f "$OUT/$1-2-setupcode.png" ]; then
    assert_differs "$1-2-setupcode.png" "$1-3-manual.png"
  fi
  kill "$APP_PID" 2>/dev/null || true; sleep 2

  echo "    scaling $1 to $4"
  for f in "$OUT/$1"-*.png; do
    convert "$f" -filter Lanczos -resize "$4!" "$f"
    assert_spec "$(basename "$f")" "$4"
  done
}

capture_set phone    "$PHONE_W"    "$PHONE_H"    "$PHONE_OUT"
capture_set tablet7  "$TABLET7_W"  "$TABLET7_H"  "$TABLET7_OUT"
capture_set tablet10 "$TABLET10_W" "$TABLET10_H" "$TABLET10_OUT"

echo "==> final check"
for f in "$OUT"/*.png; do
  identify -format '    %f  %wx%h\n' "$f"
done

echo
echo "Done. Review every image before uploading — a screenshot showing a real"
echo "address is both a privacy problem and a Play rejection risk."
