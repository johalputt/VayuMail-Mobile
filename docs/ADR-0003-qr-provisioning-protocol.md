# ADR-0003 — QR provisioning protocol

## Status

Accepted — v0.1.0. Mobile-side decode and verification complete; the live
token exchange requires the VayuPress server-side endpoint
(`/.well-known/vayumail/provision`), a cross-repo dependency that is not
yet built.

## Decision

Account onboarding is driven by a versioned JSON payload carried in a QR
code, base64url-encoded, and signed with Ed25519:

- Canonical signing input: every field except `sig`, keys
  lexicographically sorted, no whitespace, UTF-8.
- Verification order: version → expiry → signature → transport security
  (plaintext refused) → port ranges. Every failure is a typed error
  (`ErrUnknownVersion`, `ErrExpired`, `ErrInvalidSignature`,
  `ErrInsecureTransport`, `ErrInvalidPort`) mapped to a clear user-facing
  message. No field of an unverified payload is ever used to open a
  connection (Constitutional Rule 7).
- The payload carries a short-lived one-time token; `ExchangeToken`
  redeems it over HTTPS for the actual mail credential, which the caller
  places in the platform keystore. The token, not the password, transits
  the QR code.

## Context

Manual IMAP/SMTP configuration is the single worst onboarding experience
in email. A QR code printed by the user's own mail server removes every
keystroke — but an unauthenticated QR code would let an attacker redirect
the app to a hostile server by swapping a sticker. The Ed25519 signature
binds every connection parameter to the server's key; the expiry and
one-time token bound the replay window.

## Consequences

- A malicious QR cannot silently redirect the app to an
  attacker-controlled server: tampering breaks the signature, and
  plaintext transports are refused even when correctly signed.
- Key distribution: v0.1 trusts the public key embedded in the payload
  (the signature proves internal consistency and integrity, and the
  subsequent token exchange happens over HTTPS with WebPKI). Pinning the
  server key via a second channel is future work tracked in
  COMPLIANCE-TRACKER.md.
- Test coverage: six signed fixtures in `test/fixtures/qr/` (valid,
  expired, tampered, unknown version, plaintext transport, invalid port)
  are verified in CI with zero network calls.
- The `testonly` build tag may relax the plaintext-transport check for
  local integration tests; production builds never carry that tag.
