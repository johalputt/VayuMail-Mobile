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
| SQLite store + FTS5 | COMPLETE | modernc.org/sqlite, WAL, versioned migrations, external-content FTS5 with triggers, injection-safe query builder, tested |
| Setup-code provisioning (decode + verify) | COMPLETE | Ed25519 over canonical JSON, all six rejection paths fixture-tested (Rule 7); arrives by paste since v2.0.0 (ADR-0009) |
| Setup-code token exchange | COMPLETE | Client tested against httptest; reference issuer ships in this repo (`cmd/vayumail-provision`, `GET /code`, ADR-0008/0009) |
| Credential persistence (sealed keystore) | COMPLETE | AES-256-GCM sealed store in the app-private data directory; alias-bound ciphertext, atomic writes, tested incl. plaintext-leak and replay checks (ADR-0004 amendment) |
| Hardware-backed key wrapping | PENDING | `KeyProvider` seam exists; Android Keystore / iOS Keychain wrapping of the master key lands without a format change (ADR-0004) |
| Android foreground service | STUB | `internal/push/android_fgservice.go` — engine-side controller registration complete; not wired to an OS service |
| iOS APNs | PENDING | Deferred (Phase 5) — foreground sync only on iOS at v0.1.0; needs a VayuPress-side APNs relay |
| Autodiscover RFC 6186 | STUB | `account.Autodiscover` returns ErrAutodiscoverUnavailable; setup falls back to manual entry; QR path unaffected |
| Tracking protection | COMPLETE | Pixel + tracker-host detection at parse time, "sender tracks you" indicator; remote content is never fetched by design (ADR-0007) |
| Newsletter detection + unsubscribe | COMPLETE | List-Id flags list mail; RFC 2369/8058 mailto unsubscribe executed end-to-end, https targets copied for the user |
| Snooze | COMPLETE | Local snooze until tomorrow 8:00; hidden from all lists until the deadline |
| Undo send | COMPLETE | 10-second recall window before any connection opens |
| Unified inbox | COMPLETE | "All inboxes" across accounts with combined unread badge |
| Search operators + body search | COMPLETE | from:/subject:/has:attachment/is:unread/before:/after: + FTS over fetched bodies |
| Attachment download | COMPLETE | Per-file chips in the thread view; fetched on demand, saved to the app attachments dir (AttachmentSavedEvent) |
| Sent-folder append | COMPLETE | Successful sends are filed to Sent with \Seen |
| Draft save to IMAP | COMPLETE | SaveDraftCmd appends to Drafts with \Draft |
| PGP key management UI | COMPLETE | Keys persisted (schema v2), Settings screen: import by paste, WKD lookup, trust cycling, removal |
| WKD key discovery | COMPLETE | draft-koch advanced+direct endpoints, known-answer tested; auto-discovery on new mail is default-on since v2.0.0 (throttled 10-min sweeps, "0" setting opts out) |
| Auto-encrypt | COMPLETE | Composer enables encryption automatically when every recipient has a key |
| TLS key pinning (SPKI) | COMPLETE | Optional per-account pin, CLI managed, WebPKI + pin required (ADR-0008) |
| Settings sync via mailbox | COMPLETE | Sealed AES-GCM blob in VayuMail.Meta, CLI push/pull, memserver-tested (ADR-0008); in-app UI wiring PENDING |
| Reference provisioning server | COMPLETE | cmd/vayumail-provision: signed payloads, QR PNG rendering, single-use token exchange |
| MIME parser fuzzing | COMPLETE | FuzzMIMEParse + FuzzHTMLToText, seeded, smoke-run in CI |
| Constitution CI gate | COMPLETE | scripts/constitution.sh enforces all 10 rules mechanically on every push |
| Reproducible release builds | COMPLETE | -trimpath, pinned toolchains, committed pure-Go icon generator |
| Rich text compose | PENDING | Plain text only, by design; rich rendering of received HTML is a styled-text milestone |
| OAuth2 token refresh | PENDING | Static password (or one-shot OAuth token from provisioning) only |
| JMAP protocol | PENDING | Deliberate own milestone — protocol-scale work, tracked for v2 |
| F-Droid distribution | PENDING | Reproducible builds land the prerequisite; submission is an external process |
| Attachment picker | PARTIAL | Composer attach button opens the platform file picker via `gioui.org/x/explorer` (SAF on Android); picked files are added as chips (tap to remove), read up to 50 MB, and sent via the existing MIME attachment path. Android/iOS pickers verified on-device only; filename falls back to a type-derived name where the OS stream carries none. |
| On-demand body fetch in UI | PARTIAL | Messages > 512 KiB sync envelope-only; `imapsync.FetchBody` exists but no UI command triggers it yet (thread view shows a "not downloaded" notice) |
| New-mail notifications | COMPLETE | gioui.org/x/notify (Android tray, desktop DBus); bursts coalesced into a summary, suppressed during the initial-sync window |
| Haptic feedback on swipe | PENDING | No cross-platform haptics wired; swipe works without it |
| Swipe row exit animation | PARTIAL | Reveal follows finger, snap-back animated; committed rows disappear without a slide-out animation |
| Hardware back button (Android) | COMPLETE | Back/Escape closes the drawer, pops the stack, closes the window at the root |
| Android startup (non-blocking boot) | COMPLETE | Boot loop presents the first frame immediately; DataDir/SQLite/keystore init off the UI thread with on-screen error reporting (fixes the startup freeze) |
| Archive/move correctness | COMPLETE | Server move first, then local delete; no reused placeholder UID (fixes UNIQUE collision on multi-archive); regression-tested |
| Constitution CI enforcement | COMPLETE | v1.1 gate enforces channel invariants, math/rand ban, QR rejection completeness, ADR cross-refs; govulncheck + staticcheck in CI |
| Server key pinning for setup-code payloads | PENDING | Trusts payload-embedded key + HTTPS exchange (ADR-0003); direct connect rides WebPKI |
| Direct-connect onboarding | COMPLETE | Email + app password → autoconfig discovery over HTTPS (schema-checked, SSRF-vetted, contract-tested against VayuPress); setup-code paste and manual entry as fallbacks; QR scanning/camera removed (ADR-0009) |
| App lock (PIN + idle auto-lock) | COMPLETE | internal/applock: stdlib PBKDF2-SHA-256 verifier in the keystore, constant-time compare, 5 free attempts then doubling lockout to 15 min; UI gate replaces the frame; idle re-lock via frame-gap; disk-scan test proves the PIN never lands on disk (ADR-0010) |
| Account sign-out | COMPLETE | RemoveAccountCmd: bounded stop of the account's sync goroutines, keystore credential delete, cascading row delete, AccountRemovedEvent; goleak-tested |
| Multi-account switcher | COMPLETE | Drawer account header: switch accounts, add account; per-account sync + sign-out in Settings |
| Notifications toggle | COMPLETE | Settings switch (default on); gates the notifier before posting |
| Message forward + thread action bar | COMPLETE | Reply / Forward (quoted plain text) / Archive / Delete-with-confirm from the thread screen |
| Motion system (ui/anim) | COMPLETE | Time-interpolated easings, staggered list entrance, press-scale buttons/FAB, parallax screen transitions, animated switches/dialog/PIN pad; frames requested only while something animates (battery: idle screens render zero frames) |
| Two-factor unlock (TOTP) | COMPLETE | RFC 6238 second factor on the app lock: keystore-resident secret, RFC 4226 vector-tested, shared lockout ladder with the PIN, atomic enrollment (wrong confirm code removes the secret) |
| In-app password update | COMPLETE | UpdateCredentialCmd: per-account inline form in Settings; sync stops, keystore entry overwritten in place, reconnects; clears the auth-error banner |
| Compose-time key acquisition | COMPLETE | Turning encryption on fetches missing recipient keys via WKD immediately; live per-recipient readout in the action row; send names any recipient still without a key |
| Encrypt-to-self | COMPLETE | Outbound PGP mail also encrypts to the sender's own key when known, keeping the Sent copy readable; own-account keys lead every WKD sweep |
| Message details panel | COMPLETE | Per-message disclosure: full From/To/Cc, date, PGP/transport security line, tracking honesty, size |
| Pull-to-refresh | COMPLETE | Passive vertical-drag observer over the inbox list (taps and row swipes unaffected); rubber-band indicator, spins while the sync runs |
| Notification preview toggle | COMPLETE | Privacy option: generic "New mail" line instead of sender/subject on the lock screen |
| Full-bleed launcher icon | COMPLETE | tools/appicon regenerates cmd/vayumail/appicon.png from the committed brand artwork (edge-to-edge, launcher-mask-safe); `make icon` |
| APK release pipeline | COMPLETE | `.github/workflows/release.yml`: gogio build on GitHub runners, signature verification, artifact + Release upload on `v*` tags; Play-ready when `ANDROID_KEYSTORE_B64`/`ANDROID_KEYSTORE_PASS` secrets are set, test-signed otherwise |
| iOS IPA pipeline | PENDING | Requires a macOS runner and Apple signing assets |

## Environment note

Engine (`internal/*`) and `cmd/vayumail-cli` build with `CGO_ENABLED=0`
— verified in CI. The Gio UI binds to system windowing interfaces on
Linux/Android at build time (see ADR-0006, "cgo status").
