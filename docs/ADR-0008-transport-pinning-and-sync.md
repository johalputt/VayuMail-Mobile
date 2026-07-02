# ADR-0008 — TLS key pinning, encrypted settings sync, reference provisioning server

## Status

Accepted — v1.1.0.

## Decisions

**1. Optional per-account SPKI pinning.** `accounts.pinned_spki` stores a
base64 SHA-256 hash of the server's Subject Public Key Info. When set,
IMAP and SMTP connections run standard WebPKI verification **plus**
require some certificate in the verified chain to match the pin —
defense against compromised or coerced CAs. Managed via
`vayumail-cli pin -account N [-save|-clear]`; a mismatch fails the
connection with an explicit interception warning. Fixture-tested with
generated certificates.

**2. Multi-device settings sync through the mailbox.** Settings are
serialized, sealed with AES-256-GCM (`crypto.SealBlob`, context-bound),
and stored as a message in the `VayuMail.Meta` IMAP folder. Any device
holding the same 32-byte sync key can pull them
(`vayumail-cli settings-push` / `settings-pull`,
`VAYUMAIL_SYNC_KEY` env). No vendor cloud, no new account — the user's
mailbox is the sync backend, and the blob is opaque to the mail server.
Round-trip tested against the in-memory IMAP server.

**3. Reference provisioning server.** `cmd/vayumail-provision` is the
canonical server-side implementation of ADR-0003: it signs payloads with
Ed25519 (canonical JSON identical to the client verifier), renders
scannable QR PNGs (`/qr.png?user=…`), and serves the single-use token
exchange (`POST /provision`). VayuPress embeds this logic; any operator
can run it standalone behind TLS. This closes the cross-repo gap that
kept "QR token exchange" PARTIAL.

**4. Reproducible release builds.** Release APKs build with
`-trimpath`, pinned toolchain versions, and a committed, pure-Go icon
generator — no local paths or machine state in the artifact.

## Context

Tier-3 goals: trust that can be verified (pinning, reproducibility) and
multi-device continuity without surrendering sovereignty (mailbox as the
only backend).

## Consequences

- Pinning is opt-in per account: safe default (WebPKI) with a hard mode
  for high-risk users. Key rotation requires re-pinning — documented in
  the CLI output.
- The sync key is user-managed; losing it means re-configuring devices
  (never data loss — settings only).
- The provisioning server holds mail passwords to hand out; the README
  and startup log both state it must sit behind TLS.
