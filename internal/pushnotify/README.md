# pushnotify — tappable new-mail notifications (Android)

This package makes a new-mail notification **open the exact mailbox** when
tapped. It mirrors `internal/biometric`: a tiny Go JNI shim
(`notify_android.go`) drives a bundled Java helper (`VayuNotify.java`), with a
no-op stub (`notify_stub.go`) on every other platform so the host and
`CGO_ENABLED=0` builds stay clean.

> **Status: UNVERIFIED.** The Android path was written without an Android
> SDK/NDK and CI does not build the APK, so it has never been compiled or run.
> It needs the steps below plus a round of on-device testing.

## 1. Build the jar (required)

gogio bundles `*.jar` files it finds in an imported package's directory — it
does **not** compile `.java`. Produce `VayuNotify.jar` next to the sources
(same as `VayuBiometric.jar`):

```sh
cd internal/pushnotify
go generate ./...        # runs the javac + jar commands in notify_android.go
# or manually:
javac --release 8 -classpath "$ANDROID_HOME/platforms/android-34/android.jar" -d /tmp/vayunotify VayuNotify.java
jar cf VayuNotify.jar -C /tmp/vayunotify .
```

Commit `VayuNotify.jar` so the APK build picks it up.

## 2. AndroidManifest — POST_NOTIFICATIONS (required on Android 13+)

Android 13 (API 33) requires the runtime permission `POST_NOTIFICATIONS`;
without it, `NotificationManager.notify` silently drops the notification.
gogio's generated manifest does not include it, so add it (via gogio's manifest
customization for your build) and request it at runtime once at startup.

## 3. Foreground service — background delivery when the app is CLOSED (still TODO)

This package only makes notifications **tappable**. Delivering them when the app
is fully closed still needs a foreground service (see the stub in
`internal/push/android_fgservice.go`) plus a `<service>` entry and
`FOREGROUND_SERVICE` permission in the manifest — that is a separate, larger
change. While the app is open (or recently backgrounded) the existing sync fires
notifications; tapping them now opens the right mailbox.

## Flow

`ui/notifications.go` calls `pushnotify.Post(id, title, body, accountID,
folderID)`. On tap, Android re-launches the activity with the mailbox as intent
extras; `HandleViewEvent` (fed from `cmd/vayumail/main.go`) reads them via
`VayuNotify.consumeTap` and calls the tap handler, which is wired to
`ui.SetMailNavTarget` — the frame loop then opens that account + folder.
