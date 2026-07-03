# Changelog

All notable changes to VayuMail-Mobile are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
