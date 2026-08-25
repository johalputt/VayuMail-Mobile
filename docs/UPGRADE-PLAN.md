# VayuMail-Mobile — World-Class Upgrade Plan

**Companion to:** `docs/SECURITY-AUDIT-2026-08.md` (finding IDs H*/M*/L* reference that report).
**Principle carried from the constitution:** a claim is not a control — every hardening item below lands **with its enforcement test**, and releases are cut once per completed track, not per step.

---

## The language question first: stay pure Go

No second language is needed and none is recommended. Go + Gio is precisely why this app can be light (one toolchain, no Electron/WebView, no JVM runtime of its own), auditable end-to-end, and cross-platform from one tree. Where the OS demands Java/Kotlin (BiometricPrompt today; foreground service next), the established pattern is a tiny synchronous Java helper over JNI — no Gradle, no AndroidX, ~100 lines each (`internal/biometric` is the worked example). A Rust or Kotlin rewrite would multiply the audit surface for zero user-visible gain. Everything below fits inside the existing architecture.

---

## Phase 0 — Ship hygiene (≈ half day, zero product risk)

| # | Task | Closes | Acceptance test |
|---|---|---|---|
| 0.1 | ✅ **DONE** — `.gitattributes` pins text sources to LF (MIME fixtures `-text`, byte-exact) | L1 | Clone on Windows with `autocrlf=true`; `gofmt -l .` empty |
| 0.2 | ✅ **DONE** — `writeFileAtomic` retries the rename while Windows reports the target busy (`renameBusy` predicate, build-tag split; POSIX path unchanged) | M2 | `TestConcurrentFirstSealsRemainReadable` green on windows/amd64 as well as linux |
| 0.3 | Pin GitHub Actions by commit SHA; pin staticcheck/gosec/govulncheck versions in CI | M8 | CI green with immutable refs; dependabot keeps SHAs updated |
| 0.4 | Extend release size budget to the APK: record arm64 APK bytes per release, fail > current+10% | — | Budget step in release.yml fails on a bloated build |

Also landed with this track (see `CHANGELOG.md [Unreleased]`): the master-key cache after O_EXCL fallback (L2) and the stated TLS 1.2 floor on unpinned accounts (L4).

## Phase 1 — Hardware-backed secrets (closes H1 · the highest-value change in this plan)

**Status (2026-08): PARTIAL.** Implemented as `WrappedKeyProvider` in
`platform/android`: static JNI `wrap`/`unwrap` against an AndroidKeyStore
AES-256 key (`org.vayu.mail.VayuKeystore`, StrongBox attempted first), master
key at rest only inside `hardware.key` ciphertext. Provider logic host-tested;
the on-device acceptance items below await the first real-device run. Deltas
from the sketch: the key alias is `vayumail-wrap`, and wiring goes through the
existing `KeyProvider` seam in `keystore()` rather than a new
`RegisterPlatform` call — same selection point, one less indirection.

The seam already exists (`KeyProvider`, `RegisterPlatform`, format-stable sealed store). Fill it.

1. **Android bridge** (`platform/android`, JNI pattern copied from biometrics):
   - Java helper generates an AES-256 key in **AndroidKeyStore** (StrongBox when `isStrongBoxBacked` available, fall back TEE), alias `vayumail-master`.
   - `GoMasterkey.Store/Fetch/Delete` gomobile-style methods: master-key bytes never touch disk — they live in hardware; the sealed-blob file stays exactly as-is.
   - **No user-auth gate on the key** (`setUserAuthenticationOptional(true)` path): a fingerprint requirement here would lock out the recovery path the app-lock design deliberately keeps open — the same "delete a fix that costs access" rule as the Argon2id cap removal.
2. **Wire it**: call `crypto.RegisterPlatform(...)` during Android startup before engine init; `keystore()` then selects PlatformKeystore naturally.
3. **Fallbacks keep working**: API < 23 devices / missing HW → existing sealed-file provider, unchanged behavior; `VAYUMAIL_REQUIRE_SECURE_KEYSTORE` semantics preserved.
4. **iOS Keychain** bridge lands with Phase 7's iOS pipeline using the same interface.

**Acceptance (each one tested):**

- On an API 28+ device, after fresh install + account setup: `find files/ -name 'master.key'` returns nothing; credentials still unseal across process restarts.
- Mutation: force the bridge registration off → tests assert the fallback file exists (proves which path ran).
- Disk-scan test extended: private keys and credentials never appear unencrypted anywhere under DataDir.
- COMPLIANCE row "Hardware-backed key wrapping" flips PENDING → COMPLETE with the mechanism named.

## Phase 2 — Background delivery (closes H2)

**Status (2026-08): PARTIAL.** Item 1 done end-to-end except on-device
verification: `VayuSyncService.java`, JNI controller registered at engine
start / released at window close, all four permissions injected by
gogio-hardened.sh with CI string-verifying them. Items 2–4 (battery-honesty
settings, cadence option, cold-boot re-verification) remain open.

1. **Foreground service**: Java helper `VayuSyncService` (type `dataSync`), started/stopped from Go via a `push.ForegroundServiceController` binding; persistent "syncing mail" notification; IMAP IDLE goroutines keep running inside it. Permissions injected by extending the hardened-gogio patch: `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_DATA_SYNC`, `POST_NOTIFICATIONS` (and `USE_BIOMETRIC` from M6 while the patch is open).
2. **Doze/OEM reality**: document battery behavior honestly in settings ("background sync runs continuously; disable to save battery"); offer a sync-cadence option for OEMs that ignore foreground services; WorkManager-based periodic catch-up sync as the fallback path.
3. **Notification tap → mailbox** already works via the intent bridge; re-verify against the service-started cold-boot path.
4. Later, optional: VayuPress WebPush/push relay for servers that want instant notify without a held socket (also unlocks iOS).

**Acceptance:** device backgrounded ≥ 30 min → mail sent to the account arrives and notifies; `adb shell dumpsys activity services` shows the FGS while syncing; battery-historian shows no wakelock storm; COMPLIANCE STUB row closed.

## Phase 3 — Sync & radio efficiency (the "super fast" pillar)

**Status (2026-08): PARTIAL.** Command-connection reuse shipped:
`Manager.withCommandConn` keeps one authenticated IMAP socket per account
for every commandLoop execution (move/mark/delete/append/sync-now), with a
90-second idle bound, retry-once on a suspect socket, and eviction on
account removal and shutdown. Dial-counting loopback tests prove three
commands now cost one connection. CONDSTORE flag deltas shipped the same
week: `SelectFolder` enables CONDSTORE where advertised, and `refreshFlags`
fetches `CHANGEDSINCE <anchor>` instead of scanning every UID, advancing
the stored anchor from the returned MODSEQs. The CHANGEDSINCE branch is
verified by review against RFC 7162 — no CONDSTORE-capable server exists in
the offline test rig — while the fallback path is loopback-tested.
Expunge reconciliation stays a deliberate UID inventory scan (QRESYNC
VANISHED is not yet exposed by go-imap v2 beta). `COMPRESS=DEFLATE` is
deferred: go-imap-compress targets v1 only and has no tagged release.

| Task | Closes | Notes |
|---|---|---|
| ~~CONDSTORE/QRESYNC deltas~~ **DONE 2026-08 (flags)**: CONDSTORE-aware SELECT + CHANGEDSINCE refresh; expunges still inventory-scanned | M3 | HIGHESTMODSEQ anchor advances from FETCH MODSEQs; full-scan fallback preserved |
| ~~Per-account command connection reuse~~ **DONE 2026-08**: `withCommandConn` in `connpool.go`; one persistent control connection per account, IDLE socket stays dedicated | M4 | Retry-once replaces reconnect/backoff for the pooled socket; `RunIDLE` untouched |
| ~~COMPRESS=DEFLATE~~ **DEFERRED**: no v2-compatible module exists | — | Revisit if upstream ships an extension or a custom one is justified by profiling |
| `COMPRESS=DEFLATE` on IMAP where advertised | — | Big win on body fetches over mobile radio |
| Batched UID-fetch window sizing by folder size/bandwidth | — | Already delta-synced; tune chunk sizes |
| Outbox SMTP connection reuse for multi-send bursts | — | Rare path; only if profiling justifies |

**Acceptance:** store_perf-style harness comparing pre/post network bytes and wall-clock for (a) mark-read in a 20k-message folder, (b) flag-change reconciliation; numbers recorded in the PR.

## Phase 4 — Key model modernization

1. **On-device PGP key generation for new accounts**: generate at provisioning, seal via Phase 1 keystore, publish public key to WKD/VayuPress; `FetchPrivateKey` remains only as legacy-account migration (closes M1 for everyone new).
2. Sign-only outbound (RFC 3156 multipart/signed builder) so users can sign to strangers without encrypting — tracker row PENDING today.
3. OAuth refresh flow: provisioning issues refresh token; refresh happens in engine, token resealed on rotation (tracker row PENDING).
4. Optional SPKI pin offered at provisioning time for VayuPress accounts (payload already carries the server's identity material).

## Phase 5 — Motion & UX polish pass (the "super cool animated" pillar)

Already present and rare-in-Gio: physics springs, press-scale, staggered entrances, parallax push/pop, pull-to-refresh, swipe reveals, dialog enter/exit, zero idle frames. Add, in order:

1. **Thread-open hero transition**: tapped row expands into the thread header instead of a plain slide (row rect → screen-local morph over ~250 ms spring).
2. **Send flight**: composer send triggers a paper-plane accent sweep + snackbar, outbox chip animates in.
3. **Swipe exit completion** (tracker PARTIAL): committed archive/delete rows slide fully out with fade before list collapse.
4. ~~**Haptics**~~ **DONE 2026-08**: gio-x/haptic buzzer (already-pinned x@0.10.2) through the view fan-out; wrong PIN buzzes with the pad shake, pull-commit ticks; swipe-threshold tick still open. On-device JNI verification pending like the rest of the platform row.
5. **Skeleton shimmer** on first folder load instead of blank-then-pop; unread-dot spring pop on new mail.
6. ~~**Reduce-motion setting**~~ **DONE 2026-08**: one atomic gate (`anim.SetMotionEnabled`) enforced inside all four primitives — `Anim`/`Bool`/`Spring`/`Stagger` snap instead of travel, persisted, in Settings, primitive-tested. System animator-scale auto-detect (JNI) still open.
7. Talk room polish: bubble spring-in, burn countdown ring stroke animation (countdown widget already exists), read-receipt checkmark draw-on.

**Acceptance:** profiled at 60 fps on a mid-tier arm64 device (Perfetto trace in PR); idle screens still render zero frames; reduce-motion kills all non-essential movement.

## Phase 6 — Rich HTML mail rendering (the last big product gap)

Text-only rendering is the strongest sanitizer but a UX ceiling. Milestone design:

- ~~Strict allowlist sanitizer~~ **ENGINE DONE 2026-08**: `mime.SanitizeHTML` — flat inert event stream, allowlisted tags, dangerous subtrees dropped, unknown tags unwrapped, hrefs scheme-allowlisted (https/mailto only; rejected anchors emit no link events), images alt-only in v1, input/output caps. Enforcement tests + `FuzzSanitizedHTML` in `./test/`. Remaining: styled-span renderer consuming the stream + feature flag flip.
- ~~Render through the existing text-shaping pipeline as styled spans~~ **DONE 2026-08**: `widgets.RichBody` over gio-x richtext (already-pinned x@v0.10.2 — no WebView); headings/bold/italic/underline/code/lists/quote-indent/tappable links; link taps copied, never auto-opened.
- Fuzz target `FuzzSanitizedHTML` in CI smoke alongside MIME/HTMLToText — seeds run with every CI test pass.
- ~~Feature-flagged~~ **shipped flagged**: `rich_html` setting, default OFF until the pipeline survives a fuzzing season.

## Phase 7 — Distribution & assurance

1. Play internal track live via existing workflow (secrets + upload key); staged rollout policy documented.
2. iOS pipeline: macOS runner, Keychain bridge (Phase 1), APNs relay decision point; IPA job mirrors release.yml.
3. Coverage floor raised as packages move (floor mechanics already exist and can bite); add avatar + sanitizer fuzz targets to the smoke job.
4. Reproducible-build note for F-Droid submission (prerequisite rows already COMPLETE).

## Sequencing & effort

| Phase | Effort | Depends on |
|---|---|---|
| 0 hygiene | ~½ day | — |
| 1 hardware keys | 2–4 days incl. on-device verification | — |
| 2 background sync | 2–3 days + device testing | hardened-manifest patch (0.x) |
| 3 sync efficiency | 2–3 days | — |
| 4 key model | 3–5 days | Phase 1 |
| 5 motion pass | 3–4 days incremental, shippable per item | — |
| 6 rich HTML | 4–6 days, feature-flagged | — |
| 7 distribution | rolling | Phases 1–2 |

Phases 0 → 1 → 2 are the ordered priority (security posture, then the product's core promise). Phases 3–7 interleave after that; each is independently releasable under the one-release-per-track rule.
