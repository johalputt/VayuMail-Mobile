# platform/android

Build output and manual manifest overrides for the Android target.

```sh
make android    # runs gogio -target android ./cmd/vayumail
```

`gogio` generates the base APK; the manifest additions below are applied
on top and are constitutionally bounded by
[ADR-0005](../../docs/ADR-0005-android-permissions.md) — exactly four
permissions, nothing else:

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.CAMERA"/>
<uses-permission android:name="android.permission.FOREGROUND_SERVICE"/>
<uses-permission android:name="android.permission.RECEIVE_BOOT_COMPLETED"/>
```

Any manifest diff without a new ADR is a constitutional violation.

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
