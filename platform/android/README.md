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

`CAMERA` was withdrawn at v2.0.0 together with QR scanning
([ADR-0009](../../docs/ADR-0009-retire-qr-scanning-direct-connect.md));
onboarding is direct connect (email + app password, autoconfig-discovered)
or a pasted setup code — neither needs a permission. Any permission beyond
ADR-0005's set requires a new ADR.

Pending platform work (COMPLIANCE-TRACKER.md): Android Keystore bridge
(`internal/crypto.PlatformBridge`), foreground sync service
(`internal/push.ForegroundServiceController`), and the boot-completed
receiver.
