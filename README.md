<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/vayumail-dark.png">
    <img src="assets/logo/vayumail.png" alt="VayuMail" width="300">
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
| **Onboarding** | Type your email + app password — **everything else auto-discovered** (signed setup codes as fallback) | Type server settings by hand |
| **Encryption** | PGP built in; auto-encrypts when recipients have keys | Plugin or absent |
| **Real-time mail** | One held IMAP IDLE socket | Battery-hungry polling |
| **Telemetry** | None. Verifiable — it's open source | "Anonymized analytics" |
| **Server key pinning** | Optional per-account SPKI pin | Not offered |
| **App lock** | PIN gate, idle auto-lock, offline brute-force throttle | Rare or subscription-gated |
| **Sign out** | One tap: connections closed, credentials wiped from the keystore, local data removed | Often leaves data behind |
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

## Direct connect, end to end

Type your email address and an app password — the app fetches your
server's signed-over-HTTPS autoconfig document
(`/.well-known/vayumail/autoconfig.json`, served by VayuPress and
contract-tested in both repositories) and connects. PGP keys then
auto-sync from the server's WKD directory as mail arrives.

Prefer a handed-out credential? Ed25519-signed **setup codes** run the
same verified provisioning path (Rule 7: nothing from an unverified
payload is ever used), and this repository ships the reference issuer:

```sh
echo "you@example.com:app-password" > users.txt
go run ./cmd/vayumail-provision -server mail.example.com -users users.txt
# fetch http://localhost:8448/code?user=you@example.com and paste it into the app
```

See [docs/ADR-0003-qr-provisioning-protocol.md](docs/ADR-0003-qr-provisioning-protocol.md)
and [docs/ADR-0009-retire-qr-scanning-direct-connect.md](docs/ADR-0009-retire-qr-scanning-direct-connect.md).

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

`INTERNET` · `FOREGROUND_SERVICE` · `RECEIVE_BOOT_COMPLETED` — and **no
others**, each justified in
[docs/ADR-0005-android-permissions.md](docs/ADR-0005-android-permissions.md).
(`CAMERA` was withdrawn at v2.0.0 with QR scanning — ADR-0009.)
A manifest change without a new ADR fails review; forbidden permissions
fail CI.

## Governance

- [GOVERNANCE-CONSTITUTION.md](GOVERNANCE-CONSTITUTION.md) — the ten rules, v1.0
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — normative topology
- [docs/](docs/) — ADR-0001 … ADR-0010 (every architectural decision, recorded)
- [COMPLIANCE-TRACKER.md](COMPLIANCE-TRACKER.md) — the honest ledger
- [CONTRIBUTING.md](CONTRIBUTING.md) — how changes land

## License

Apache License 2.0 — see [LICENSE](LICENSE).
