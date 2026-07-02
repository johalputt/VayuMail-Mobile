# ADR-0004 — Platform keystore for all credentials

## Status

Accepted — v0.1.0. Interface and in-memory adapter complete; the gomobile
Android Keystore / iOS Keychain bridges are stubs tracked in
COMPLIANCE-TRACKER.md.

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
