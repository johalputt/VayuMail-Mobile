# Signing keystore

`debug.keystore` is a **throwaway test key** (password `vayumail-test`,
committed on purpose, like Android Studio's `debug.keystore`). The
release workflow signs test builds with it so that **every sideload
build updates over the previous one** — Android blocks an in-place
update when the signature changes, which silently breaks "download the
new APK and update".

It is **not** a Play Store upload key. For Play-ready builds, set the
`ANDROID_KEYSTORE_B64` / `ANDROID_KEYSTORE_PASS` repository secrets to
your own upload key; the workflow then uses that instead and the debug
key is ignored.
