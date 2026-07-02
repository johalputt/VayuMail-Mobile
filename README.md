<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/vayumail-dark.svg">
    <img src="assets/logo/vayumail.svg" alt="VayuMail" width="360">
  </picture>

  <p><strong>A sovereign mobile email client, written in pure Go.</strong></p>

  <p>
    <a href="https://github.com/johalputt/VayuMail-Mobile/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/johalputt/VayuMail-Mobile/actions/workflows/ci.yml/badge.svg"></a>
    <a href="https://github.com/johalputt/VayuMail-Mobile/actions/workflows/release.yml"><img alt="Release APK" src="https://github.com/johalputt/VayuMail-Mobile/actions/workflows/release.yml/badge.svg"></a>
    <a href="https://github.com/johalputt/VayuMail-Mobile/releases"><img alt="Latest release" src="https://img.shields.io/github/v/release/johalputt/VayuMail-Mobile?include_prereleases&label=release"></a>
    <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
    <img alt="Go" src="https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white">
    <img alt="Telemetry" src="https://img.shields.io/badge/telemetry-zero-success">
  </p>
</div>

---

Vayu is Sanskrit for wind — the invisible carrier that moves things
without being seen. VayuMail moves your mail the same way: **no telemetry,
no analytics, no vendor lock-in, no credential ever written to disk in
plaintext.** One language, one binary per platform.

VayuMail-Mobile is a sibling project to VayuPress and inherits its
constitutional discipline: ten binding rules in
[GOVERNANCE-CONSTITUTION.md](GOVERNANCE-CONSTITUTION.md), mechanically
enforced by CI on every push (`scripts/constitution.sh`). What is complete
versus stubbed is always truthfully recorded in
[COMPLIANCE-TRACKER.md](COMPLIANCE-TRACKER.md).

## Why VayuMail

| | VayuMail | Typical mail apps |
|---|---|---|
| **Tracking pixels** | Detected and flagged — *"this sender tracks you"* | Silently loaded |
| **Remote content** | Never fetched | Fetched by default |
| **Credentials** | AES-256-GCM sealed store / platform keystore | Often plaintext in a database |
| **Onboarding** | Scan one Ed25519-signed QR code | Type server settings by hand |
| **Encryption** | PGP built in; auto-encrypts when recipients have keys | Plugin or absent |
| **Real-time mail** | One held IMAP IDLE socket | Battery-hungry polling |
| **Telemetry** | None. Verifiable — it's open source | "Anonymized analytics" |
| **Server key pinning** | Optional per-account SPKI pin | Not offered |
| **Multi-device settings** | Encrypted blob in your own mailbox | Vendor cloud account |

## The five pillars

- **Elegance** — every element earns its place or is removed.
- **Minimalism** — fewer UI elements than competitors, not more.
- **Speed** — SQLite on first paint, IMAP IDLE for instant delivery,
  virtualized lists, no spinner where a cached value exists.
- **Intelligence** — automatic threading, unified inbox, newsletter
  detection, one-tap unsubscribe, snooze, undo-send, full-text search
  with operators (`from:`, `subject:`, `has:attachment`, `is:unread`,
  `before:`, `after:`) — all computed on-device. The app learns nothing
  about you and sends nothing anywhere.
- **Lightness** — every dependency justifies its size; the binary-size
  budget is enforced in CI.

## Architecture

```
cmd/vayumail ──► ui (Gio, single-threaded event loop — never blocks)
                  │  eventCh (Event, buffered 256)  ▲
                  ▼  cmdCh   (Cmd,   buffered 64)   │
              internal/syncmanager ──► internal/mail (IMAP/SMTP/MIME/PGP)
                          │
                          ▼
              internal/store (modernc.org/sqlite, WAL, FTS5)
```

- `internal/mail`, `internal/store`, `internal/syncmanager` never import
  Gio — CI-enforced. The engine runs headless (`vayumail-cli`) and builds
  with `CGO_ENABLED=0`.
- Credentials live in the platform keystore or the sealed AES-256-GCM
  store; SQLite holds a keystore alias, never a secret.
- Full topology and data-flow walkthroughs:
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Getting the app

**Android APK** — push a `v*` tag (or run the *Release APK* workflow):
GitHub Actions builds, signs, and attaches `vayumail-<version>.apk` to the
[Release](https://github.com/johalputt/VayuMail-Mobile/releases). Set the
`ANDROID_KEYSTORE_B64` / `ANDROID_KEYSTORE_PASS` repository secrets to
sign with your own upload key for the Play Store; without them builds are
test-signed for sideloading.

**Desktop** — the same binary runs on Linux/macOS/Windows:

```sh
make build && ./dist/vayumail
```

**Headless engine**:

```sh
go run ./cmd/vayumail-cli --help
```

## QR onboarding, end to end

The primary onboarding path is scanning an Ed25519-signed QR code from
your own mail server — zero keystrokes, MITM-proof (Rule 7: nothing from
an unverified payload is ever used). This repository ships the reference
server too:

```sh
echo "you@example.com:app-password" > users.txt
go run ./cmd/vayumail-provision -server mail.example.com -users users.txt
# open http://localhost:8448/qr.png?user=you@example.com and scan
```

See [docs/ADR-0003-qr-provisioning-protocol.md](docs/ADR-0003-qr-provisioning-protocol.md).

## Development

```sh
make lint          # gofmt + vet + boundary + all 10 constitution rules
make race          # full test suite under the race detector
make fuzz          # fuzz the MIME parser (the attacker-facing surface)
make coverage      # engine coverage report
make android       # local APK (requires Android SDK/NDK)
```

Quality gates on every push: constitution compliance, gofmt, go vet,
staticcheck, race-detector tests, parser fuzzing, goroutine-leak checks
(`goleak`), binary-size budget, and the Gio-free engine boundary. All
tests run offline against fixtures and an in-memory IMAP server — CI
needs no credentials and makes no network calls.

## Permissions (Android)

`INTERNET` · `CAMERA` · `FOREGROUND_SERVICE` · `RECEIVE_BOOT_COMPLETED` —
and **no others**, each justified in
[docs/ADR-0005-android-permissions.md](docs/ADR-0005-android-permissions.md).
A manifest change without a new ADR fails review; forbidden permissions
fail CI.

## Governance

- [GOVERNANCE-CONSTITUTION.md](GOVERNANCE-CONSTITUTION.md) — the ten rules, v1.0
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — normative topology
- [docs/](docs/) — ADR-0001 … ADR-0008 (every architectural decision, recorded)
- [COMPLIANCE-TRACKER.md](COMPLIANCE-TRACKER.md) — the honest ledger
- [CONTRIBUTING.md](CONTRIBUTING.md) — how changes land

## License

Apache License 2.0 — see [LICENSE](LICENSE).
