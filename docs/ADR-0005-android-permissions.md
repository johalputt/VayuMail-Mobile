# ADR-0005 — Android permissions: the minimum, and no more

## Status

Accepted — v0.1.0. **Amended by ADR-0009 (v2.0.0):** the CAMERA
permission was withdrawn with QR scanning, leaving three
(`INTERNET`, `FOREGROUND_SERVICE`, `RECEIVE_BOOT_COMPLETED`).

## Decision

The Android manifest requests exactly these permissions, and no others:

| Permission | Justification |
|---|---|
| `INTERNET` | Required for IMAP and SMTP. Self-evident for a mail client. |
| `FOREGROUND_SERVICE` | Hosts the background IMAP IDLE connections so mail arrives in real time without polling (battery honesty: one held socket beats scheduled wakeups). |
| `RECEIVE_BOOT_COMPLETED` | Restarts the sync service after reboot so the user does not silently stop receiving mail. |

Withdrawn: `CAMERA` (v0.1.0–v1.5.0) existed solely for QR onboarding
and left the manifest with it at v2.0.0 (ADR-0009) — direct connect and
pasted setup codes need no permission at all.

Explicitly refused, permanently unless a future ADR argues otherwise:
`ACCESS_FINE_LOCATION`, `ACCESS_COARSE_LOCATION`, `READ_CONTACTS`,
`RECORD_AUDIO`, `READ_EXTERNAL_STORAGE` (attachment picking will use the
Storage Access Framework, which needs no permission).

## Context

Constitutional Rule 8: battery and permission honesty. Every permission
is an attack-surface and trust cost; a privacy-first mail client must be
auditable at a glance from its manifest.

## Consequences

- Any future permission requires a new ADR **before** the manifest is
  touched; CI reviewers treat a manifest diff without an ADR as a
  constitutional violation.
- Contact autocomplete in the composer cannot read the system address
  book (no `READ_CONTACTS`); it will build on sender history already in
  the local store instead.
- The foreground service shows Android's persistent notification — the
  honest cost of real-time delivery, stated plainly in settings.
