# VayuMail-Mobile Governance Constitution

**Version: v1.2**

These rules govern all code in this repository without exception. Any
contribution that violates a rule must resolve the violation before merge.
No rule may be bypassed by adding an exception — the rule must be revised by
the maintainer with a documented rationale if it no longer serves the
project.

Every rule below is enforced mechanically where a machine can decide it,
and by review where judgement is required. The mechanical half runs in CI
as its own job on every push and pull request
(`scripts/constitution.sh`); see the **Enforcement** table at the end for
the exact check behind each rule. A rule with a machine check cannot be
merged in violation — the pipeline goes red.

---

## Rule 1 — Pure Go, no cgo

Every dependency must be pure Go or have a documented, tested cgo-free build
path. SQLite driver: `modernc.org/sqlite` only — never `mattn/go-sqlite3` or
any cgo variant. If a required capability has no pure-Go implementation, it
is documented explicitly in COMPLIANCE-TRACKER.md as a known gap, never
hidden behind a silent workaround.

## Rule 2 — Apache 2.0 / MIT dependency chain only

Before any import is added to `go.mod`, its SPDX license identifier must be
confirmed as Apache-2.0 or MIT. GPL, AGPL, LGPL, CDDL, EPL — any copyleft
license — is a hard stop. Every dependency's license confirmation is
recorded in docs/ADR-0006-dependency-license-audit.md, which must be updated
every time `go.mod` changes.

## Rule 3 — Module path

`module github.com/johalputt/VayuMail-Mobile`. All internal imports use this
path. Relative imports are forbidden everywhere in the codebase.

## Rule 4 — Strict package boundary, enforced by CI

`internal/mail/`, `internal/store/`, and `internal/syncmanager/` must never
import `gioui.org/*`, any platform-specific package, or any package from
`ui/` or `platform/`. Only `ui/` and `platform/` may import Gio. CI fails
the build on any violation:

```sh
result=$(grep -rl "gioui.org" internal/mail internal/store internal/syncmanager 2>/dev/null)
[ -z "$result" ] || (echo "BOUNDARY VIOLATION: $result" && exit 1)
```

Purpose: `internal/mail/` could be imported by a CLI, a VayuPress server
plugin, or a desktop client tomorrow without any modification.

## Rule 5 — Async discipline: Gio is single-threaded

Gio's event loop runs on one goroutine. Nothing inside any `Layout()`,
`Update()`, or event handler may block on network I/O, disk I/O, mutexes
held longer than a map lookup, or `time.Sleep()`. All blocking operations
happen in syncmanager goroutines. State flows to the UI through typed
channels polled non-blockingly with `select { default: }`. A violation of
this rule produces UI jank; there are none in this codebase.

## Rule 6 — Credential sovereignty

Raw IMAP passwords, SMTP passwords, OAuth tokens, and provisioning secrets
are never written to SQLite or any file on disk by application code. Ever.
Android uses the Android Keystore; iOS uses the iOS Keychain — both via
gomobile bind. The account row in SQLite contains server addresses, ports,
usernames, TLS settings, and a keystore key alias — never the credential
itself. The credential is fetched from the platform keystore at connection
time, used in memory, and discarded when the connection closes.

## Rule 7 — Provisioning signature verification

Signed provisioning payloads (setup codes — however they arrive: pasted,
fetched, or handed over by tooling) must be Ed25519-signature-verified
before any field is used to open a network connection. An unsigned or
unverifiable payload returns a typed error and produces a clear
user-facing message. It never silently falls back to using unverified
values. A malicious setup code must not be able to redirect the app to
an attacker-controlled mail server. (Reworded from "QR provisioning" in
v1.2 — QR scanning was retired by ADR-0009; the payload format, the
verifier, and every rejection path are unchanged.)

## Rule 8 — Battery and permission honesty

Permissions requested at v2.0.0, and no others:

| Permission | Justification |
|---|---|
| `INTERNET` | Required; self-evident for a mail client |
| `FOREGROUND_SERVICE` | Android background IMAP IDLE (ADR-0005) |
| `RECEIVE_BOOT_COMPLETED` | Restart sync on reboot (ADR-0005) |

`CAMERA` was authorized at v0.1.0 for QR onboarding and withdrawn with
it at v2.0.0 (ADR-0009). No `ACCESS_FINE_LOCATION`. No `READ_CONTACTS`.
No `RECORD_AUDIO`. Any future permission requires a new ADR before the
manifest is touched.

## Rule 9 — Honest compliance

Any incomplete implementation is marked `// STUB: <reason>` in code and
entered in COMPLIANCE-TRACKER.md with status PARTIAL or PENDING. No unmarked
stubs. No silent omissions. COMPLIANCE-TRACKER.md is always a true
representation of what is and is not production-ready.

## Rule 10 — File size discipline

No file exceeds 400 lines. Logic that grows beyond that is split into
focused sub-files. This keeps every file graspable in one reading session.

---

## Enforcement

The `constitution` CI job (`scripts/constitution.sh`) is a required check.
Each rule is enforced as follows — "machine" checks fail the build
automatically; "review" checks are the reviewer's responsibility.

| Rule | Mechanical check (scripts/constitution.sh) | Also |
|---|---|---|
| 1 — Pure Go, no cgo | rejects `mattn/go-sqlite3`; builds engine + both CLIs with `CGO_ENABLED=0` | ADR-0006 |
| 2 — Permissive licenses | greps `go.mod` for AGPL/LGPL/GPL markers | ADR-0006 audit table, review |
| 3 — Module path / no relative imports | asserts module line; greps for `"./"`/`"../"` imports | — |
| 4 — Gio-free engine boundary | greps `internal/{mail,store,syncmanager}` for `gioui.org` | `make boundary` |
| 5 — Async discipline | bans `time.Sleep` in `ui/`; pins eventCh=256 / cmdCh=64 | `-race` + `goleak` tests, review |
| 6 — Credential sovereignty | bans credential columns in schema; bans `math/rand` in production | sealed-keystore tests, review |
| 7 — Provisioning signature verification | asserts every rejection error exists in the verifier | fixture tests, ADR-0003/0009 |
| 8 — Permission honesty | bans forbidden Android permissions under `platform/` | ADR-0005, manifest review |
| 9 — Honest compliance | requires COMPLIANCE-TRACKER entries for STUBs; bans unmarked TODO/FIXME | review |
| 10 — File size | fails any `*.go` over 400 lines | — |
| Telemetry (cross-cutting) | bans analytics/crash SDKs in `go.mod` | review |
| Docs (cross-cutting) | every referenced `ADR-00NN` must have a `docs/` file | — |

Supply-chain integrity is enforced alongside the constitution: CI runs
`govulncheck ./...` (known-vulnerability scan) and `staticcheck ./...` on
every push.

## Amendment process

A rule is amended only by the maintainer, in a commit that changes this
document, bumps its version, records the rationale in a new or updated
ADR, and updates `scripts/constitution.sh` when the mechanical check
changes. Code may never lead the constitution; the constitution leads the
code.

### Changelog

- **v1.2** — Rule 7 generalized from "QR provisioning" to signed
  provisioning payloads over any transport, and Rule 8's permission
  table drops CAMERA — both consequences of retiring QR scanning
  (ADR-0009). No mechanical check changed: the Rule 7 gate verifies the
  same five rejection errors in the same verifier file.
- **v1.1** — Added mechanical enforcement for the channel-buffer
  invariants (Rule 5), `math/rand` ban (Rule 6), QR rejection-path
  completeness (Rule 7), ADR cross-reference integrity, and supply-chain
  scanning (`govulncheck`). Documented the Enforcement map.
- **v1.0** — Initial constitution: the ten rules.
