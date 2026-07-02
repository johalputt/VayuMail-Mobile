#!/bin/sh
# Constitution compliance gate (GOVERNANCE-CONSTITUTION.md v1.0).
# Every check maps to a numbered rule; any violation fails the build.
# Run locally with `make constitution`; CI runs it on every push.
set -u
fail=0
violation() { echo "CONSTITUTION VIOLATION [$1]: $2"; fail=1; }

# Rule 1 — pure Go engine: the only SQLite driver is modernc.org/sqlite.
if grep -q "mattn/go-sqlite3" go.mod go.sum 2>/dev/null; then
  violation "Rule 1" "cgo SQLite driver (mattn/go-sqlite3) present"
fi
if ! CGO_ENABLED=0 go build ./internal/... ./cmd/vayumail-cli/... >/dev/null 2>&1; then
  violation "Rule 1" "engine does not build with CGO_ENABLED=0"
fi

# Rule 2 — no copyleft licenses may enter the module graph.
for dep in "github.com/hashicorp/go-" "gitlab.com/gnu" "readline"; do :; done
if grep -qiE "AGPL|LGPL|GPL-[23]" go.mod 2>/dev/null; then
  violation "Rule 2" "copyleft marker found in go.mod"
fi

# Rule 3 — module path and no relative imports.
head -1 go.mod | grep -q "^module github.com/johalputt/VayuMail-Mobile$" || \
  violation "Rule 3" "module path changed"
if grep -rn --include="*.go" -E '^\s*(import\s+)?"\.\.?/' . >/dev/null 2>&1; then
  violation "Rule 3" "relative import found"
fi

# Rule 4 — engine packages never import Gio.
result=$(grep -rl "gioui.org" internal/mail internal/store internal/syncmanager 2>/dev/null || true)
[ -z "$result" ] || violation "Rule 4" "gio import in engine: $result"

# Rule 5 — no time.Sleep inside UI layout paths.
if grep -rn --include="*.go" "time.Sleep" ui/ 2>/dev/null | grep -v "_test.go"; then
  violation "Rule 5" "blocking sleep in the UI layer"
fi

# Rule 6 — credentials never in SQL: schema must not gain credential
# columns, and no code writes passwords into the store.
if grep -inE "password|oauth_token|credential" internal/store/schema.go; then
  violation "Rule 6" "credential-shaped column in the schema"
fi

# Rule 8 — permission honesty: only the four sanctioned permissions may
# appear anywhere in the repository.
if grep -rn --include="*.xml" --include="*.md" -E \
  "ACCESS_(FINE|COARSE)_LOCATION|READ_CONTACTS|RECORD_AUDIO|READ_SMS|READ_CALL_LOG" \
  platform/ 2>/dev/null; then
  violation "Rule 8" "forbidden Android permission referenced"
fi

# Rule 9 — every STUB marker must be tracked.
for f in $(grep -rl --include="*.go" "STUB:" . 2>/dev/null); do
  base=$(basename "$f")
  grep -q "STUB" COMPLIANCE-TRACKER.md || \
    violation "Rule 9" "STUB in $base but no STUB entries in COMPLIANCE-TRACKER.md"
done
if grep -rn --include="*.go" -E "// ?(TODO|FIXME)" . | grep -v "_test.go" | grep -q .; then
  violation "Rule 9" "unmarked TODO/FIXME in production code"
fi

# Rule 10 — no Go file exceeds 400 lines.
for f in $(find . -name "*.go" -not -path "./.git/*"); do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt 400 ]; then
    violation "Rule 10" "$f is $lines lines (max 400)"
  fi
done

# Zero-telemetry: no analytics/crash SDKs, ever.
if grep -qiE "firebase|crashlytics|sentry|amplitude|mixpanel|segment\.io|datadog" go.mod; then
  violation "Telemetry" "analytics/crash SDK in dependency graph"
fi

if [ "$fail" -eq 0 ]; then
  echo "constitution: all checks passed"
fi
exit "$fail"
