# platform/android

Build output and manual manifest overrides for the Android target.

```sh
make android    # runs gogio -target android ./cmd/vayumail
```

`gogio` generates the APK manifest. It adds `INTERNET` by default, and it
adds any other permission **only when the app imports the matching
`gioui.org/app/permission/*` package** — there is no separate manual
manifest-merge step. So a permission that is not backed by an import is
simply absent from the APK.

Declared permissions, constitutionally bounded by
[ADR-0005](../../docs/ADR-0005-android-permissions.md):

| Permission | How it gets into the manifest |
|---|---|
| `INTERNET` | Added by gogio automatically. |
| `FOREGROUND_SERVICE` | Pending — added when the foreground sync service is wired (no Gio permission package; needs a manifest fragment). |
| `RECEIVE_BOOT_COMPLETED` | Pending — added with the boot receiver. |
| `USE_BIOMETRIC` | Not yet in the manifest. gogio only emits permissions backed by a `gioui.org/app/permission/*` import, and there is no biometric package, so it cannot be added the normal way. The fingerprint-unlock helper (`internal/biometric`) uses the framework `BiometricPrompt`, which is a *normal*-protection permission: it works without the manifest entry on most devices, and the helper catches any `SecurityException` and falls back to the PIN. A manifest-inject step in `release.yml` (or a gogio patch) can add it explicitly. |

## Backup hardening (audit L12) — now enforced in the release pipeline

A stock gogio manifest leaves Android's default `allowBackup="true"`, which makes
the app-private directory — `vayumail.db` and the sealed keystore
(`credentials.sealed` + `master.key`) — eligible for `adb backup` and cloud Auto
Backup. That is the no-root path from an at-rest weakness to an off-device one.

This was previously described here as pending, while `CHANGELOG.md` and both rule
files stated it as done. It is now actually done, and the difference is worth
being precise about.

`scripts/gogio-hardened.sh` builds gogio from a **pinned** module version and
patches its compiled-in manifest template so `<application>` carries:

```xml
android:allowBackup="false"
```

`release.yml` runs that script instead of `go install gioui.org/cmd/gogio@latest`.
The patch asserts it matched exactly one `<application>` line, so a future gogio
that rewords the template fails the release rather than silently shipping an APK
with backup enabled. Three tests in `test/android_manifest_audit_test.go` pin the
wiring, the assertion and the pinning; a fourth fails the build if any file in the
tree claims backup is disabled while the pipeline does not disable it.

`android:dataExtractionRules` / `android:fullBackupContent` are deliberately **not**
set. They are `@xml/` resource references, and gogio builds its `res/` directory
internally, so wiring them means a second, riskier patch for no additional
protection — `allowBackup="false"` already disables cloud backup, device-to-device
transfer and `adb backup` outright. The two rule files stay here as the exclusion
list to apply if backup is ever deliberately re-enabled, and both now say so
rather than claiming to be active.

`CAMERA` was withdrawn at v2.0.0 together with QR scanning
([ADR-0009](../../docs/ADR-0009-retire-qr-scanning-direct-connect.md));
onboarding is direct connect (email + app password, autoconfig-discovered)
or a pasted setup code — neither needs a permission. Any permission beyond
ADR-0005's set requires a new ADR.

Pending platform work (COMPLIANCE-TRACKER.md): Android Keystore bridge
(`internal/crypto.PlatformBridge`), foreground sync service
(`internal/push.ForegroundServiceController`), and the boot-completed
receiver.
