# ADR-0009 — Retire QR scanning; direct connect is the onboarding path

- **Status:** Accepted (v2.0.0)
- **Amends:** ADR-0003 (protocol retained, camera transport retired),
  ADR-0005 (CAMERA permission removed), Constitution Rules 7 and 8
  (v1.2)

## Context

QR scanning was the flagship onboarding path at v0.1.0: the server
displayed an Ed25519-signed payload as a QR code, the app scanned it
with an NDK Camera2 bridge and gozxing decoder, verified the signature,
and redeemed a one-time token. Since then the ground shifted:

1. **VayuPress removed QR generation in v3.9.16.** The companion
   server — the reason the scanner existed — no longer renders setup
   QRs. Its supported path is manual settings plus the
   `/.well-known/vayumail/autoconfig.json` document (ADR-0108 in the
   VayuPress repository), which this app already consumes and
   contract-tests (`test/autoconfig_contract_test.go`).
2. **The camera was our heaviest platform surface.** A 300-line cgo
   bridge that CI could never exercise (device-only), an iOS
   counterpart that never landed, a runtime permission Android could
   only grant through Settings (the Gio context is not an Activity),
   and the app's single non-INTERNET dangerous permission.
3. **Autoconfig discovery is the better first-run.** Typing an email
   address and an app password is faster than pointing a camera at a
   screen, works when the QR is on the same device (the common
   self-hosted case — where scanning is impossible), and rides WebPKI
   channel authentication instead of trusting whatever screen happens
   to be in front of the lens.

## Decision

- **Remove the scanner, the camera bridge, and the CAMERA
  permission**: `platform/camera/` (NDK Camera2 cgo bridge),
  `ui/widgets/qrscanner.go`, the gozxing dependency, and the
  `gioui.org/app/permission/camera` blank import are deleted. The APK
  manifest now declares INTERNET only, until the foreground sync
  service lands.
- **Direct connect is the primary onboarding**: email + app password →
  `account.DiscoverAutoconfig` fetches the domain's signed-over-HTTPS
  autoconfig document → the account connects. VayuPress operators mint
  app passwords in the VayuOS console (ADR-0126 in the VayuPress
  repository).
- **The signed-payload protocol survives as the setup code.**
  `internal/mail/account/qrprovision.go` is untouched: payloads are
  opaque bytes and `ParseAndVerify` never cared how they arrived. The
  paste-a-setup-code path runs the identical Ed25519 verification and
  one-time token exchange (Rule 7). `cmd/vayumail-provision` keeps
  issuing the same payloads at `GET /code` (and the legacy `/qr`
  path), minus the PNG rendering.
- **Constitution v1.2** rewords Rule 7 from "QR-derived account
  payloads" to signed provisioning payloads regardless of transport,
  and drops CAMERA from Rule 8's permission table. The mechanical
  checks are unchanged: the Rule 7 gate still greps the same five
  rejection errors in the same file.

## Consequences

- Onboarding needs zero permissions and works on every platform CI can
  build, including desktop.
- The engine's attacker-facing provisioning surface is byte-identical;
  all six rejection-path fixtures keep passing unmodified.
- Anyone still running a QR workflow can copy the code text under the
  QR (every generator shows it) and paste it — the payload is the same
  string that was in the QR.
- gozxing and its `golang.org/x/xerrors` companion leave the
  dependency tree (ADR-0006 updated); the engine binary shrinks
  accordingly.
