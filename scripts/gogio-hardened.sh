#!/bin/sh
# Build a gogio whose generated AndroidManifest disables backup.
#
# WHY THIS EXISTS
#
# gogio writes the AndroidManifest itself, from a template compiled into the
# tool, and offers no flag to change it. Its <application> tag carries only an
# icon and a label — no android:allowBackup — so Android's default of "true"
# applies to every APK and AAB this project has ever shipped. That makes the
# app-private directory (vayumail.db and the sealed keystore) eligible for
# `adb backup` and cloud Auto Backup: the no-root path that turns an at-rest
# weakness into an off-device one.
#
# platform/android/README.md has described this as pending release-pipeline
# work for some time. This is that work. The two rule files next to that README
# stay as documentation of what to exclude if backup is ever deliberately
# re-enabled; they are NOT referenced from the manifest, because @xml/ resource
# references require the files to be compiled into the resource table and gogio
# builds its res/ directory internally. allowBackup="false" needs no resource
# and is the whole control on its own.
#
# HOW IT FAILS
#
# Loudly. The template text is matched exactly and the match count asserted. If
# a future gogio rewrites that line, this script stops the build rather than
# quietly producing an APK with backup enabled — a security patch that silently
# stops applying is worse than one that was never written.
set -eu

# Pinned deliberately. A release that resolves its build tool at @latest is not
# reproducible, and this patch is tied to a known template.
GOGIO_VERSION="${GOGIO_VERSION:-v0.10.0}"
OUT="${1:-$(go env GOPATH)/bin/gogio}"

# Made absolute BEFORE the build, which runs from a temp directory. A relative
# -o would resolve against that temp directory and the binary would vanish with
# it, leaving the script exiting 0 having produced nothing.
case "$OUT" in
  /*) ;;
  *)  OUT="$(pwd)/$OUT" ;;
esac

echo "gogio-hardened: building gioui.org/cmd@${GOGIO_VERSION} with backup disabled"

# Fetched through the module proxy so the bytes are checksum-verified, rather
# than cloned from a forge over plain git.
go mod download -x "gioui.org/cmd@${GOGIO_VERSION}" >/dev/null 2>&1 || \
  GOFLAGS=-mod=mod go mod download "gioui.org/cmd@${GOGIO_VERSION}"

SRC="$(go env GOMODCACHE)/gioui.org/cmd@${GOGIO_VERSION}"
[ -d "$SRC" ] || { echo "gogio-hardened: module not in cache: $SRC" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp -R "$SRC/." "$WORK/"
chmod -R u+w "$WORK"

python3 - "$WORK" <<'PY'
import sys, os

work = sys.argv[1]
path = os.path.join(work, "gogio", "androidbuild.go")
src = open(path).read()

# The exact <application> line from gogio's APK manifest template.
old = '\t<application {{.IconSnip}} android:label="{{.AppName}}">'
new = ('\t<application {{.IconSnip}} android:label="{{.AppName}}"\n'
       '\t\tandroid:allowBackup="false">')

n = src.count(old)
if n != 1:
    sys.exit(
        "gogio-hardened: expected exactly 1 <application> template line, found %d.\n"
        "gogio's manifest template has changed. Re-read gogio/androidbuild.go and\n"
        "update this patch — do NOT relax the check, because the failure mode of a\n"
        "patch that no longer applies is an APK with Android backup enabled." % n)

open(path, "w").write(src.replace(old, new))
print("gogio-hardened: manifest template patched (allowBackup=false)")
PY

(cd "$WORK" && go build -trimpath -o "$OUT" ./gogio)
echo "gogio-hardened: built $OUT"
