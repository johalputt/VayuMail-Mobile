# Security Policy

VayuMail-Mobile is privacy-and-security-first by construction. This
document describes what we protect, how to report a problem, and the
guarantees the codebase enforces.

## Reporting a vulnerability

**Do not open a public issue for security problems.**

Report privately through GitHub's
[Security Advisories](https://github.com/johalputt/VayuMail-Mobile/security/advisories/new)
("Report a vulnerability"). Include:

- affected version or commit,
- a description and, ideally, a minimal reproduction,
- the impact you foresee.

We aim to acknowledge within **72 hours** and to ship a fix or mitigation
for confirmed high-severity issues within **14 days**. We will credit
reporters who wish to be named once a fix is released.

## Supported versions

The latest tagged release and `main` receive security fixes. Older tags
do not.

## Threat model

VayuMail defends primarily against:

1. **Local device compromise / data-at-rest exposure.** Credentials never
   touch disk in plaintext (Constitutional Rule 6). They live in the
   platform keystore (Android Keystore / iOS Keychain) or, until those
   bridges land, an AES-256-GCM sealed store in app-private storage
   (ADR-0004). SQLite holds only a keystore alias, never a secret. A
   test asserts no plaintext credential is ever written.
2. **Malicious onboarding QR codes.** Provisioning payloads are
   Ed25519-signature-verified before any field opens a network
   connection; version, expiry, transport security, and port range are
   all checked, and plaintext transports are refused (Rule 7, ADR-0003).
   A tampered or unsigned payload cannot redirect the app to an
   attacker-controlled server.
3. **Network interception.** IMAP/SMTP are TLS-only; STARTTLS upgrades
   before any credential is sent. Accounts may additionally pin the
   server's SPKI so a compromised or coerced CA cannot substitute a
   certificate (ADR-0008).
4. **Hostile message content.** HTML is rendered as sanitized text only —
   scripts, styles, iframes, and remote references are dropped, and
   **remote content is never fetched**, which also neutralizes tracking
   pixels (the app flags them instead). The MIME parser is fuzzed in CI.
5. **Surveillance / telemetry.** There is none. No analytics or crash
   SDK may enter the dependency graph (enforced by the constitution
   gate). The app learns nothing about the user and sends nothing
   anywhere.

Out of scope for v1.x: a rooted/jailbroken device with an active
attacker, malicious mail-server operators (beyond transport and payload
integrity), and side-channel attacks.

## Hardening in CI

Every push runs, as required checks: the constitution gate (10 rules),
`staticcheck`, `govulncheck` (known-vulnerability scan), `-race` tests,
`goleak` goroutine-leak detection, MIME-parser fuzzing, and the Gio-free
engine-boundary check. See `.github/workflows/ci.yml`.

## Cryptography

- Payload signatures: Ed25519 (`crypto/ed25519`).
- Credential/settings sealing: AES-256-GCM (`crypto/aes`, `crypto/cipher`).
- OpenPGP: `github.com/ProtonMail/go-crypto`.
- All randomness for secrets/tokens uses `crypto/rand`; `math/rand` is
  banned in production code by the constitution gate.
