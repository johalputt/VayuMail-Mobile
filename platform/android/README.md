# platform/android

Build output and manual manifest overrides for the Android target.

```sh
make android    # runs gogio -target android ./cmd/vayumail
```

`gogio` generates the APK manifest. It adds `INTERNET` by default, and it
adds any other permission **only when the app imports the matching
`gioui.org/app/permission/*` package** — there is no separate manual
manifest-merge step. So a permission that is not backed by an import is
simply absent from the APK (this is exactly why CAMERA was missing until
v1.4.2: nothing imported `gioui.org/app/permission/camera`).

Declared permissions, constitutionally bounded by
[ADR-0005](../../docs/ADR-0005-android-permissions.md):

| Permission | How it gets into the manifest |
|---|---|
| `INTERNET` | Added by gogio automatically. |
| `CAMERA` | Blank import `gioui.org/app/permission/camera` in `cmd/vayumail` (v1.4.2). |
| `FOREGROUND_SERVICE` | Pending — added when the foreground sync service is wired (no Gio permission package; needs a manifest fragment). |
| `RECEIVE_BOOT_COMPLETED` | Pending — added with the boot receiver. |

Any permission beyond the four in ADR-0005 requires a new ADR.

## Camera QR scanning (NDK Camera2)

The onboarding QR scanner's camera feed is implemented in
[`internal/camera/camera_android.go`](../../internal/camera/camera_android.go)
as a **pure-cgo bridge over the NDK Camera2 API** — no Java/Kotlin source, so
`make android` builds it unchanged. It links `libcamera2ndk`, `libmediandk`,
`libandroid`, and `liblog` (all NDK-provided; gogio's toolchain has them on the
default link path). It streams the `YUV_420_888` luminance plane straight to the
gozxing decoder, requests the already-declared CAMERA permission over JNI on
scanner open, powers on lazily, and releases the device after a short idle.

This file compiles only for `GOOS=android` and can only be exercised on a
physical device — the desktop/CI build uses the no-op `camera_other.go`. After
`make android`, test on-device: grant the camera prompt, point at a VayuMail
setup QR, and confirm it decodes; check `adb logcat -s vayumail-camera` for any
Camera2 errors if the preview stays black.

Pending platform work (COMPLIANCE-TRACKER.md): Android Keystore bridge
(`internal/crypto.PlatformBridge`), foreground sync service
(`internal/push.ForegroundServiceController`), boot-completed receiver, and iOS
camera capture.
