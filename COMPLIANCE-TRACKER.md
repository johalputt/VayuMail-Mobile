# COMPLIANCE-TRACKER

The live, honest record of what is production-ready and what is not
(Constitutional Rule 9). Every `// STUB:` marker in the code has a row
here; every PARTIAL/PENDING row names its blocker.

Statuses: **COMPLETE** (production-ready, tested) · **PARTIAL** (works
with a named limitation) · **STUB** (interface exists, implementation
does not) · **PENDING** (not started, deliberately deferred).

| Feature | Status | Notes |
|---|---|---|
| IMAP IDLE sync | COMPLETE | go-imap v2; delta sync, reconnect backoff 5s→300s, UIDVALIDITY reset handling, poll fallback for servers without IDLE; offline-tested against in-memory IMAP server |
| SMTP send + outbox | COMPLETE | STARTTLS/TLS, Bcc stripped on wire, retry ladder 1m·2ⁿ, dead-letter after 5 failures, tested |
| MIME parse + render | COMPLETE | go-message; text/HTML/attachments, PGP/MIME detection, sanitized text-only HTML rendering (scripts/styles/iframes dropped), tested |
| PGP encrypt/decrypt | COMPLETE | ProtonMail/go-crypto; encrypt+sign, decrypt+verify, detached signatures, keyring with trust levels, round-trip tested |
| PGP sign-only outbound (RFC 3156 multipart/signed) | PENDING | Engine signs (detached) but the composer path refuses sign-without-encrypt rather than pretending; needs multipart/signed builder |
| PGP key management UI | PENDING | Keyring API complete; in-app import/export screens not built — keys import via engine API only at v0.1.0 |
| SQLite store + FTS5 | COMPLETE | modernc.org/sqlite, WAL, versioned migrations, external-content FTS5 with triggers, injection-safe query builder, tested |
| QR provisioning decode + verify | COMPLETE | Ed25519 over canonical JSON, all six rejection paths fixture-tested (Rule 7) |
| QR token exchange | PARTIAL | Client complete and tested against httptest; requires the VayuPress server endpoint `/.well-known/vayumail/provision` (ADR-0003, cross-repo) |
| Camera preview bridge | STUB | `widgets.FrameSource` hook + decode pipeline complete (gozxing); Android/iOS camera feed via gomobile not implemented — scanner shows "Camera unavailable" |
| Platform keystore bridges | STUB | `internal/crypto` interface + in-memory adapter complete and used; Android Keystore / iOS Keychain gomobile bridges not implemented (ADR-0004) |
| Android foreground service | STUB | `internal/push/android_fgservice.go` — engine-side controller registration complete; not wired to an OS service |
| iOS APNs | PENDING | Deferred (Phase 5) — foreground sync only on iOS at v0.1.0; needs a VayuPress-side APNs relay |
| Autodiscover RFC 6186 | STUB | `account.Autodiscover` returns ErrAutodiscoverUnavailable; setup falls back to manual entry; QR path unaffected |
| Rich text compose | PENDING | Plain text only at v0.1.0, by design |
| OAuth2 token refresh | PENDING | Static password (or one-shot OAuth token from provisioning) only at v0.1.0 |
| Attachment picker | STUB | Composer attach button present; platform document-picker bridge not implemented — shows a snackbar |
| On-demand body fetch in UI | PARTIAL | Messages > 512 KiB sync envelope-only; `imapsync.FetchBody` exists but no UI command triggers it yet (thread view shows a "not downloaded" notice) |
| Haptic feedback on swipe/scan | PENDING | No cross-platform haptics wired; swipe and scan work without it |
| Swipe row exit animation | PARTIAL | Reveal follows finger, snap-back animated; committed rows disappear without a slide-out animation |
| Hardware back button (Android) | PENDING | In-UI back affordances complete; system back key not intercepted |
| Server key pinning for QR payloads | PENDING | v0.1 trusts payload-embedded key + HTTPS exchange (ADR-0003) |
| gogio release pipeline (APK/IPA) | PENDING | Makefile targets exist; store signing/packaging not configured |

## Environment note

Engine (`internal/*`) and `cmd/vayumail-cli` build with `CGO_ENABLED=0`
— verified in CI. The Gio UI binds to system windowing interfaces on
Linux/Android at build time (see ADR-0006, "cgo status").
