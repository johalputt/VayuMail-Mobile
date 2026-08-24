# VayuMail-Mobile — Deep Code & Security Audit · Consolidated Report

**Date:** 2026-08-24 · **Target:** `johalputt/VayuMail-Mobile` @ main (`33d83d2`, post 2.2.17) · **Method:** inline read-only deep review of every security-critical package (crypto, pgp, imapsync, smtpsend, mime, chat, store, syncmanager, applock, account provisioning), the UI layer, platform manifests and both CI workflows; every finding verified against source; full gate suite executed locally on Windows (`go test ./...` across all 196 files / ~27.7k LOC, 53 test files).

---

## Executive summary

VayuMail-Mobile is **one of the most disciplined Go codebases in this fleet** — the constitutional model, the honest COMPLIANCE-TRACKER, the meta-tests that enforce security *invariants* (`test/transport_security_test.go` bans `math/rand` imports and any `InsecureSkipVerify` mention at compile time), and the mutation-tested remediation history put it above typical commercial mail clients. No SQL injection, no plaintext credential path, no TLS downgrade, no unsigned-provisioning bypass was found. The attacker-facing surface is small because remote content is never fetched by design.

The real risk concentrates in exactly two places, and both are already half-admitted by the repo itself:

| # | Root cause | Where it bites |
|---|---|---|
| RC-1 | **The hardware keystore seam exists but nothing calls it** — secrets are sealed under a master key that lives in a file *beside* the ciphertext | One sandbox escape / root / backup bug away from total compromise incl. PGP private keys |
| RC-2 | **Background delivery is a stub** — no foreground service, so IMAP IDLE dies when Android backgrounds the app | The app cannot actually deliver mail in real time on a phone, its headline feature |

Everything else is hardening, efficiency, or product polish.

Totals: 2 HIGH · 8 MEDIUM · 8 LOW/INFO.

> **Remediation status:** M2, L1, L2 and L4 were fixed in the same session
> that produced this report — see the `[Unreleased]` section of
> `CHANGELOG.md` for the mechanism each fix shipped with. Everything else
> remains open and is scheduled in `docs/UPGRADE-PLAN.md`.

---

## Verified strengths (claims that ARE controls)

These were each traced to source and, where possible, to their enforcement test:

- **Credentials & PGP private keys sealed with AES-256-GCM**, alias-bound as AAD, fresh nonce per write, atomic persist via unique-temp + fsync + rename + dir-sync (`internal/crypto/sealed.go`). Master-key creation race closed twice over: `link(2)` create-if-not-exists, with an `O_CREATE|O_EXCL` fallback for Android's SELinux link ban — losers adopt the winner's key instead of orphaning secrets (2.2.17 fix, regression-tested).
- **App lock**: PBKDF2-SHA-256 @600k iterations (OWASP floor), constant-time compare, verifier fail-closed against empty hashes, 5 free attempts then 30s→15min doubling lockout; TOTP second factor RFC-vector-tested (`internal/applock/`).
- **Provisioning**: Ed25519 over canonical JSON, expiry checked, self-certifying-key honesty documented, every host bound to the mailbox domain, token endpoint https-only + userinfo-free + port-443-only, redirect refusal, endpoint re-vetted at point of use (`internal/mail/account/qrprovision.go`).
- **Transport**: plaintext IMAP/SMTP impossible in production builds (`allowPlainTransport=false` const, build-tag flipped only for tests); TLS 1.2 floor written down; optional SPKI pin enforced on **every** handshake including resumed sessions (`VerifyConnection`, the G123-class fix); SSRF guards (public-domain regex, IP-literal and localhost refusal) on autoconfig, WKD, keydir, VayuTalk and privkey paths; every network body size-capped.
- **PGP sender binding**: `SenderVerified` is true only when the signing key carries the claimed From address — the UI refuses to render "verified" unless the fingerprint matches the peer's verified key (`ui/state/chatstate_events.go:278`).
- **Store hygiene**: parameterized queries throughout (no string-built SQL found), WAL + foreign_keys + busy_timeout + temp_store=MEMORY pragmas tuned for Android, append-only migrations, FTS5 external-content with change-gated triggers, folder-leading indexes added in v5 after measuring the scans.
- **UI architecture**: immutable snapshot + coalesced async reloads; layout never touches SQLite/network; virtualized fixed-height list; zero-allocation closed-form spring physics; frames requested only while something animates; precomputed row text preserves the text-shaper cache.
- **CI**: constitution gate, govulncheck, gofmt/vet/golangci-lint/staticcheck/gosec(high/high), go.mod tidy check, race-detector tests, fuzz smoke, android/arm64+arm cross-compile of the engine, gogio manifest-patch application asserted, binary size budget, coverage floor that can actually fail.

## 🔴 HIGH findings

| # | Finding | Where |
|---|---|---|
| H1 | **Secrets at rest are only as strong as the OS sandbox — the hardware bridge is never registered.** `RegisterPlatform` has zero callers; `keystore()` falls through to the sealed store whose master key is a sibling 0600 file. The comment admits it ("audit M16 … stopgap until the hardware bridge is wired") and the tracker rows PENDING it. Practical impact today: PGP private keys (`sealPrivateKey` → `ks.Store`) and all mail credentials are one app-private-directory read from disclosure; `allowBackup="false"` closes the easy road but not root, OEM bugs, or a future backup-rule regression. This is the single highest-value upgrade available. | `internal/crypto/keystore.go:93` (no callers) · `cmd/vayumail/main.go:151-172` · `internal/crypto/sealed.go:26-38` · `COMPLIANCE-TRACKER.md:22` |
| H2 | **No background mail delivery.** The foreground-service controller is a logged no-op; Android freezes/kills the process minutes after backgrounding, killing the held IDLE socket. Real-time delivery — the product's core promise — works only while the screen is on and the app foregrounded. Notifications for new mail therefore cannot fire for backgrounded users at all. | `internal/push/android_fgservice.go:22-45` · `COMPLIANCE-TRACKER.md:23-24` |

## 🟡 MEDIUM findings

| # | Finding | Where |
|---|---|---|
| M1 | **Private-key escrow model**: the device fetches its armored private key by POSTing the mailbox password to VayuPress; the server (and anyone holding the credential — phishing, server compromise, a malicious operator build) can obtain the key that decrypts all past and future mail. Sovereign-server trust is assumed, but the client could remove itself from that trust chain for new accounts (generate keys on-device, publish public only). | `internal/mail/account/privkey.go:37-74` |
| M2 | **Atomic-write primitive assumes POSIX rename semantics** — found by executing the repo's own race test off-Linux: concurrent first-seals fail on Windows with `rename … Access is denied` (sharing violation on the target). Single-process Android masks it; multi-instance/multi-process use of one sealed path is unguarded and the audit test documents intent the code does not deliver cross-platform. Fix: bounded retry-on-sharing-violation inside `writeFileAtomic` (standard Windows remedy), then make the test pass on all three OSes. | `internal/crypto/sealed.go:311-342` · `test/keystore_race_audit_test.go:90` (fails on windows/amd64, passes ubuntu CI) |
| M3 | **Full-mailbox UID inventory per unilateral event**: flag refresh and expunge reconciliation fetch UID sets for UIDs 1→∞ on every notification — O(mailbox) traffic on large folders. HIGHESTMODSEQ is already tracked; CONDSTORE/QRESYNC deltas would make these O(changes). | `internal/mail/imapsync/idle.go:212-275` |
| M4 | **Every user action dials a fresh TLS+IMAP connection** (mark-read, move, send trigger `WithConnection`): handshake latency + battery cost per tap on mobile radio. Reuse a per-account command connection (or the held IDLE socket when legal). | `internal/syncmanager/exec.go:100,166,212,265,314,331` |
| M5 | **Auto-WKD sweeps contact correspondent domains automatically** (default-on, 10-min throttle, `"0"` opts out). It is throttled and skip-known-keys cheap, but it does leak "this address received mail" metadata to recipients' servers — a nuance against the README's blanket "never phones home". Consider first-contact prompt or opt-in default. | `ui/state/appstate.go:130-132,284-289` |
| M6 | **Biometric unlock ships without `USE_BIOMETRIC`** in the manifest (gogio limitation, admitted) — the helper survives via SecurityException fallback, but the feature is dead weight until release.yml injects the permission. Extend the existing hardened-manifest patch to inject permissions while it is patching `<application>` anyway. | `COMPLIANCE-TRACKER.md:66` · `scripts/gogio-hardened.sh` · `.github/workflows/release.yml:58-59` |
| M7 | **Untrusted avatar bytes hit oksvg/rasterx and x/image decoders with no fuzz target.** Input capped (1 MiB, 128 px raster) and domain-allow-listed, but parser CVE history argues for `FuzzAvatarDecode` alongside the MIME fuzzers. | `internal/avatarimg/cache.go:163-217` · `test/fuzz_test.go` |
| M8 | **CI actions/tools pinned by tag, some at `@latest`** (staticcheck, gosec, govulncheck resolve latest at run time; actions use `@v7/@v5` tags, not SHAs). A compromised upstream action or linter release executes inside the trusted build. Pin actions by commit SHA and tools by version. | `.github/workflows/ci.yml:9,36,74,79,84` · `release.yml:31-44,157,200,238` |

## 🟢 LOW / INFO

| # | Finding | Where |
|---|---|---|
| L1 | No `.gitattributes`: on `core.autocrlf=true` checkouts every Go file flips to CRLF and `gofmt -l` flags the entire tree (reproduced on Windows this audit). Add `*.go text eol=lf` (+ `*.md` rules) so gates are OS-independent. | repo root |
| L2 | After a successful `publishExcl` fallback creation (or adoption) the master key is returned without populating `FileKeyProvider.key`, so every subsequent seal/unseal re-reads master.key from disk — extra secret handling and I/O on a hot-ish path. Cache it like the other success paths. | `internal/crypto/sealed.go:145-148` |
| L3 | `DecryptFrom` reads the decompressed plaintext unbounded (`io.ReadAll(md.UnverifiedBody)`); upstream caps exist (fetch limits, 1 MiB part caps) but the PGP layer itself has no ceiling against a decompression bomb if a caller ever passes raw network bytes. Add `maxPlaintextBytes`. | `internal/mail/pgp/decrypt.go:76-79` |
| L4 | Unpinned accounts get `TLSConfig() == nil` → toolchain-default verification. The written TLS 1.2 floor applies only to pinned configs today; return a default config with `MinVersion: tls.VersionTLS12` for the unpinned path too (the comment in pin.go already argues floors should be stated, not inherited). | `internal/mail/account/pin.go:24-27` |
| L5 | HTML mail renders text-only (strongest sanitizer there is, and honest about it). World-class competitors render styled HTML; the milestone needs a strict allowlist sanitizer + fuzzing before any markup reaches the canvas. Tracked as PENDING; keep it out of the security story until it exists. | `internal/mail/mime/render.go` |
| L6 | OAuth tokens are static (no refresh flow) — acknowledged PENDING; Gmail/Yahoo modern-auth users will hit re-auth walls. | `COMPLIANCE-TRACKER.md:45` |
| L7 | `SetMaxOpenConns(1)` is deliberate and right for mobile WAL; noting here so nobody "fixes" it into SQLITE_BUSY races. | `internal/store/db.go:51-54` |
| L8 | iOS is absent end-to-end (no IPA pipeline, no Keychain bridge, APNs relay pending) — roadmap, not defect. | `platform/ios/README.md` |

## Hacker-audit traces attempted (all refused)

- Malicious QR/setup code steering to attacker host/port/internal address → blocked by signature + domain-binding + port/https vetting + redirect refusal.
- Plaintext/downgrade provisioning payload → rejected (`ErrInsecureTransport`; const false).
- Forged "verified sender" in mail or chat → signing-fingerprint↔From binding; UI refuses mismatched fingerprints.
- Tracking pixel / remote-content beacon → nothing is ever fetched; detection is parse-time only.
- SQL/HTML/script injection through message content → parameterized store; text-only renderer drops script/style/iframe/object/embed/svg/template subtrees.
- Credential recovery via `adb backup`/cloud backup → `allowBackup="false"` patched into gogio itself with application asserted in CI (the L12 lesson institutionalized).
- Brute-force PIN/TOTP offline vs leaked verifier → 600k PBKDF2 + lockout ladder; online → same ladder shared.
- Token replay to off-domain host via redirect (chat/connect, privkey exchange) → `CheckRedirect` refusals everywhere.

## Verdict

Ship-blocking issues: none in code correctness; H1/H2 are strategic gaps rather than exploitable bugs. The fastest route to "world class" is not a rewrite — it is wiring the two seams the architecture already prepared for (hardware keystore, foreground service) and then harvesting the performance and UX upgrades listed in `docs/UPGRADE-PLAN.md`.
