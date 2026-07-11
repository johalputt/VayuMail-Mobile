# Architecture Decision Records

Every architectural decision in VayuMail-Mobile is recorded as an ADR —
a short, dated document capturing the context, the decision, and its
consequences. Nothing structural changes without one; the constitution
([GOVERNANCE-CONSTITUTION.md](../GOVERNANCE-CONSTITUTION.md)) requires
that every `ADR-00NN` referenced anywhere in the tree has a file here,
and CI enforces it.

Read [ARCHITECTURE.md](ARCHITECTURE.md) first for the normative topology
and data-flow walkthroughs; the ADRs below record *why* it is that way.

## Index

| ADR | Title | Status | Since |
|---|---|---|---|
| [0001](ADR-0001-package-boundary.md) | Strict Gio-free package boundary | Accepted | v0.1.0 |
| [0002](ADR-0002-sqlite-fts5.md) | SQLite FTS5 for full-text search | Accepted | v0.1.0 |
| [0003](ADR-0003-qr-provisioning-protocol.md) | Signed provisioning protocol | Accepted · amended by 0009 | v0.1.0 |
| [0004](ADR-0004-credential-keystore.md) | Platform keystore for all credentials | Accepted · amended v1.0.0 | v0.1.0 |
| [0005](ADR-0005-android-permissions.md) | Android permissions: the minimum, and no more | Accepted · amended by 0009 | v0.1.0 |
| [0006](ADR-0006-dependency-license-audit.md) | Dependency license audit | Accepted (living) | v0.1.0 |
| [0007](ADR-0007-schema-v2-local-intelligence.md) | Schema v2 and on-device intelligence | Accepted | v1.1.0 |
| [0008](ADR-0008-transport-pinning-and-sync.md) | TLS key pinning, encrypted settings sync, reference provisioning server | Accepted | v1.1.0 |
| [0009](ADR-0009-retire-qr-scanning-direct-connect.md) | Retire QR scanning; direct connect is the onboarding path | Accepted | v2.0.0 |
| [0010](ADR-0010-app-lock.md) | App lock: PIN gate with idle auto-lock | Accepted | v2.0.0 |
| [0011](ADR-0011-device-approval-onboarding.md) | Device-approval onboarding | Accepted | v2.1.6 |

## The v2.0.0 amendment chain

The enterprise redesign turned on three superseding decisions, recorded
rather than silently applied:

- **ADR-0009** retires QR *scanning* and the camera bridge. The Ed25519
  provisioning protocol (ADR-0003) is untouched — payloads now arrive as
  pasted setup codes, or the app skips them entirely via autoconfig
  direct connect. This is what drops the `CAMERA` permission from
  ADR-0005 and the `gozxing` dependency from ADR-0006, and it moves
  Constitution Rule 7 from "QR-derived" to transport-agnostic (v1.2).
- **ADR-0010** adds the PIN app lock, whose verifier lives in the same
  keystore as mail credentials (ADR-0004) and never in SQLite (Rule 6).

## Writing a new ADR

1. Copy the shape of a recent ADR: `# ADR-00NN — Title`, a `## Status`
   line (`Accepted — vX.Y.Z`, plus any `Amended by …`), then
   `## Context`, `## Decision`, `## Consequences`.
2. Use the next free number and add a row to the table above.
3. If the decision changes a mechanical rule, update
   `scripts/constitution.sh` and bump the constitution version in the
   same commit — code may never lead the constitution.
4. If it changes `go.mod`, update ADR-0006 in the same commit.
