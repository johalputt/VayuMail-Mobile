# VayuMail-Mobile

A sovereign mobile email client written in pure Go.

Vayu (वायु) is Sanskrit for wind — the invisible carrier that moves things
without being seen. VayuMail moves your mail the same way: no telemetry, no
analytics, no vendor lock-in, no credential ever written to disk. One
language, one binary per platform.

VayuMail-Mobile is a sibling project to VayuPress and inherits its
constitutional discipline. The binding rules live in
[GOVERNANCE-CONSTITUTION.md](GOVERNANCE-CONSTITUTION.md); what is complete
versus stubbed lives in [COMPLIANCE-TRACKER.md](COMPLIANCE-TRACKER.md).

## Pillars

- **Elegance** — every element earns its place or is removed.
- **Minimalism** — fewer UI elements than competitors, not more.
- **Speed** — SQLite on first paint, IMAP IDLE for real-time delivery,
  no spinner where a cached value can be shown.
- **Intelligence** — automatic threading, QR onboarding, zero learning
  about the user, zero data sent anywhere.
- **Lightness** — every dependency justifies its size.

## Architecture at a glance

```
cmd/vayumail ──> ui (Gio, single-threaded event loop)
                  │  eventCh (Event, buffered 256)  ▲
                  ▼  cmdCh   (Cmd,   buffered 64)   │
              internal/syncmanager ──> internal/mail (IMAP/SMTP/MIME/PGP)
                          │
                          ▼
              internal/store (modernc.org/sqlite, WAL, FTS5)
```

- `internal/mail`, `internal/store`, `internal/syncmanager` never import
  Gio — enforced by CI. The engine is reusable by a CLI, a server plugin,
  or a desktop client without modification.
- Nothing blocks the Gio event loop. All I/O happens in syncmanager
  goroutines; state flows to the UI through typed channels.
- Credentials live only in the platform keystore (Android Keystore /
  iOS Keychain). SQLite stores a keystore alias, never a secret.

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full goroutine
topology and data-flow walkthroughs.

## Onboarding: QR provisioning

Scanning a signed QR code from your mail server is the primary onboarding
path. The payload is Ed25519-signature-verified before any field is used to
open a connection — a malicious QR code cannot redirect the app to an
attacker-controlled server. See
[docs/ADR-0003-qr-provisioning-protocol.md](docs/ADR-0003-qr-provisioning-protocol.md).

## Building

```sh
make build      # desktop binaries into dist/ (engine + UI smoke build)
make race       # go test -race ./...
make lint       # gofmt + go vet + package boundary check
make android    # APK via gogio (requires Android SDK/NDK)
make ios        # iOS app via gogio (requires Xcode on macOS)
```

The headless engine can be exercised without any UI:

```sh
go run ./cmd/vayumail-cli --help
```

## Permissions (Android, v0.1.0)

`INTERNET`, `CAMERA`, `FOREGROUND_SERVICE`, `RECEIVE_BOOT_COMPLETED` — and
no others. Each is justified in
[docs/ADR-0005-android-permissions.md](docs/ADR-0005-android-permissions.md).
Any future permission requires a new ADR before the manifest is touched.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) and the constitution first. All
dependencies must be Apache-2.0 or MIT
([docs/ADR-0006-dependency-license-audit.md](docs/ADR-0006-dependency-license-audit.md)),
pure Go, and cgo-free.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
