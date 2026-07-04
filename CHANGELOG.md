# Changelog

All notable changes to VayuMail-Mobile are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- **Onboard by email — VayuMail autoconfig discovery.** The manual account
  screen gains an **"Auto-detect from email"** button: enter your address, tap
  it, and the server/IMAP-port/SMTP-port fields fill themselves from the
  domain's published autoconfig — you only add your password. Backed by new
  `account.DiscoverAutoconfig`, which fetches the server's first-party document
  at `https://<domain>/.well-known/vayumail/autoconfig.json` (with an
  `autoconfig.<domain>` fallback) and returns an account `Config` prefilled with
  the IMAP/SMTP hosts, ports, TLS modes and username — so a VayuPress mailbox can
  be set up from just an email address, alongside the existing QR and paste-code
  onboarding. The lookup runs off the UI thread and applies its result on the
  next frame (no cross-thread widget mutation). User-initiated only (no
  phone-home); SSRF-guarded (public domains only, no IP literals/loopback),
  size-capped, and it rejects any document whose `schema` it does not recognise.
  The document shape is locked to the VayuPress server by a shared contract test
  (`test/autoconfig_contract_test.go`). Discovery also honours the server's
  declared `auth` field, mapping it to the account's `AuthMech`: `password`
  (VayuPress's default) stays password auth, while `oauthbearer` / `xoauth2`
  select the matching bearer-token mechanism, so a token-minting server is
  configured from discovery alone; an unrecognised value is rejected rather than
  silently mis-configuring the account.
- **WKD interop contract test (shared with VayuPress).** Froze an expanded
  set of WKD address-hash known-answer vectors (`test/wkd_contract_test.go`)
  that is kept byte-for-byte identical to the matching table in the VayuPress
  server. The app computes the lookup hash and VayuPress computes the publish
  hash with two independent z-base-32/SHA-1 implementations; the shared table
  makes any drift between them fail CI on whichever side moved, instead of
  silently breaking key discovery (server publishes at a path the app never
  requests). Also pins case-folding of the local part.

## [1.2.7] — 2026-07-03

### Added
- **Auto-fetch keys on new mail (opt-in).** Settings → PGP → "Auto-fetch
  keys on new mail (WKD)". When on, new mail triggers a throttled WKD
  sweep (at most once per 10 minutes) that imports keys for any
  correspondent still missing one — so VayuPress contacts' keys stay
  current with no taps. Off by default and user-enabled, honouring the
  no-phone-home rule; the setting persists (`auto_wkd`).

## [1.2.6] — 2026-07-03

### Added
- **One-tap contact key discovery via WKD.** Settings → PGP →
  "Fetch contacts’ keys (WKD)" looks up a public key for every address you
  correspond with through Web Key Directory and imports the ones it finds.
  Because VayuPress publishes its users' keys over WKD, this
  pulls your VayuPress contacts' keys with no separate key server and no
  VayuPress change — the two systems interoperate over the open WKD
  standard. User-initiated, per the no-phone-home rule.
  - New `store.CorrespondentEmails` (distinct senders, newest first);
    reuses the existing WKD client.

## [1.2.5] — 2026-07-03

### Added
- **Token-based authentication (modern auth / 2FA).** Accounts can now log
  in with a bearer token instead of a mail password, via SASL
  **OAUTHBEARER** (RFC 7628) or **XOAUTH2** (Google/Microsoft style), for
  both IMAP and SMTP. The mechanism is stored per account (`auth_mech`,
  schema v4); the token itself stays in the platform keystore (Rule 6).
  When a provisioned account returns an OAuth token, VayuMail selects the
  token mechanism automatically (XOAUTH2 when the token type says so, else
  OAUTHBEARER). Interactive 2FA is not part of IMAP/SMTP — this is the
  standards path a 2FA-protected account uses to authenticate.
  - New `internal/mail/account/oauth.go` with a tested XOAUTH2 SASL client.

## [1.2.4] — 2026-07-03

### Added
- **Paste setup code onboarding.** When the camera is unavailable, you can
  now add an account by pasting the QR's setup code (its base64url
  payload, served by the provisioning tool's text endpoint). It runs the
  identical Ed25519-verified provisioning path as a live scan — no field
  of an unverified payload is ever used. The scanner screen and the
  welcome screen both point to this path.

## [1.2.3] — 2026-07-03

### Added
- **PGP key sync from VayuPress.** A new Settings section, "PGP key
  directory (VayuPress)", lets you point VayuMail at a key-directory URL
  on your own site and pull every contact's public key in one tap ("Sync
  keys"). Keys are imported into the keyring and persisted. The client is
  HTTPS-only and never contacts a directory you have not configured.
  - Per-address form: `GET {url}?email={addr}` → armored/binary public key.
  - Bulk form: `GET {url}` → `{"keys":[{"email","armored"}]}`.
  - New `internal/mail/pgp/keydir.go`, `settings` table (schema v3),
    end-to-end tests over an HTTPS test server.

## [1.2.2] — 2026-07-03

### Fixed
- **Tapping a message did nothing.** The row's swipe gesture sat above its
  Clickable and swallowed taps, so messages never opened. The swipeable
  now recognises a press-and-release-in-place as a tap and opens the
  conversation; horizontal drags still archive/delete.
- **Sent mail (and other folders) not showing.** "Sync now" only synced
  INBOX, so messages sent from another client or filed server-side never
  appeared. It now discovers and syncs every folder. Opening a folder
  (Sent, Archive, …) also triggers a one-shot sync of just that folder.

### Added
- **Refresh button** in the inbox toolbar — fetches new mail across all
  folders on demand (`SyncFolderCmd` / `SyncNow`).

## [1.2.1] — 2026-07-03

### Fixed
- **First-launch "disk I/O error" crash.** On Android the app failed at
  startup with `store: migrate: apply migration v2: disk I/O error
  (6410)`. Extended code 6410 is `SQLITE_IOERR_GETTEMPPATH`: the
  migration-v2 FTS5 index rebuild needed an on-disk temp directory, which
  Android does not expose to SQLite. The store now opens with
  `temp_store=MEMORY`, keeping transient b-trees in memory so no OS temp
  path is required. The transaction helper also no longer masks the real
  error with a spurious "cannot rollback" message when SQLite has already
  auto-aborted the transaction.
- **Supply-chain vulnerabilities.** Bumped `golang.org/x/image` to v0.41.0
  and `github.com/cloudflare/circl` to v1.6.3, clearing five reachable
  advisories `govulncheck` flagged (TIFF OOM/panic and a secp384r1
  miscalculation).

### Changed
- **Logo — original artwork.** The app now uses the supplied master logo
  PNGs verbatim (`assets/logo/vayumail.png` / `vayumail-dark.png`) instead
  of a redrawn vector. The launcher icon is the mark cropped from that
  master on an opaque white square, and the in-app splash paints the
  embedded master directly (`ui/logo-light.png`). The redrawn SVGs and the
  `tools/genicon` rasterizer were removed.
- **Static splash.** The launch splash shows the logo statically — no
  draw-on or breathing animation.

## [1.2.0] — 2026-07-03

### Fixed
- **Android startup freeze.** The app opened to a frozen brand mark and
  never progressed. Root cause: `app.DataDir()` and the SQLite open ran
  before the window pumped its first frame, and on Android `app.DataDir()`
  blocks until a window event arrives — a startup deadlock. The window
  now presents frames immediately via a boot loop (`ui/boot.go`), and all
  blocking initialization runs off the UI thread with on-screen error
  reporting instead of an eternal splash.
- **Archive/move UID collision.** Moving a second message into the same
  folder failed on the `UNIQUE(account, folder, uid)` constraint (moved
  rows reused `uid = 0`), so the message was never moved server-side. The
  move now deletes the local row after a successful server move and lets
  the next sync re-add it with its real UID; the server move runs first so
  a failure never loses the message locally.
- **HIGHESTMODSEQ reset.** The IDLE delta-sync path reset the stored
  CONDSTORE modseq to zero on every pass; it is now preserved.
- **Provisioning-server token leak.** Unredeemed one-time tokens are now
  pruned on expiry instead of accumulating forever.

### Changed
- **New logo.** Redrawn as a single confident rounded "V" (a wind-drawn
  mark). New `vayumail-icon/​wordmark/​dark` SVGs, an **animated** wordmark
  that draws itself on, a regenerated launcher icon, and an **animated
  in-app splash** that renders the mark live on every launch.

### Added
- End-to-end provisioning tests proving the reference server's signed
  payload verifies with the client's own verifier (canonical JSON parity),
  plus single-use and expiry coverage.
- Regression test for the archive/move UID collision.
- Constitution **v1.1**: mechanical enforcement extended (channel-buffer
  invariants, `math/rand` ban, QR rejection-path completeness, ADR
  cross-reference integrity) and a documented Enforcement map.
- `SECURITY.md` (threat model + disclosure), `CHANGELOG.md`, issue/PR
  templates, `CODEOWNERS`, Dependabot, and `govulncheck` in CI.

## [1.1.0] — 2026-07-02

### Added
- **Intelligence (on-device):** unified "All inboxes" view, tracking-pixel
  detection with a "sender tracks you" indicator, newsletter detection and
  RFC 2369/8058 one-tap unsubscribe, snooze, 10-second undo-send, search
  operators (`from:`/`subject:`/`has:attachment`/`is:unread`/`before:`/
  `after:`) and full-body FTS.
- **PGP UX:** persisted keys, WKD discovery, in-app key management, and
  auto-encrypt when every recipient has a key.
- **Sovereignty:** optional per-account TLS SPKI pinning, encrypted
  multi-device settings sync through the user's own mailbox, and a
  reference provisioning server (`cmd/vayumail-provision`).
- Schema v2, ADR-0007 and ADR-0008, parser fuzzing, coverage and
  binary-size CI gates.

## [1.0.0] — 2026-07-02

### Added
- New-mail system notifications (Android tray / desktop DBus).
- Sealed AES-256-GCM credential store surviving restarts (ADR-0004
  amendment).
- Android hardware back-button navigation.
- Signed-APK release pipeline (GitHub Actions) and launcher icon.

## [0.1.0] — 2026-07-02

### Added
- Initial pure-Go mobile email client: IMAP IDLE sync, SMTP outbox, MIME
  parse/render, OpenPGP, SQLite + FTS5, Ed25519 QR provisioning, hand-
  rolled Gio UI, the ten-rule constitution, ADR-0001…0006, and an offline
  test suite.

[Unreleased]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/johalputt/VayuMail-Mobile/releases/tag/v0.1.0
