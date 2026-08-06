#!/bin/sh
# Constitution compliance gate (GOVERNANCE-CONSTITUTION.md v1.1).
#
# Every check maps to a numbered rule and fails the build on violation.
# This is the mechanical half of the constitution; the human half is code
# review. Run locally with `make constitution`; CI runs it as its own job
# on every push and pull request.
set -u
fail=0
violation() { echo "CONSTITUTION VIOLATION [$1]: $2"; fail=1; }

# ── Rule 1 — pure Go engine, no cgo ─────────────────────────────────────
if grep -q "mattn/go-sqlite3" go.mod go.sum 2>/dev/null; then
  violation "Rule 1" "cgo SQLite driver (mattn/go-sqlite3) present"
fi
if ! CGO_ENABLED=0 go build ./internal/... ./cmd/vayumail-cli/... ./cmd/vayumail-provision/... >/dev/null 2>&1; then
  violation "Rule 1" "engine/CLI does not build with CGO_ENABLED=0"
fi

# ── Rule 2 — permissive licenses only (no copyleft) ─────────────────────
if grep -qiE "AGPL|LGPL|(^|[^A-Za-z-])GPL-[23]" go.mod 2>/dev/null; then
  violation "Rule 2" "copyleft marker found in go.mod"
fi

# ── Import listing, shared by Rules 3 and 4 ─────────────────────────────
# Emits "file<TAB>importpath" for every import DECLARATION in the given dirs.
#
# Grepping the raw source for a quoted path is what this replaces, and both
# callers below had the same bug: a grep cannot tell an import from a string
# literal that happens to look like one. Rule 3 fired on a test holding
# "../internal/mail/imapsync/client.go" in a slice of files to read, and Rule 4
# would fire on a comment that names gioui.org while explaining why a package
# may import it. A gate that cries wolf is a gate people switch off.
list_imports() {
  find "$@" -name "*.go" -not -path "*/.git/*" 2>/dev/null | while IFS= read -r f; do
    awk -v F="$f" '
      /^import[ \t]*\(/ { inblk = 1; next }
      inblk && /^\)/    { inblk = 0; next }
      {
        if (inblk) {
          if (match($0, /"[^"]+"/)) print F "\t" substr($0, RSTART + 1, RLENGTH - 2)
        } else if ($0 ~ /^import[ \t]/) {
          if (match($0, /"[^"]+"/)) print F "\t" substr($0, RSTART + 1, RLENGTH - 2)
        }
      }
    ' "$f"
  done
}

# ── Rule 3 — module path and no relative imports ────────────────────────
head -1 go.mod | grep -q "^module github.com/johalputt/VayuMail-Mobile$" || \
  violation "Rule 3" "module path changed"
rel=$(list_imports . | awk -F'\t' '$2 ~ /^\.\.?\//' || true)
[ -z "$rel" ] || violation "Rule 3" "relative import found: $rel"

# ── Rule 4 — engine packages never import Gio ───────────────────────────
# Packages permitted to import Gio despite living outside ui/ and platform/.
# Both are JNI bridges that need gioui.org/app's JVM handle; neither can be
# written without naming those types. Kept in step with gioAllowedOutsideUI in
# test/constitution_test.go by TestTheShellAndGoConstitutionAgreeOnRule4 —
# change one and you must change the other.
GIO_ALLOWED="internal/biometric internal/pushnotify"
#
# The scope is every package under internal/ and ui/state/, not the three
# hand-listed directories this used to check. Eleven of the fourteen internal
# packages, and the whole of ui/state, were outside the old grep — so the rule
# CLAUDE.md calls "do not violate" was unenforced almost everywhere it applies.
gio=$(list_imports internal ui/state | awk -F'\t' '$2 ~ /^gioui\.org/ { print $1 }' | sort -u || true)
for f in $gio; do
  pkg=$(dirname "$f" | sed 's|^\./||')
  allowed=0
  for a in $GIO_ALLOWED; do
    [ "$pkg" = "$a" ] && allowed=1
  done
  [ "$allowed" -eq 1 ] || violation "Rule 4" "gio import in engine: $f"
done

# ── Rule 5 — async discipline ───────────────────────────────────────────
# No time.Sleep in the UI layer (would block the frame goroutine).
if grep -rn --include="*.go" "time.Sleep" ui/ 2>/dev/null | grep -v "_test.go" | grep -q .; then
  violation "Rule 5" "blocking sleep in the UI layer"
fi
# The typed channel buffer sizes are fixed by ARCHITECTURE.md.
grep -q "make(chan Event, 256)" internal/syncmanager/manager.go || \
  violation "Rule 5" "eventCh buffer is not 256 (ARCHITECTURE.md)"
grep -q "make(chan Cmd, 64)" internal/syncmanager/manager.go || \
  violation "Rule 5" "cmdCh buffer is not 64 (ARCHITECTURE.md)"

# ── Rule 6 — credential sovereignty ─────────────────────────────────────
if grep -inE "password|oauth_token|credential" internal/store/schema.go | grep -q .; then
  violation "Rule 6" "credential-shaped column in the SQLite schema"
fi
# Random used for secrets/tokens must be crypto/rand, never math/rand.
if grep -rn --include="*.go" '"math/rand"' internal/ cmd/ ui/ 2>/dev/null | grep -v "_test.go" | grep -q .; then
  violation "Rule 6" "math/rand in production code (use crypto/rand)"
fi

# ── Rule 7 — QR verification completeness ───────────────────────────────
# Every rejection path from ADR-0003 must exist in the verifier.
for e in ErrUnknownVersion ErrExpired ErrInvalidSignature ErrInsecureTransport ErrInvalidPort; do
  grep -q "$e" internal/mail/account/qrprovision.go || \
    violation "Rule 7" "verifier missing rejection path $e"
done

# ── Rule 8 — permission honesty ─────────────────────────────────────────
if grep -rn --include="*.xml" --include="*.md" -E \
  "ACCESS_(FINE|COARSE)_LOCATION|READ_CONTACTS|RECORD_AUDIO|READ_SMS|READ_CALL_LOG|READ_EXTERNAL_STORAGE" \
  platform/ 2>/dev/null | grep -q .; then
  violation "Rule 8" "forbidden Android permission referenced under platform/"
fi

# ── Rule 9 — honest compliance ──────────────────────────────────────────
if grep -rl --include="*.go" "STUB:" . >/dev/null 2>&1; then
  grep -q "STUB" COMPLIANCE-TRACKER.md || \
    violation "Rule 9" "STUB markers exist but COMPLIANCE-TRACKER.md has no STUB entries"
fi
if grep -rn --include="*.go" -E "//\s*(TODO|FIXME)" . | grep -v "_test.go" | grep -q .; then
  violation "Rule 9" "unmarked TODO/FIXME in production code"
fi

# ── Rule 10 — file size discipline ──────────────────────────────────────
for f in $(find . -name "*.go" -not -path "./.git/*"); do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt 400 ]; then
    violation "Rule 10" "$f is $lines lines (max 400)"
  fi
done

# ── Cross-cutting: zero telemetry ───────────────────────────────────────
if grep -qiE "firebase|crashlytics|sentry|amplitude|mixpanel|segment\.io|datadog|google-analytics" go.mod; then
  violation "Telemetry" "analytics/crash SDK in dependency graph"
fi

# ── Cross-cutting: every referenced ADR exists ──────────────────────────
for adr in $(grep -rohE "ADR-00[0-9][0-9]" --include="*.go" --include="*.md" . | sort -u); do
  ls docs/${adr}-*.md >/dev/null 2>&1 || \
    violation "Docs" "referenced $adr has no docs/${adr}-*.md file"
done

if [ "$fail" -eq 0 ]; then
  echo "constitution: all checks passed"
fi
exit "$fail"
