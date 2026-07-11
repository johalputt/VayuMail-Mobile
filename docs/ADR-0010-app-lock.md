# ADR-0010 — App lock: PIN gate with idle auto-lock

- **Status:** Accepted (v2.0.0). **Amended v2.1.0:** an optional RFC
  6238 TOTP second factor joins the PIN — same authenticator secret a
  VayuPress operator already carries, stored in the keystore beside the
  verifier, verified with a ±1-step window against RFC 4226 test
  vectors, and throttled by the same lockout ladder as the PIN.
  Enrollment is atomic (secret + one live code, or nothing), and
  disabling the app lock removes both factors.

## Context

An email client holds the most sensitive data on a phone. Device-level
locks don't cover the handed-over-phone and left-on-desk cases, and
enterprise checklists expect an in-app lock. The constraint set is
tight: pure Go (Rule 1), no new dependencies (Rule 2), no secrets on
disk outside the keystore (Rule 6), and a Gio UI that must never block
a frame (Rule 5).

## Decision

**Verifier, not secret.** The PIN itself is never persisted. Enrolling
derives a PBKDF2-SHA-256 verifier (600 000 iterations — Go 1.24+ ships
`crypto/pbkdf2` in the standard library — 16-byte `crypto/rand` salt,
32-byte key) stored as JSON under the keystore alias
`vayumail-applock-v1`, in the same sealed AES-256-GCM store (or
platform keystore) that guards mail credentials. Comparison is
constant-time (`crypto/subtle`). SQLite never sees PIN material —
only lockout bookkeeping (failure count, locked-until timestamp) and
the auto-lock window live in the settings table.

**Online guessing is throttled.** Five free attempts, then a 30-second
lockout that doubles per subsequent failure, capped at 15 minutes. A
correct PIN resets the ladder. `Verify` consults the lockout before
touching the counter, so hammering during a lockout cannot extend it.

**The gate replaces the frame.** While locked, the root draws only the
PIN screen — no mail pixels render underneath, so app switchers and
screenshots capture nothing. Verification runs on a goroutine; the
lock screen reads an outcome mailbox on the next frame (Rule 5).

**Idle auto-lock via frame-gap detection.** Gio v0.10 exposes no
lifecycle/stage events, so the root measures the wall-clock gap
between rendered frames: a backgrounded or idle app stops rendering,
and a gap beyond the user's window (30 seconds / 1 / 5 / 15 minutes;
default 1 minute) re-arms the lock before the next frame draws. The
30-second floor exists because a foreground screen someone is quietly
reading also renders no frames — a shorter window would lock mid-read. This is
platform-independent and needs no cgo.

**Honest scope.** This is an access gate, not at-rest encryption: the
sealed store's master key does not derive from the PIN (a 4-digit PIN
would weaken it — see ADR-0004's hardware-wrapping plan, which remains
the at-rest roadmap). Biometrics need platform bridges and are
deferred to the gomobile-bind milestone alongside the Keystore bridge.

## Consequences

- `internal/applock` is engine-side, Gio-free, and fully testable —
  including a disk scan proving the literal PIN never lands in any
  file (`test/applock_test.go`).
- Wrong-PIN lockouts survive restarts (settings table), while the
  verifier survives exactly as long as the credential store does:
  wiping app data removes both, which is correct — the data the PIN
  guards is gone too.
- The frame-gap heuristic locks on idle as well as on background; for
  a mail client this is the stricter, safer reading of "auto-lock".
