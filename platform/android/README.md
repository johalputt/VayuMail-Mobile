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

Pending platform work (COMPLIANCE-TRACKER.md): Android Keystore bridge
(`internal/crypto.PlatformBridge`), foreground sync service
(`internal/push.ForegroundServiceController`), camera frame source
(`ui/widgets.FrameSource`), boot-completed receiver.
