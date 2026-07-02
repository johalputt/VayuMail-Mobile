# ADR-0004 — Platform keystore for all credentials

## Status

Accepted — v0.1.0. Amended — v1.0.0 (sealed keystore, below).

## Decision

Every credential — IMAP password, SMTP password, OAuth token,
provisioning secret, PGP private key material — lives exclusively in the
platform keystore: Android Keystore on Android, Keychain on iOS, both
reached through the `internal/crypto.Keystore` interface and a
gomobile-bound `PlatformBridge`. The SQLite account row stores server
addresses, ports, usernames, TLS modes, and a **keystore alias** — never
the credential itself.

## Context

Credential sovereignty (Constitutional Rule 6): no credential ever
touches disk in application-readable form. A database file that leaks
via backup, debugging, or filesystem access must be worthless to an
attacker.

## Consequences

- Credentials are fetched from the keystore at connection time via a
  `func() (string, error)` closure, used for exactly one connection, and
  discarded. Nothing caches them.
- `AddAccountCmd` zeroes the in-memory credential copy immediately after
  storing it (verified by test).
- gomobile bind is required for production mobile builds; until the
  bridges land, desktop/CI runs use the in-memory keystore, whose
  credentials last one process lifetime and still never touch disk.
- The headless CLI takes its password from `VAYUMAIL_PASSWORD` at
  connect time — not from flags (shell history) and not from the
  database.
- Deleting an account must also delete its keystore entry; the alias in
  the account row is the pointer that makes this reliable.

## Amendment — v1.0.0: sealed keystore

Rule 6 forbids **raw** credentials on disk. v1.0.0 adds
`crypto.SealedKeystore`: credentials are persisted as AES-256-GCM
ciphertext (fresh nonce per write, alias bound as GCM additional data,
atomic file replacement) inside the app-private data directory, so
accounts survive restarts. The master key comes from a `KeyProvider`;
the current `FileKeyProvider` keeps it in a separate 0600 file in the
same sandboxed directory.

Honest security statement: at-rest protection currently equals the
platform app sandbox (comparable to mainstream clients, which store
passwords in their databases outright). The `KeyProvider` seam exists so
hardware-backed wrapping — Android Keystore / iOS Keychain holding the
master key — can replace the file provider **without a storage format
change**. That work is tracked in COMPLIANCE-TRACKER.md
("Hardware-backed key wrapping"). No code path ever writes a plaintext
credential to disk; a test asserts it.
